package app_test

import (
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/apperr"
	siteapp "myblogx/internal/site/app"
	"myblogx/internal/site/apptest"
	"myblogx/internal/site/domain"
	"testing"
)

// newEnv 构造一套带默认站点配置的替身。
func newEnv() *apptest.Env {
	site := conf.Site{
		SiteInfo: confsite.SiteInfo{Beian: "粤ICP备12345678号"},
		Project:  confsite.Project{Title: "项目标题", Icon: "/favicon.ico"},
		Seo:      confsite.Seo{Keywords: "go,blog", Description: "站点描述"},
	}
	ai := conf.AI{
		Enable:     true,
		TimeoutSec: 60,
		Nickname:   "AI 助手",
		Avatar:     "/ai.png",
		Abstract:   "你好",
	}
	return apptest.NewEnv(site, ai, "v1.2.3", "https://graph.qq.com/oauth2.0/show?client_id=x")
}

func TestPublicViews(t *testing.T) {
	env := newEnv()

	if got := env.Service.PublicSite(); got.About.Version != "v1.2.3" || got.SiteInfo.Beian != "粤ICP备12345678号" {
		t.Fatalf("站点信息错误: %+v", got)
	}
	seo := env.Service.PublicSEO()
	if seo.ProjectTitle != "项目标题" || seo.Description != "站点描述" || seo.Keywords != "go,blog" {
		t.Fatalf("SEO 信息错误: %+v", seo)
	}
	// favicon 仍走站点配置里的项目图标（站点 Logo 已删除，图标不删）。
	if seo.Icon != "/favicon.ico" {
		t.Fatalf("SEO 图标应来自 project.icon，得到 %q", seo.Icon)
	}
	ai := env.Service.PublicAI()
	if !ai.Enable || ai.Nickname != "AI 助手" {
		t.Fatalf("AI 信息错误: %+v", ai)
	}
	if env.Service.QQLoginURL() == "" {
		t.Fatal("QQ 登录地址不应为空")
	}
}

func TestAdminViewsMaskSecret(t *testing.T) {
	env := newEnv()
	// AI 段已经不含密钥了（模型密钥搬到 ai_provider_models 表，由 AI 模块自己
	// 的接口遮蔽），所以这里只确认它仍原样透传、公开接口也照常。
	if got := env.Service.AdminAI().Nickname; got != "AI 助手" {
		t.Fatalf("管理端 AI 配置应有昵称，得到 %q", got)
	}
	if env.Service.PublicAI().Nickname == "" {
		t.Fatal("公开 AI 信息应包含昵称")
	}
	if got := env.Service.AdminMail().SecretKey; got != domain.SensitivePlaceholder {
		t.Fatalf("邮件密钥仍应遮蔽，得到 %s", got)
	}
}

func TestUpdateAIUpdatesSiteLevelSettings(t *testing.T) {
	env := newEnv()
	err := env.Service.UpdateAI(conf.AI{
		Enable:     true,
		Nickname:   "新助手",
		TimeoutSec: 30,
	})
	if err != nil {
		t.Fatalf("更新 AI 配置失败: %v", err)
	}
	current := env.Runtime.AI()
	if current.Nickname != "新助手" || current.TimeoutSec != 30 {
		t.Fatalf("AI 配置未更新: %+v", current)
	}
}

func TestUpdateSitePersists(t *testing.T) {
	env := newEnv()
	site := env.Service.PublicSite()
	site.SiteInfo.Beian = "京ICP备00000000号"
	site.Seo.Description = "新的描述"
	if err := env.Service.UpdateSite(site); err != nil {
		t.Fatalf("更新站点配置失败: %v", err)
	}
	if got := env.Runtime.Site().SiteInfo.Beian; got != "京ICP备00000000号" {
		t.Fatalf("备案号未更新，得到 %s", got)
	}
	if got := env.Runtime.Site().Seo.Description; got != "新的描述" {
		t.Fatalf("站点描述未更新，得到 %s", got)
	}
}

// 标签上限越界必须**否决保存**，而不是静默回落。
//
// 两条读面（后台表单 / 读者站写稿页）都按这个值显示"最多选几个"，
// 而真正拦人的后端会把越界值回落成默认 5 —— 存进来的 25 一旦被读出去，
// 作者就会选到"界面上合法、提交却被拒"的数量。
func TestUpdateSiteRejectsArticleMaxTagsOutOfRange(t *testing.T) {
	for _, limit := range []int{-1, confsite.MaxArticleMaxTags + 1, 9999} {
		env := newEnv()
		site := env.Service.PublicSite()
		site.Article.MaxTagsPerArticle = limit
		err := env.Service.UpdateSite(site)
		if err == nil {
			t.Fatalf("标签上限 %d 应当被拒绝", limit)
		}
		if !apperr.IsCode(err, apperr.CodeSiteArticleInvalid) {
			t.Fatalf("标签上限 %d 的错误码应为 site.article_invalid，得到 %v", limit, err)
		}
		// 拒绝之后库里不能留下这个值。
		if got := env.Runtime.Site().Article.MaxTagsPerArticle; got == limit {
			t.Fatalf("被拒绝的值 %d 仍被写入", limit)
		}
	}
}

// 合法区间内（含两端）必须原样保存 —— 区间边界是最容易被写错的地方。
func TestUpdateSiteAcceptsArticleMaxTagsInRange(t *testing.T) {
	// 0 也在这条里：它表示"没配过 → 用默认上限"，整段保存不能被它否决。
	for _, limit := range []int{0, confsite.MinArticleMaxTags, 5, confsite.MaxArticleMaxTags} {
		env := newEnv()
		site := env.Service.PublicSite()
		site.Article.MaxTagsPerArticle = limit
		if err := env.Service.UpdateSite(site); err != nil {
			t.Fatalf("标签上限 %d 应当被接受，得到 %v", limit, err)
		}
		if got := env.Runtime.Site().Article.MaxTagsPerArticle; got != limit {
			t.Fatalf("标签上限未保存：期望 %d，得到 %d", limit, got)
		}
	}
}

func TestAdminViewsMaskNewSecrets(t *testing.T) {
	env := newEnv()
	if err := env.Service.UpdateMail(conf.Mail{SecretID: "id-1", SecretKey: "mail-secret"}); err != nil {
		t.Fatalf("更新邮件配置失败: %v", err)
	}
	if err := env.Service.UpdateStorage(conf.QiNiu{AccessKey: "ak", SecretKey: "qiniu-secret"}); err != nil {
		t.Fatalf("更新存储配置失败: %v", err)
	}
	if err := env.Service.UpdateOAuth(conf.QQ{AppID: "123", AppKey: "qq-secret"}); err != nil {
		t.Fatalf("更新 QQ 配置失败: %v", err)
	}

	if got := env.Service.AdminMail().SecretKey; got != domain.SensitivePlaceholder {
		t.Fatalf("邮件密钥应被遮蔽，得到 %s", got)
	}
	if got := env.Service.AdminStorage().SecretKey; got != domain.SensitivePlaceholder {
		t.Fatalf("七牛密钥应被遮蔽，得到 %s", got)
	}
	if got := env.Service.AdminOAuth().AppKey; got != domain.SensitivePlaceholder {
		t.Fatalf("QQ AppKey 应被遮蔽，得到 %s", got)
	}

	// 非密钥字段要如实回显，否则管理员没法确认自己填的是哪个号的密钥。
	if got := env.Service.AdminMail().SecretID; got != "id-1" {
		t.Fatalf("SecretID 不是密钥，应如实回显，得到 %s", got)
	}
	if got := env.Service.AdminStorage().AccessKey; got != "ak" {
		t.Fatalf("AccessKey 应如实回显，得到 %s", got)
	}
}

func TestUpdateNewSectionsKeepSecretOnPlaceholder(t *testing.T) {
	// 前端每次保存都会把回显的占位符原样提交回来。
	// 若不识别占位符，密钥就会被写成 "******"，下一次发信/上传直接失败。
	env := newEnv()

	if err := env.Service.UpdateMail(conf.Mail{SecretKey: "mail-secret", Region: "ap-guangzhou"}); err != nil {
		t.Fatalf("更新邮件配置失败: %v", err)
	}
	if err := env.Service.UpdateMail(conf.Mail{SecretKey: domain.SensitivePlaceholder, Region: "ap-hongkong"}); err != nil {
		t.Fatalf("更新邮件配置失败: %v", err)
	}
	if got := env.Runtime.Mail(); got.SecretKey != "mail-secret" || got.Region != "ap-hongkong" {
		t.Fatalf("占位符应保留原密钥、其余字段应更新，得到 %+v", got)
	}

	if err := env.Service.UpdateStorage(conf.QiNiu{SecretKey: "qiniu-secret", Bucket: "b1"}); err != nil {
		t.Fatalf("更新存储配置失败: %v", err)
	}
	if err := env.Service.UpdateStorage(conf.QiNiu{SecretKey: domain.SensitivePlaceholder, Bucket: "b2"}); err != nil {
		t.Fatalf("更新存储配置失败: %v", err)
	}
	if got := env.Runtime.Storage(); got.SecretKey != "qiniu-secret" || got.Bucket != "b2" {
		t.Fatalf("占位符应保留原密钥、其余字段应更新，得到 %+v", got)
	}
}

func TestUpdateMailClearsSecretOnEmptyString(t *testing.T) {
	// 空字符串与占位符语义不同：空字符串表示"我确实要清掉密钥"，
	// 否则管理员没办法主动撤销一个已经泄露的密钥。
	env := newEnv()
	if err := env.Service.UpdateMail(conf.Mail{SecretKey: "mail-secret"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if err := env.Service.UpdateMail(conf.Mail{SecretKey: ""}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if got := env.Runtime.Mail().SecretKey; got != "" {
		t.Fatalf("空字符串应能清空密钥，得到 %q", got)
	}
}

func TestUpdateLogPreservesProcessLevelFields(t *testing.T) {
	// 日志目录/服务名/输出格式由部署时的 settings.yaml 决定。
	// 允许后台改它们会造成两种事故：清理任务按旧目录算保留窗口、
	// 或者目录不存在导致写入失败且失败日志一起消失。
	env := newEnv()
	// 进程级字段由启动时从 settings.yaml 播种（见 core.InitRuntimeSite）。
	// 这里直接写替身来模拟"进程刚起来"的状态 —— 走服务方法的话，
	// 这几个字段会被保护逻辑覆盖成"当前值"，而当前值此刻还是空的。
	base := conf.Logrus{
		App:          "blogx-api",
		Dir:          "/app/logs",
		WALDir:       "/app/logs/wal",
		Level:        "info",
		StdoutFormat: "json",
	}
	if err := env.Runtime.UpdateLog(base); err != nil {
		t.Fatalf("播种日志配置失败: %v", err)
	}

	// 管理员提交了被禁止修改的字段。
	if err := env.Service.UpdateLog(conf.Logrus{
		App:               "hacked",
		Dir:               "/tmp/elsewhere",
		WALDir:            "/tmp/wal",
		StdoutFormat:      "text",
		Level:             "debug",
		RequestLogEnabled: true,
	}); err != nil {
		t.Fatalf("更新日志配置失败: %v", err)
	}

	got := env.Runtime.Log()
	if got.Dir != "/app/logs" || got.WALDir != "/app/logs/wal" || got.App != "blogx-api" || got.StdoutFormat != "json" {
		t.Fatalf("进程级字段不该被后台改掉，得到 %+v", got)
	}
	if got.Level != "debug" || !got.RequestLogEnabled {
		t.Fatalf("可改字段应生效，得到 %+v", got)
	}
}

// 白名单的整体替换语义随 upload 段一起搬到 storage 段（票 418）：
// 现在由 TestUpdateStorage 的 whitelist 断言覆盖，这里不再重复。

// ── deco 段（装饰，九段之一） ────────────────────────────────────────

func TestUpdateDecoClampsIntoRange(t *testing.T) {
	// 钳位规则本身在 conf.Deco.Normalize，但必须由服务层调用：
	// 忘了这一步，后台就能存进 4000，读者站按 4000 渲染。
	// 关键点：越界**不报错**，否则管理员改一半会被 400 打回、却不知道哪项错了。
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"远超上限钳到 96", 4000, conf.CharmLogoSizeMax},
		{"略低于下限钳到 24", 10, conf.CharmLogoSizeMin},
		{"没填（零值）回落默认 48", 0, conf.CharmLogoSizeDefault},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newEnv()
			if err := env.Service.UpdateDeco(conf.Deco{Charm: conf.Charm{Enable: true, LogoSize: c.in}}); err != nil {
				t.Fatalf("越界值不该报错（钳位而不是 400）: %v", err)
			}
			if got := env.Runtime.Deco().Charm.LogoSize; got != c.want {
				t.Fatalf("落库值应为 %v，实际 %v", c.want, got)
			}
			if !env.Runtime.Deco().Charm.Enable {
				t.Fatal("钳位不应顺手关掉总开关")
			}
		})
	}
}

func TestUpdateDecoClampsPhysicsParams(t *testing.T) {
	// 服务层必须把四个物理参数也交给 Normalize —— 只钳站标尺寸是票 #40 的
	// 半成品形态；漏掉任何一个，后台就能存进 4000px 的绳子或阻尼 2
	// （后者会让摆动越甩越大，是"发散值"而不是"难看值"）。
	env := newEnv()
	if err := env.Service.UpdateDeco(conf.Deco{Charm: conf.Charm{
		Enable:     true,
		RopeLength: 4000,
		Segments:   100,
		Gravity:    100000,
		Damping:    2,
	}}); err != nil {
		t.Fatalf("越界值不该报错（钳位而不是 400）: %v", err)
	}

	got := env.Runtime.Deco().Charm
	if got.RopeLength != conf.CharmRopeLengthMax {
		t.Fatalf("rope_length 应钳到 %v，实际 %v", conf.CharmRopeLengthMax, got.RopeLength)
	}
	if got.Segments != conf.CharmSegmentsMax {
		t.Fatalf("segments 应钳到 %v，实际 %v", conf.CharmSegmentsMax, got.Segments)
	}
	if got.Gravity != conf.CharmGravityMax {
		t.Fatalf("gravity 应钳到 %v，实际 %v", conf.CharmGravityMax, got.Gravity)
	}
	if got.Damping != conf.CharmDampingMax {
		t.Fatalf("damping 应钳到 %v，实际 %v", conf.CharmDampingMax, got.Damping)
	}
}

func TestDecoReadsShareOneBlock(t *testing.T) {
	// 装饰配置没有任何敏感字段，所以公开读与管理读返回同一份。
	// 这条断言保证两条路由都接上了 deco 分支（漏一个就有一个读到零值）。
	env := newEnv()
	if err := env.Service.UpdateDeco(conf.Deco{Charm: conf.Charm{Enable: false, LogoSize: 30}}); err != nil {
		t.Fatalf("更新装饰配置失败: %v", err)
	}

	pub := env.Service.PublicDeco()
	adm := env.Service.AdminDeco()
	if pub.Charm.Enable || pub.Charm.LogoSize != 30 {
		t.Fatalf("公开读应拿到刚存的装饰配置，得到 %+v", pub)
	}
	if adm != pub {
		t.Fatalf("管理读与公开读应一致，得到 %+v 与 %+v", adm, pub)
	}
}

// ── content_filter 段（第 11 段，ADR-0032）────────────────────────────

// 保存前整理词条：去空白、去重、校验档位/类目/单字词；校验失败不落库。
func TestUpdateContentFilterNormalizesAndRejectsInvalid(t *testing.T) {
	env := newEnv()

	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{
			{Word: "  刷单  ", Tier: "watch", Category: "fraud"},
			{Word: "刷单", Tier: "watch", Category: "fraud"},
			{Word: "   ", Tier: "watch", Category: "fraud"},
		},
		Whitelist:        []string{"  ", "测试"},
		WhitelistPhrases: []string{" 安全套接层 ", "", "安全套接层"},
	}); err != nil {
		t.Fatalf("保存词表失败: %v", err)
	}
	got := env.Runtime.ContentFilter()
	if len(got.Words) != 1 || got.Words[0].Word != "刷单" {
		t.Fatalf("应只剩去空白后的 1 条词，得到 %+v", got.Words)
	}
	if len(got.Whitelist) != 1 || got.Whitelist[0] != "测试" {
		t.Fatalf("白名单应只剩 1 条，得到 %+v", got.Whitelist)
	}
	// 短语白名单同样要去空白去重：否则跨表重复会让体检里的三个数字对不上，
	// 也让引擎收到带空格的条目（永远匹配不上，静默失效）。
	if len(got.WhitelistPhrases) != 1 || got.WhitelistPhrases[0] != "安全套接层" {
		t.Fatalf("短语白名单应只剩去空白后的 1 条，得到 %+v", got.WhitelistPhrases)
	}

	// 校验失败必须拒绝保存：坏词条写进库里只会在启动构建引擎时炸。
	invalid := map[string]conf.ContentFilterWord{
		"未知档位": {Word: "冰毒", Tier: "blocked", Category: "drugs"},
		"未知类目": {Word: "冰毒", Tier: "block", Category: "weapons"},
		"单字词":  {Word: "毒", Tier: "block", Category: "drugs"},
	}
	for name, word := range invalid {
		if err := env.Service.UpdateContentFilter(conf.ContentFilter{
			Words: []conf.ContentFilterWord{word},
		}); err == nil {
			t.Fatalf("%s 应当拒绝保存", name)
		}
	}
	if after := env.Runtime.ContentFilter(); len(after.Words) != 1 || after.Words[0].Word != "刷单" {
		t.Fatalf("校验失败不应改动已保存的词表，得到 %+v", after.Words)
	}
}

// 保存成功后回调一次（装配层把过滤引擎的重建挂在这里）；校验失败不回调。
func TestUpdateContentFilterNotifiesEngineOnSuccess(t *testing.T) {
	env := newEnv()
	var got []conf.ContentFilter
	env.Service.SetContentFilterChangedHook(func(contentFilter conf.ContentFilter) {
		got = append(got, contentFilter)
	})

	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
	}); err != nil {
		t.Fatalf("保存词表失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("保存成功后应回调一次，实际 %d 次", len(got))
	}
	if len(got[0].Words) != 1 || got[0].Words[0].Word != "冰毒" {
		t.Fatalf("回调应拿到整理后的词表，得到 %+v", got[0].Words)
	}

	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{{Word: "毒", Tier: "block", Category: "drugs"}},
	}); err == nil {
		t.Fatal("单字词应被拒绝")
	}
	if len(got) != 1 {
		t.Fatalf("校验失败不应触发回调，实际 %d 次", len(got))
	}
}

// 保存词表时请求体里**没有** auto_dispose（旧前端 / 老客户端）→ 落库是 mask/mask。
//
// 这是"手写字面量清零"的回归用例：normalizeContentFilter 返回的是逐字段手写的
// conf.ContentFilter，漏写 AutoDispose 就把零值（两个空串）存进库 —— 不报错、
// 不红测试，只在下一次读配置时才看出"自动处置莫名其妙变空了"。
// 断言 mask/mask 而不是空串，是因为空串虽然按 mask 解释，但后台下拉框会显示空，
// 运营看到的是"没配置"，与"配了 mask"不是一回事。
func TestUpdateContentFilterDefaultsAutoDisposeWhenBodyOmitsIt(t *testing.T) {
	env := newEnv()

	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
	}); err != nil {
		t.Fatalf("保存词表失败: %v", err)
	}
	got := env.Runtime.ContentFilter().AutoDispose
	want := conf.ContentFilterAutoDispose{
		BlockAction: siteapp.ContentFilterActionMask,
		WatchAction: siteapp.ContentFilterActionMask,
	}
	if got != want {
		t.Fatalf("请求体缺 auto_dispose 时应落库 mask/mask，实际 %+v", got)
	}
}

// 显式配 reject 要原样存下来（GET 回显与下一次处置都靠它），并且再存一次词表
// **带着**这个值时要保持不变。
func TestUpdateContentFilterKeepsExplicitReject(t *testing.T) {
	env := newEnv()

	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		AutoDispose: conf.ContentFilterAutoDispose{
			BlockAction: siteapp.ContentFilterActionReject,
			WatchAction: siteapp.ContentFilterActionMask,
		},
	}); err != nil {
		t.Fatalf("保存词表失败: %v", err)
	}
	got := env.Runtime.ContentFilter().AutoDispose
	if got.BlockAction != siteapp.ContentFilterActionReject || got.WatchAction != siteapp.ContentFilterActionMask {
		t.Fatalf("reject 应原样落库，实际 %+v", got)
	}

	// 再存一次（带上同一个口径）：不能被"手写字面量"洗回默认值。
	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words:       []conf.ContentFilterWord{{Word: "枪支", Tier: "block", Category: "violence"}},
		AutoDispose: got,
	}); err != nil {
		t.Fatalf("二次保存词表失败: %v", err)
	}
	if after := env.Runtime.ContentFilter().AutoDispose; after != got {
		t.Fatalf("二次保存不该改动处置口径，期望 %+v，实际 %+v", got, after)
	}
}

// 处置口径的非法取值必须当场拒绝：写进库里只会让处置链路读到一条读不懂的口径。
func TestUpdateContentFilterRejectsInvalidAutoDispose(t *testing.T) {
	env := newEnv()

	invalid := map[string]conf.ContentFilterAutoDispose{
		"block_action 未知": {BlockAction: "blocked", WatchAction: "mask"},
		"watch_action 未知": {BlockAction: "mask", WatchAction: "warn"},
		"大小写不对":           {BlockAction: "Reject", WatchAction: "mask"},
	}
	for name, dispose := range invalid {
		err := env.Service.UpdateContentFilter(conf.ContentFilter{
			Words:       []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
			AutoDispose: dispose,
		})
		if err == nil {
			t.Fatalf("%s 应当拒绝保存", name)
		}
		if !apperr.IsCode(err, apperr.CodeSiteContentFilterInvalid) {
			t.Fatalf("%s 应报 site.content_filter_invalid，实际 %v", name, err)
		}
	}
	// 校验失败不能改动已保存的配置（env 是干净的，所以仍是零值）。
	if after := env.Runtime.ContentFilter(); len(after.Words) != 0 {
		t.Fatalf("校验失败不应改动已保存的词表，得到 %+v", after.Words)
	}

	// 空串是合法值（= mask），不能当非法值拒 —— 旧前端缺字段要能存。
	if err := env.Service.UpdateContentFilter(conf.ContentFilter{
		Words:       []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		AutoDispose: conf.ContentFilterAutoDispose{},
	}); err != nil {
		t.Fatalf("空串口径应按 mask 接受，实际被拒: %v", err)
	}
}
