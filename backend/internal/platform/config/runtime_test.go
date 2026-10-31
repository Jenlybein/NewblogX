package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/platform/idgen"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ── 夹具 ──────────────────────────────────────────────────────

// newTestService 建一个跑在内存 sqlite 上的服务。
//
// 不用 testutil：那会让 internal/platform 反过来依赖 test/，
// 而平台层是"零业务依赖"的底座，测试也不例外。
func newTestService(t *testing.T, defaults conf.RuntimeDefaults) (*RuntimeConfigService, *gorm.DB) {
	t.Helper()
	// 每个用例一个独立的内存库，避免 cache=shared 串味。
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	// BaseRow.BeforeCreate 会调 idgen 生成雪花主键，不初始化就插不进去。
	if err := idgen.Init(1); err != nil {
		t.Fatalf("初始化 idgen 失败: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	if err := db.AutoMigrate(&runtimeConfigRow{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return NewRuntimeConfigService(defaults, logger, db), db
}

// quietLogger 返回一个不打印的 logger，用于验证 warn 分支不会 panic。
func quietLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

// readPayload 直接读回某段在库里的原始 JSON，用于确认"存进去的是什么"。
func readPayload(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var row runtimeConfigRow
	if err := db.Where("name = ?", name).Take(&row).Error; err != nil {
		t.Fatalf("读取段 %s 失败: %v", name, err)
	}
	return row.Payload
}

// ── decodeSection：默认值补齐 ────────────────────────────────

func TestDecodeSection_缺字段回退默认值(t *testing.T) {
	// 这是"新增配置字段不写数据迁移"的核心保证：
	// 库里那段 JSON 只有老字段，新字段必须从默认值补齐，而不是变成零值。
	defaults := conf.Logrus{Level: "info"}
	payloads := map[string]string{
		SectionLog: `{"level":"debug"}`,
	}

	got, err := decodeSection(SectionLog, defaults, payloads)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.Level != "debug" {
		t.Fatalf("载荷里的 level 应生效，实际 %q", got.Level)
	}

	// 换一份只含"完全无关字段"的载荷，新字段仍应保留默认值。
	got2, err := decodeSection(SectionLog, defaults, map[string]string{
		SectionLog: `{"some_removed_field":true}`,
	})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got2.Level != "info" {
		t.Fatalf("载荷未涉及 level 时应保留默认值 info，实际 %q", got2.Level)
	}
}

func TestDecodeSection_段缺失时整段用默认值(t *testing.T) {
	defaults := conf.Upload{Whitelist: []string{"jpg", "png"}}

	got, err := decodeSection(SectionUpload, defaults, map[string]string{})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(got.Whitelist) != 2 || got.Whitelist[0] != "jpg" {
		t.Fatalf("段缺失时应整段用默认值，实际 %v", got.Whitelist)
	}
}

func TestDecodeSection_嵌套结构逐字段合并(t *testing.T) {
	// conf.Site 是 8 段嵌套 + 数组，是最容易"整段退化成零值"的地方。
	defaults := conf.Site{Login: siteconf.Login{EmailCodeTimeout: 10}}
	payloads := map[string]string{
		SectionSite: `{"login":{"qq_login":true}}`,
	}

	got, err := decodeSection(SectionSite, defaults, payloads)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !got.Login.QQLogin {
		t.Fatal("载荷里的 qq_login 应生效")
	}
	if got.Login.EmailCodeTimeout != 10 {
		t.Fatalf("同段内未提及的字段应保留默认值 10，实际 %d", got.Login.EmailCodeTimeout)
	}
}

func TestDecodeOrDefault_坏JSON降级不panic(t *testing.T) {
	// "配置坏了"和"数据库连不上"的严重性不同：前者应该降级可用。
	defaults := conf.Mail{Region: "ap-guangzhou", TimeoutSec: 10}
	payloads := map[string]string{SectionMail: `{"region": 不是字符串`}

	got := decodeOrDefault(SectionMail, defaults, payloads, quietLogger())
	if got.Region != "ap-guangzhou" || got.TimeoutSec != 10 {
		t.Fatalf("坏 JSON 应整段回退默认值，实际 %+v", got)
	}
}

// ── InitRuntimeConfig：补行与缓存 ────────────────────────────

func TestInitRuntimeConfig_给缺失段补默认行(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{
		Site:  conf.Site{Project: siteconf.Project{Title: "默认标题"}},
		Mail:  conf.Mail{Region: "ap-guangzhou"},
		Log:   conf.Logrus{Level: "info"},
		OAuth: conf.QQ{AppID: "默认appid"},
	})

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	var count int64
	if err := db.Model(&runtimeConfigRow{}).Count(&count).Error; err != nil {
		t.Fatalf("统计行数失败: %v", err)
	}
	if count != 8 {
		t.Fatalf("八段都应补一行，实际 %d 行", count)
	}

	// 补进去的内容必须是该段默认值的完整 JSON，不能是空字符串。
	if payload := readPayload(t, db, SectionMail); !strings.Contains(payload, "ap-guangzhou") {
		t.Fatalf("mail 段的默认值没写进去，实际 %s", payload)
	}
}

func TestInitRuntimeConfig_已有段不会被覆盖(t *testing.T) {
	// 关键点：重启不能把管理员改过的值冲回默认。
	svc, db := newTestService(t, conf.RuntimeDefaults{
		Mail: conf.Mail{Region: "ap-guangzhou", FromAddress: "默认@example.com"},
	})
	if err := db.Create(&runtimeConfigRow{
		Name:    SectionMail,
		Payload: `{"region":"ap-hongkong","from_address":"改过@example.com"}`,
	}).Error; err != nil {
		t.Fatalf("预置数据失败: %v", err)
	}

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	got := svc.GetRuntimeMail()
	if got.Region != "ap-hongkong" || got.FromAddress != "改过@example.com" {
		t.Fatalf("已有段应保持库里的值，实际 %+v", got)
	}

	// 且不能再插一行同名段（name 上有唯一索引，插进去会直接报错）。
	var count int64
	if err := db.Model(&runtimeConfigRow{}).Where("name = ?", SectionMail).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("mail 段应只有一行，实际 %d 行", count)
	}
}

func TestInitRuntimeConfig_无表时报错而不是静默(t *testing.T) {
	dsn := "file:noruntimecfg?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	svc := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)

	if err := svc.InitRuntimeConfig(); err == nil {
		t.Fatal("表不存在时应返回错误，让启动流程显式失败")
	}
}

func TestInitRuntimeConfig_db为nil时报错(t *testing.T) {
	svc := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), nil)
	if err := svc.InitRuntimeConfig(); err == nil {
		t.Fatal("db 未注入时应返回错误")
	}
}

// ── 更新：落库 + 刷缓存 ─────────────────────────────────────

func TestUpdateMail_落库并立刻生效(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{
		Mail: conf.Mail{Region: "ap-guangzhou", FromAddress: "旧@example.com"},
	})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	newMail := conf.Mail{
		SecretID:    "id-123",
		SecretKey:   "key-456",
		Region:      "ap-hongkong",
		FromAddress: "新@example.com",
		TemplateID:  9527,
		TimeoutSec:  15,
	}
	if err := svc.UpdateMail(newMail); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 1) 缓存立刻生效（热更新：不需要重启）
	got := svc.GetRuntimeMail()
	if got.SecretID != "id-123" || got.Region != "ap-hongkong" || got.TemplateID != 9527 {
		t.Fatalf("缓存未刷新，实际 %+v", got)
	}

	// 2) 库里也真的换了（不是只改内存）
	payload := readPayload(t, db, SectionMail)
	if !strings.Contains(payload, "id-123") || !strings.Contains(payload, "ap-hongkong") {
		t.Fatalf("库里未写入新值，实际 %s", payload)
	}

	// 3) 换成新进程重新加载，值仍然在
	svc2 := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	if err := svc2.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if reloaded := svc2.GetRuntimeMail(); reloaded.TemplateID != 9527 {
		t.Fatalf("重新加载后应读到落库的值，实际 %+v", reloaded)
	}
}

func TestUpdateSections_各段互不影响(t *testing.T) {
	svc, _ := newTestService(t, conf.RuntimeDefaults{})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	if err := svc.UpdateStorage(conf.QiNiu{Enable: true, Bucket: "b1"}); err != nil {
		t.Fatalf("更新 storage 失败: %v", err)
	}
	if err := svc.UpdateOAuth(conf.QQ{AppID: "app1"}); err != nil {
		t.Fatalf("更新 oauth 失败: %v", err)
	}
	if err := svc.UpdateUpload(conf.Upload{Whitelist: []string{"webp"}}); err != nil {
		t.Fatalf("更新 upload 失败: %v", err)
	}
	if err := svc.UpdateLog(conf.Logrus{Level: "warn"}); err != nil {
		t.Fatalf("更新 log 失败: %v", err)
	}

	if !svc.GetRuntimeStorage().Enable || svc.GetRuntimeStorage().Bucket != "b1" {
		t.Fatalf("storage 未更新: %+v", svc.GetRuntimeStorage())
	}
	if svc.GetRuntimeOAuth().AppID != "app1" {
		t.Fatalf("oauth 未更新: %+v", svc.GetRuntimeOAuth())
	}
	if list := svc.GetRuntimeUpload().Whitelist; len(list) != 1 || list[0] != "webp" {
		t.Fatalf("upload 未更新: %+v", list)
	}
	if svc.GetRuntimeLog().Level != "warn" {
		t.Fatalf("log 未更新: %+v", svc.GetRuntimeLog())
	}
}

func TestUpdateSection_为空时插入而非报错(t *testing.T) {
	// 段被手工删掉后，后台再保存一次应该能自愈（upsert 的 insert 分支）。
	svc, db := newTestService(t, conf.RuntimeDefaults{Site: conf.Site{}})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	if err := db.Where("name = ?", SectionSite).Delete(&runtimeConfigRow{}).Error; err != nil {
		t.Fatalf("删除预置段失败: %v", err)
	}

	if err := svc.UpdateSite(conf.Site{Project: siteconf.Project{Title: "补回来"}}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if got := svc.GetRuntimeSite().Project.Title; got != "补回来" {
		t.Fatalf("应能重新插入，实际 %q", got)
	}
}

func TestUpdate_db为nil时报错(t *testing.T) {
	svc := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), nil)
	if err := svc.UpdateMail(conf.Mail{}); err == nil {
		t.Fatal("db 未注入时更新应返回错误")
	}
}

// ── 软删除：唯一索引 + 默认 scope 的陷阱 ────────────────────
//
// 这一组用例守着两个曾出现过的真实故障形态：
// 默认 scope 会"看不见"软删除的行，于是 (a) 保存时走 INSERT 撞唯一索引，
// (b) 启动时 seedMissing 撞唯一索引。两者都表现为整站起不来 / 后台永久报错。

func TestUpdateSection_软删除的段保存时被恢复(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{Site: conf.Site{}})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	// 软删除：行还在表里，只是 deleted_at 非空。
	if err := db.Where("name = ?", SectionSite).Delete(&runtimeConfigRow{}).Error; err != nil {
		t.Fatalf("软删除失败: %v", err)
	}

	if err := svc.UpdateSite(conf.Site{Project: siteconf.Project{Title: "恢复后"}}); err != nil {
		t.Fatalf("软删除后保存不应报唯一索引冲突，实际: %v", err)
	}

	var row runtimeConfigRow
	if err := db.Unscoped().Where("name = ?", SectionSite).Take(&row).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if row.DeletedAt.Valid {
		t.Fatal("保存后该行应已从软删除中恢复")
	}
	if !strings.Contains(row.Payload, "恢复后") {
		t.Fatalf("恢复时应写入新值，实际 %s", row.Payload)
	}
}

func TestInitRuntimeConfig_软删除的段不会撞唯一索引(t *testing.T) {
	// 故障形态：某段被软删除 → loadPayloads 看不到它 → seedMissing 试图插入
	// → 撞 name 唯一索引 → 启动直接失败。正确行为是恢复该行并写回默认值。
	svc, db := newTestService(t, conf.RuntimeDefaults{
		Mail: conf.Mail{Region: "ap-guangzhou"},
	})
	if err := db.Create(&runtimeConfigRow{
		Name: SectionMail, Payload: `{"region":"ap-hongkong"}`,
	}).Error; err != nil {
		t.Fatalf("预置数据失败: %v", err)
	}
	if err := db.Where("name = ?", SectionMail).Delete(&runtimeConfigRow{}).Error; err != nil {
		t.Fatalf("软删除失败: %v", err)
	}

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("软删除的段不应让启动失败，实际: %v", err)
	}

	// 软删除 = "这段配置作废"，所以生效值应回到默认值。
	if got := svc.GetRuntimeMail().Region; got != "ap-guangzhou" {
		t.Fatalf("软删除的段应回到默认值，实际 %q", got)
	}
	// 且行被恢复，不是又插了一行。
	var count int64
	if err := db.Unscoped().Model(&runtimeConfigRow{}).
		Where("name = ?", SectionMail).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("mail 段应只有一行（被恢复而非新建），实际 %d 行", count)
	}
}

// ── 缓存副本 ────────────────────────────────────────────────

func TestGetRuntimeConfig_返回副本(t *testing.T) {
	// 调用方拼装响应时会改字段（把密钥换成占位符）。
	// 如果这里给出缓存指针，"读"就会变成"写"——下次别人读到的就是占位符。
	svc, _ := newTestService(t, conf.RuntimeDefaults{
		Mail: conf.Mail{SecretKey: "真实密钥"},
	})
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	first := svc.GetRuntimeConfig()
	first.Mail.SecretKey = "被改掉了"

	second := svc.GetRuntimeConfig()
	if second.Mail.SecretKey != "真实密钥" {
		t.Fatalf("返回值应是副本，缓存被改坏了：%q", second.Mail.SecretKey)
	}
}

func TestGetRuntimeConfig_未初始化时回退默认值(t *testing.T) {
	svc := NewRuntimeConfigService(conf.RuntimeDefaults{
		Mail: conf.Mail{Region: "ap-guangzhou"},
	}, quietLogger(), nil)

	if got := svc.GetRuntimeMail().Region; got != "ap-guangzhou" {
		t.Fatalf("缓存为空时应回退默认值，实际 %q", got)
	}
}

// ── 段名常量与默认值文件的契约 ──────────────────────────────

func TestDefaultSettingsYaml_段名与常量对齐(t *testing.T) {
	// 段名常量与 site_default_settings.yaml 的顶层键必须对得上：
	// 对不上会静默读到零值，表现为"后台改了没反应"。
	//
	// 这里用 yaml.v3 直接解析而不是 LoadDefaults，因为文件里有 ${BLOGX_*}
	// 占位符，envyaml 缺环境变量会整份拒绝解析 —— 那属于"部署环境问题"，
	// 不该让一个校验键名的单测跟着红。
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}

	var top map[string]any
	if err := yaml.Unmarshal(raw, &top); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	// 只有业务段在这个文件里；log 段属于进程级配置，基线来自 settings.yaml，
	// 故意不写在这里（见 InitRuntimeSite 的注释）。
	want := []string{
		SectionSite, SectionAI, SectionMail,
		SectionStorage, SectionOAuth, SectionUpload, SectionDeco,
	}
	for _, name := range want {
		if _, ok := top[name]; !ok {
			t.Errorf("site_default_settings.yaml 缺少 %q 段（段名常量与文件键名对不上）", name)
		}
	}
}

func TestLoadDefaults_从目录读取(t *testing.T) {
	// 用临时目录，避免依赖真实配置文件里的 ${BLOGX_*} 占位符。
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(settingsPath, []byte("system://n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("写 settings.yaml 失败: %v", err)
	}
	defaultsYAML := `
mail:
  region: ap-hongkong
  from_address: noreply@example.com
  template_id: 42
storage:
  region: z2
  size: 5
upload:
  whitelist:
    - jpg
    - png
`
	if err := os.WriteFile(filepath.Join(dir, "site_default_settings.yaml"), []byte(defaultsYAML), 0o644); err != nil {
		t.Fatalf("写默认值文件失败: %v", err)
	}

	defaults, err := LoadDefaults(settingsPath)
	if err != nil {
		t.Fatalf("读取默认值失败: %v", err)
	}
	if defaults.Mail.Region != "ap-hongkong" || defaults.Mail.TemplateID != 42 {
		t.Fatalf("mail 段未读到: %+v", defaults.Mail)
	}
	if defaults.Storage.Size != 5 {
		t.Fatalf("storage 段未读到: %+v", defaults.Storage)
	}
	if len(defaults.Upload.Whitelist) != 2 {
		t.Fatalf("upload 段未读到: %+v", defaults.Upload)
	}
}

func TestLoadDefaults_路径为空时报错(t *testing.T) {
	if _, err := LoadDefaults(""); err == nil {
		t.Fatal("配置文件路径为空时应返回错误")
	}
}

func TestLoadDefaults_默认值文件缺失时报错(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(settingsPath, []byte("system: {}\n"), 0o644); err != nil {
		t.Fatalf("写 settings.yaml 失败: %v", err)
	}
	if _, err := LoadDefaults(settingsPath); err == nil {
		t.Fatal("默认值文件不存在时应返回错误，让调用方走兜底分支")
	}
}

// ── 结构体 json tag：合并的隐蔽前提 ────────────────────────

func TestAllSections_都有jsonTag(t *testing.T) {
	// decodeSection 靠 JSON 往返做字段级合并，所以每个字段都必须有 json tag。
	// 漏了 tag 的字段在合并后仍能按 Go 字段名匹配上（encoding/json 不区分大小写），
	// 但 map 的键名会变成 Go 字段名，跟库里存的 snake_case 对不上 —— 表现为
	// "改了这个字段没反应"，且完全不报错。
	cases := map[string]any{
		SectionSite:    conf.Site{},
		SectionAI:      conf.AI{},
		SectionMail:    conf.Mail{},
		SectionStorage: conf.QiNiu{},
		SectionOAuth:   conf.QQ{},
		SectionLog:     conf.Logrus{},
		SectionUpload:  conf.Upload{},
		SectionDeco:    conf.Deco{},
	}
	for name, value := range cases {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("段 %s 无法序列化: %v", name, err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("段 %s 无法反序列化: %v", name, err)
		}
		for key := range m {
			if strings.ContainsAny(key, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
				t.Errorf("段 %s 的字段 %q 缺 json tag（合并键名会与库里的 snake_case 对不上）", name, key)
			}
		}
	}
}

// ── 第 8 段 deco：段注册的往返 ───────────────────────────────
//
// 这一组守的是"新增段却忘了加某个注册点"：漏掉段名常量 / 结构体字段 /
// 全量解码 / 缺失补种 / 缓存回写 / 空缓存默认 里任何一处，症状都是
// "后台改了没反应"或"读到零值"，而编译器一个字都不会说。

func TestUpdateDeco_落库并立刻生效(t *testing.T) {
	defaults := conf.RuntimeDefaults{
		Deco: conf.Deco{Charm: conf.Charm{
			Enable: true, LogoSize: 48,
			RopeLength: conf.CharmRopeLengthDefault, Segments: conf.CharmSegmentsDefault,
			LineWidth: conf.CharmLineWidthDefault, Gravity: conf.CharmGravityDefault,
			Damping:     conf.CharmDampingDefault,
			IdleSwayHz:  conf.CharmIdleSwayHzDefault,
			IdleSwayAmp: conf.CharmIdleSwayAmpDefault,
		}},
	}
	svc, db := newTestService(t, defaults)
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 1) 缺失补种：deco 段应有一行完整默认值（漏了 seedMissing 这里就取不到行）。
	// 这里同时钉一个新字段（rope_length）：票 #41 加字段时最容易漏的是
	// "结构体字段加了、yaml 默认值没加"，症状是默认补种成 0 后被钳成别的值。
	seeded := readPayload(t, db, SectionDeco)
	if !strings.Contains(seeded, `"enable":true`) || !strings.Contains(seeded, `"logo_size":48`) ||
		!strings.Contains(seeded, `"rope_length":120`) || !strings.Contains(seeded, `"line_width":2.5`) ||
		!strings.Contains(seeded, `"idle_sway_hz":0.8`) || !strings.Contains(seeded, `"idle_sway_amp":20`) {
		t.Fatalf("deco 段的默认值没补进库，实际 %s", seeded)
	}
	if got := svc.GetRuntimeDeco(); !got.Charm.Enable || got.Charm.LogoSize != 48 ||
		got.Charm.RopeLength != conf.CharmRopeLengthDefault ||
		got.Charm.LineWidth != conf.CharmLineWidthDefault ||
		got.Charm.IdleSwayHz != conf.CharmIdleSwayHzDefault ||
		got.Charm.IdleSwayAmp != conf.CharmIdleSwayAmpDefault {
		t.Fatalf("补种后应读到默认值，实际 %+v", got)
	}

	// 2) 缓存立刻生效（热更新：不需要重启）。
	if err := svc.UpdateDeco(conf.Deco{Charm: conf.Charm{
		Enable: false, LogoSize: 72,
		RopeLength: 200, Segments: 12, LineWidth: 4.5, Gravity: 2000, Damping: 0.98,
		IdleSwayHz: 0, IdleSwayAmp: 0,
	}}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if got := svc.GetRuntimeDeco(); got.Charm.Enable || got.Charm.LogoSize != 72 ||
		got.Charm.RopeLength != 200 || got.Charm.LineWidth != 4.5 ||
		got.Charm.IdleSwayHz != 0 || got.Charm.IdleSwayAmp != 0 {
		t.Fatalf("缓存未刷新，实际 %+v", got)
	}

	// 3) 库里也真的换了（不是只改内存）。
	payload := readPayload(t, db, SectionDeco)
	if !strings.Contains(payload, `"enable":false`) || !strings.Contains(payload, `"logo_size":72`) ||
		!strings.Contains(payload, `"rope_length":200`) || !strings.Contains(payload, `"line_width":4.5`) ||
		!strings.Contains(payload, `"idle_sway_amp":0`) {
		t.Fatalf("库里未写入新值，实际 %s", payload)
	}

	// 4) 换成新进程重新加载，值仍然在（重启不丢，且 decodeAll 认得这一段）。
	svc2 := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	if err := svc2.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if reloaded := svc2.GetRuntimeDeco(); reloaded.Charm.Enable || reloaded.Charm.LogoSize != 72 ||
		reloaded.Charm.RopeLength != 200 || reloaded.Charm.LineWidth != 4.5 ||
		reloaded.Charm.IdleSwayHz != 0 || reloaded.Charm.IdleSwayAmp != 0 {
		t.Fatalf("重新加载后应读到落库的值，实际 %+v", reloaded)
	}
}

func TestGetRuntimeDeco_未初始化时回退默认值(t *testing.T) {
	// 缓存为空（InitRuntimeConfig 还没跑）时 snapshotLocked 要按默认值拼一份。
	// 漏了这一处，只有这条路径读到的是零值 —— 表现是"站点刚起来时挂件消失"。
	svc := NewRuntimeConfigService(conf.RuntimeDefaults{
		Deco: conf.Deco{Charm: conf.Charm{Enable: true, LogoSize: 60}},
	}, quietLogger(), nil)

	if got := svc.GetRuntimeDeco(); !got.Charm.Enable || got.Charm.LogoSize != 60 {
		t.Fatalf("缓存为空时应回退默认值，实际 %+v", got)
	}
}

func TestDefaultSettingsYaml_装饰段与常量对齐(t *testing.T) {
	// 常量是写入路径的钳位边界，yaml 是"库里缺这段"时的补种值。
	// 两者漂移就会出现"后台没填过"和"填过默认值"渲染出不同尺寸。
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}

	var file struct {
		Deco conf.Deco `yaml:"deco"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	if !file.Deco.Charm.Enable {
		t.Fatal("deco.charm.enable 在默认值文件里应为 true（上线即可见，不用先去后台打开）")
	}
	if file.Deco.Charm.LogoSize != conf.CharmLogoSizeDefault {
		t.Fatalf("deco.charm.logo_size 应为 %v，实际 %v", conf.CharmLogoSizeDefault, file.Deco.Charm.LogoSize)
	}
	// 补种值本身必须落在钳位区间内，否则"默认值"会被 Normalize 改掉。
	if file.Deco.Charm.LogoSize < conf.CharmLogoSizeMin || file.Deco.Charm.LogoSize > conf.CharmLogoSizeMax {
		t.Fatalf("默认值 %v 不在 [%v, %v] 区间内", file.Deco.Charm.LogoSize, conf.CharmLogoSizeMin, conf.CharmLogoSizeMax)
	}

	// 票 #41 的四个物理参数同样要与常量对齐：它们不是"反正钳位会兜底"就可以
	// 留空的 —— yaml 里的默认值是补种进库的那一份，填错了库里的初始值就错了。
	if file.Deco.Charm.RopeLength != conf.CharmRopeLengthDefault {
		t.Fatalf("deco.charm.rope_length 应为 %v，实际 %v", conf.CharmRopeLengthDefault, file.Deco.Charm.RopeLength)
	}
	if file.Deco.Charm.Segments != conf.CharmSegmentsDefault {
		t.Fatalf("deco.charm.segments 应为 %v，实际 %v", conf.CharmSegmentsDefault, file.Deco.Charm.Segments)
	}
	if file.Deco.Charm.Gravity != conf.CharmGravityDefault {
		t.Fatalf("deco.charm.gravity 应为 %v，实际 %v", conf.CharmGravityDefault, file.Deco.Charm.Gravity)
	}
	if file.Deco.Charm.Damping != conf.CharmDampingDefault {
		t.Fatalf("deco.charm.damping 应为 %v，实际 %v", conf.CharmDampingDefault, file.Deco.Charm.Damping)
	}
	if file.Deco.Charm.RopeLength < conf.CharmRopeLengthMin || file.Deco.Charm.RopeLength > conf.CharmRopeLengthMax {
		t.Fatalf("rope_length 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.RopeLength, conf.CharmRopeLengthMin, conf.CharmRopeLengthMax)
	}
	if file.Deco.Charm.Segments < conf.CharmSegmentsMin || file.Deco.Charm.Segments > conf.CharmSegmentsMax {
		t.Fatalf("segments 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.Segments, conf.CharmSegmentsMin, conf.CharmSegmentsMax)
	}
	if file.Deco.Charm.Gravity < conf.CharmGravityMin || file.Deco.Charm.Gravity > conf.CharmGravityMax {
		t.Fatalf("gravity 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.Gravity, conf.CharmGravityMin, conf.CharmGravityMax)
	}
	if file.Deco.Charm.Damping < conf.CharmDampingMin || file.Deco.Charm.Damping > conf.CharmDampingMax {
		t.Fatalf("damping 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.Damping, conf.CharmDampingMin, conf.CharmDampingMax)
	}

	// 票 #42 的三个新参数：描边宽度、静止摆动频率与幅度。
	if file.Deco.Charm.LineWidth != conf.CharmLineWidthDefault {
		t.Fatalf("deco.charm.line_width 应为 %v，实际 %v", conf.CharmLineWidthDefault, file.Deco.Charm.LineWidth)
	}
	if file.Deco.Charm.LineWidth < conf.CharmLineWidthMin || file.Deco.Charm.LineWidth > conf.CharmLineWidthMax {
		t.Fatalf("line_width 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.LineWidth, conf.CharmLineWidthMin, conf.CharmLineWidthMax)
	}
	if file.Deco.Charm.IdleSwayHz != conf.CharmIdleSwayHzDefault {
		t.Fatalf("deco.charm.idle_sway_hz 应为 %v，实际 %v", conf.CharmIdleSwayHzDefault, file.Deco.Charm.IdleSwayHz)
	}
	if file.Deco.Charm.IdleSwayHz < conf.CharmIdleSwayHzMin || file.Deco.Charm.IdleSwayHz > conf.CharmIdleSwayHzMax {
		t.Fatalf("idle_sway_hz 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.IdleSwayHz, conf.CharmIdleSwayHzMin, conf.CharmIdleSwayHzMax)
	}
	if file.Deco.Charm.IdleSwayAmp != conf.CharmIdleSwayAmpDefault {
		t.Fatalf("deco.charm.idle_sway_amp 应为 %v，实际 %v", conf.CharmIdleSwayAmpDefault, file.Deco.Charm.IdleSwayAmp)
	}
	if file.Deco.Charm.IdleSwayAmp < conf.CharmIdleSwayAmpMin || file.Deco.Charm.IdleSwayAmp > conf.CharmIdleSwayAmpMax {
		t.Fatalf("idle_sway_amp 默认值 %v 不在 [%v, %v] 内", file.Deco.Charm.IdleSwayAmp, conf.CharmIdleSwayAmpMin, conf.CharmIdleSwayAmpMax)
	}
}

// ── 票 #60：删站点模式（ADR-0017） ───────────────────────────
//
// 站点模式是死配置：只有枚举声明、默认值、后台下拉框与两端手写类型，
// 全仓没有一处读它。它给管理员的是一个"看起来能改站点定位、实际什么都不改变"
// 的开关，所以整条链一起删。这里钉住的是**存储形状**（site 段不再有 mode），
// 而不是某个 Go 字段名 —— 字段名再改回来也一样会被拦住。

func TestSiteSection_序列化后不再有mode(t *testing.T) {
	raw, err := json.Marshal(conf.Site{})
	if err != nil {
		t.Fatalf("site 段无法序列化: %v", err)
	}
	var sections map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatalf("site 段无法反序列化: %v", err)
	}
	siteInfo, ok := sections["site_info"]
	if !ok {
		t.Fatal("site 段必须仍有 site_info 子块")
	}
	if _, hasMode := siteInfo["mode"]; hasMode {
		t.Fatal("site_info 不得再有 mode 键（站点模式已删除，见 ADR-0017）")
	}
}

func TestDefaultSettingsYaml_站点段不再有mode(t *testing.T) {
	// 默认值文件同样不能再写 mode：它就是补种进库的那一份，留着等于把死配置
	// 又种回每个新部署的环境。
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site struct {
			SiteInfo map[string]any `yaml:"site_info"`
		} `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}
	if _, hasMode := file.Site.SiteInfo["mode"]; hasMode {
		t.Fatalf("site_default_settings.yaml 的 site.site_info 不得再写 mode，实际 %v", file.Site.SiteInfo)
	}
}

func TestUpdateSite_旧mode残留不报错且保存后消失(t *testing.T) {
	// 库里已存的 site 段可能还留着 "mode": 1。ADR-0017 明确不做数据迁移：
	// 深度合并会把它留在 map 里但不再进类型，管理员下次保存整段即自然消失。
	// 两个方向都要钉：读不能报错（否则老部署升级后站点配置整段回退默认值），
	// 保存要把残留键挤掉（否则"删了配置"只在代码里成立）。
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{SiteInfo: siteconf.SiteInfo{Title: "默认标题"}},
	}
	svc, db := newTestService(t, defaults)

	legacy := `{"site_info":{"title":"老站点","mode":1},"article":{"skip_examining":true}}`
	if err := upsertPayload(db, SectionSite, legacy); err != nil {
		t.Fatalf("预置老载荷失败: %v", err)
	}
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("带残留 mode 的老载荷必须能正常加载，实际报错: %v", err)
	}
	if got := svc.GetRuntimeSite().SiteInfo.Title; got != "老站点" {
		t.Fatalf("载荷里的站点标题应生效，实际 %q", got)
	}

	site := svc.GetRuntimeSite()
	site.SiteInfo.Beian = "京ICP备00000000号"
	if err := svc.UpdateSite(site); err != nil {
		t.Fatalf("保存站点配置失败: %v", err)
	}
	if payload := readPayload(t, db, SectionSite); strings.Contains(payload, `"mode"`) {
		t.Fatalf("管理员保存一次后 site 段不应再含 mode，实际 %s", payload)
	}
}

// ── 票 #60：发布与展示三开关（ADR-0017 决策 2） ──────────────
//
// 三个布尔都在 site.article 段、默认 true，后台同一张「发布与展示」卡维护。
// 默认值只在 site_default_settings.yaml 里写一份（深度合并负责把老库缺的键补上），
// 所以老库不需要迁移、后台打开就是"全开"，而不是三个看起来被关掉的开关。

func TestDefaultSettingsYaml_发布展示开关默认全开(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site conf.Site `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	article := file.Site.Article
	if !article.AllowNonAdminPublish {
		t.Error("site.article.allow_non_admin_publish 默认值应为 true（关掉等于非管理员一律不能发文）")
	}
	if !article.AllowNonAdminDraft {
		t.Error("site.article.allow_non_admin_draft 默认值应为 true（关掉等于非管理员连草稿都存不了）")
	}
	if !article.ShowNonAdminArticles {
		t.Error("site.article.show_non_admin_articles 默认值应为 true（关掉等于读者站看不到非管理员文章）")
	}
}

func TestDecodeSection_发布展示开关缺字段回退默认值(t *testing.T) {
	// 老库里的 site.article 只有 skip_examining：三个新开关必须从默认值补齐。
	defaults := conf.Site{Article: siteconf.Article{
		AllowNonAdminPublish: true,
		AllowNonAdminDraft:   true,
		ShowNonAdminArticles: true,
	}}
	payloads := map[string]string{
		SectionSite: `{"article":{"skip_examining":true}}`,
	}

	got, err := decodeSection(SectionSite, defaults, payloads)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !got.Article.AllowNonAdminPublish || !got.Article.AllowNonAdminDraft || !got.Article.ShowNonAdminArticles {
		t.Fatalf("缺这三个键时应回退默认值 true，实际 %+v", got.Article)
	}
	if !got.Article.SkipExamining {
		t.Fatal("同段内已有的 skip_examining 应保留")
	}
}

func TestUpdateSite_发布展示开关关闭后重新加载仍是关闭(t *testing.T) {
	// 「保存后重新加载仍是所选值」是验收标准：关掉三个开关 → 落库 → 换一个
	// 进程（空默认值）重新加载 → 读到的还是关闭。用空默认值是为了证明值来自
	// 库里那份载荷，而不是又从默认值补了一遍。
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{Article: siteconf.Article{
			AllowNonAdminPublish: true,
			AllowNonAdminDraft:   true,
			ShowNonAdminArticles: true,
		}},
	}
	svc, db := newTestService(t, defaults)
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	if got := svc.GetRuntimeArticle(); !got.AllowNonAdminPublish || !got.ShowNonAdminArticles {
		t.Fatalf("补种后应是默认值全开，实际 %+v", got)
	}

	site := svc.GetRuntimeSite()
	site.Article.AllowNonAdminPublish = false
	site.Article.AllowNonAdminDraft = false
	site.Article.ShowNonAdminArticles = false
	if err := svc.UpdateSite(site); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	payload := readPayload(t, db, SectionSite)
	for _, key := range []string{"allow_non_admin_publish", "allow_non_admin_draft", "show_non_admin_articles"} {
		// 键名必须与 json tag 一致：合并靠 JSON 往返，写成别的名字就是
		// "后台改了没反应"，且编译器不会说话。
		if !strings.Contains(payload, `"`+key+`":false`) {
			t.Fatalf("site 段里 %s 应为 false，实际 %s", key, payload)
		}
	}

	svc2 := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	if err := svc2.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if got := svc2.GetRuntimeArticle(); got.AllowNonAdminPublish || got.AllowNonAdminDraft || got.ShowNonAdminArticles {
		t.Fatalf("重新加载后三开关应仍为 false，实际 %+v", got)
	}
}
