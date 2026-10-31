package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"

	"gopkg.in/yaml.v3"
)

// 站点段与「发布与展示」三开关的默认值/存储形状（ADR-0017 决策 1–2）。
// 拆成独立文件是为了守住 R6：单文件不得超过 800 行。

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
