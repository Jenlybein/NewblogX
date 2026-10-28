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
	if count != 7 {
		t.Fatalf("七段都应补一行，实际 %d 行", count)
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
		SectionStorage, SectionOAuth, SectionUpload,
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
