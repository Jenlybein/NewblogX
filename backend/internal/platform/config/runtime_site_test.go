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

// ── 旧右栏配置（index_right）下线 ───────────────────────────
//
// `site.index_right` 是只有"标题 + 启用"两字段的半残配置：前端只把标题当
// 模块 key、完全忽略启用位，后端不校验也不认识这些 key。右栏改由卡片表驱动后，
// 它必须整条链删掉，否则运营改新配置、旧配置还在，调试时分不清谁生效。
//
// 零 DDL：老库里可能还留着这段 JSON。读取必须容忍（未知键被 JSON 解码忽略），
// 管理员下次保存整段时残留键被挤出类型，自然消失。

func TestSiteSection_序列化后不再有index_right(t *testing.T) {
	raw, err := json.Marshal(conf.Site{})
	if err != nil {
		t.Fatalf("site 段无法序列化: %v", err)
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatalf("site 段无法反序列化: %v", err)
	}
	if _, has := sections["index_right"]; has {
		t.Fatal("site 段不得再序列化出 index_right（旧右栏配置已下线）")
	}
}

func TestDefaultSettingsYaml_站点段不再有index_right(t *testing.T) {
	// 默认值文件就是补种进库的那一份；留着等于把废弃配置又种回每个新部署。
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site map[string]any `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}
	if _, has := file.Site["index_right"]; has {
		t.Fatalf("site_default_settings.yaml 的 site 段不得再写 index_right，实际 %v", file.Site)
	}
}

func TestUpdateSite_旧index_right残留不报错且保存后消失(t *testing.T) {
	// 与 ADR-0017 删站点模式同一口径：不做数据迁移。
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{Project: siteconf.Project{Title: "默认标题"}},
	}
	svc, db := newTestService(t, defaults)

	legacy := `{"project":{"title":"老站点"},"index_right":{"list":[{"title":"hot_tags","enable":true}]}}`
	if err := upsertPayload(db, SectionSite, legacy); err != nil {
		t.Fatalf("预置老载荷失败: %v", err)
	}
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("带残留 index_right 的老载荷必须能正常加载，实际报错: %v", err)
	}
	if got := svc.GetRuntimeSite().Project.Title; got != "老站点" {
		t.Fatalf("载荷里的项目标题应生效，实际 %q", got)
	}

	site := svc.GetRuntimeSite()
	site.Project.WebPath = "/"
	if err := svc.UpdateSite(site); err != nil {
		t.Fatalf("保存站点配置失败: %v", err)
	}
	if payload := readPayload(t, db, SectionSite); strings.Contains(payload, "index_right") {
		t.Fatalf("管理员保存一次后 site 段不应再含 index_right，实际 %s", payload)
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

// ── #202：陌生人私信开关（site.chat 段） ──────────────────────
//
// 这个开关是**站点级**策略：它决定"陌生人能不能给任何人发私信"，
// 不落在任何用户行上（零 DDL 也不允许给既有表加列）。
// 缺省行为必须是**显式写下来的「关」**：如果只靠"配置里没有这个键"
// 隐式得到 false，那么将来有人把默认值改成 true（或把键删掉）时，
// 语义会静默翻转，而没有任何测试会红。

func TestDefaultSettingsYaml_陌生人私信开关显式关闭(t *testing.T) {
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
	if file.Site.Chat.StrangerMsgEnabled {
		t.Fatal("site.chat.stranger_msg_enabled 默认值应为 false（陌生人私信默认关闭）")
	}
	// 仅解析出 false 还不够：缺键同样解析成 false。必须能在原文里看到
	// 这个键被**显式**写成 false，否则"默认关"就是隐式的。
	if !strings.Contains(string(raw), "stranger_msg_enabled: false") {
		t.Fatalf("site_default_settings.yaml 必须显式写 stranger_msg_enabled: false，实际内容里没有这一行")
	}
}

func TestDecodeSection_老载荷没有陌生人开关时按默认值补齐(t *testing.T) {
	// 老库里的 site 段完全没有 chat 子块：新开关必须从默认值补齐。
	defaults := conf.Site{Chat: siteconf.Chat{StrangerMsgEnabled: true}}
	got, err := decodeSection(SectionSite, defaults, map[string]string{
		SectionSite: `{"site_info":{"title":"老站点"},"article":{"skip_examining":true}}`,
	})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !got.Chat.StrangerMsgEnabled {
		t.Fatal("载荷里没有 chat 子块时应回退默认值 true")
	}
}

func TestUpdateSite_陌生人开关保存后立即生效且重新加载仍是所选值(t *testing.T) {
	// 「保存后立即生效」= UpdateSite 返回后缓存已经是新值（读者下一次发送就该看到）。
	// 「重新加载仍是所选值」= 值真的落进了库里那一行，而不是只活在缓存里。
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{Chat: siteconf.Chat{StrangerMsgEnabled: false}},
	}
	svc, db := newTestService(t, defaults)
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	if got := svc.GetRuntimeSite().Chat.StrangerMsgEnabled; got {
		t.Fatalf("初始应为默认的关闭，实际 %v", got)
	}

	site := svc.GetRuntimeSite()
	site.Chat.StrangerMsgEnabled = true
	if err := svc.UpdateSite(site); err != nil {
		t.Fatalf("保存站点配置失败: %v", err)
	}
	if !svc.GetRuntimeSite().Chat.StrangerMsgEnabled {
		t.Fatal("保存后应立即生效（缓存未刷新）")
	}
	payload := readPayload(t, db, SectionSite)
	if !strings.Contains(payload, `"stranger_msg_enabled":true`) {
		t.Fatalf("site 段里 stranger_msg_enabled 应为 true，实际 %s", payload)
	}

	svc2 := NewRuntimeConfigService(conf.RuntimeDefaults{}, quietLogger(), db)
	if err := svc2.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if !svc2.GetRuntimeSite().Chat.StrangerMsgEnabled {
		t.Fatal("重新加载后陌生人私信开关应仍为 true")
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
