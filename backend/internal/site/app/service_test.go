package app_test

import (
	"context"
	"testing"

	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/app"
	"myblogx/internal/site/apptest"
	"myblogx/internal/site/domain"
)

// newEnv 构造一套带默认站点配置的替身。
func newEnv() *apptest.Env {
	site := conf.Site{
		SiteInfo: confsite.SiteInfo{Title: "技术博客", Logo: "/logo.png"},
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

	if got := env.Service.PublicSite(); got.About.Version != "v1.2.3" || got.SiteInfo.Title != "技术博客" {
		t.Fatalf("站点信息错误: %+v", got)
	}
	seo := env.Service.PublicSEO()
	if seo.ProjectTitle != "项目标题" || seo.Description != "站点描述" || seo.Keywords != "go,blog" {
		t.Fatalf("SEO 信息错误: %+v", seo)
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
	site.SiteInfo.Title = "新站点"
	site.Seo.Description = "新的描述"
	if err := env.Service.UpdateSite(site); err != nil {
		t.Fatalf("更新站点配置失败: %v", err)
	}
	if got := env.Runtime.Site().SiteInfo.Title; got != "新站点" {
		t.Fatalf("站点标题未更新，得到 %s", got)
	}
	if got := env.Runtime.Site().Seo.Description; got != "新的描述" {
		t.Fatalf("站点描述未更新，得到 %s", got)
	}
}

func TestBannerLifecycle(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	env.Images.Put(12, "/b.png")

	created, err := env.Service.CreateBanner(ctx, apptestInput(11, "/a", true))
	if err != nil {
		t.Fatalf("创建轮播图失败: %v", err)
	}
	if created.Cover != "/a.png" || created.ID.IsZero() {
		t.Fatalf("创建结果错误: %+v", created)
	}

	if _, err := env.Service.CreateBanner(ctx, apptestInput(12, "/b", true)); err != nil {
		t.Fatalf("创建第二张轮播图失败: %v", err)
	}

	rows, hasMore, err := env.Service.ListBanners(ctx, domain.BannerQuery{Show: true})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 2 || hasMore {
		t.Fatalf("展示中的轮播图应有 2 条，得到 %d hasMore=%v", len(rows), hasMore)
	}
	if rows[0].CoverImageID.IsZero() {
		t.Fatalf("列表应回填封面图片 ID: %+v", rows[0])
	}

	// 隐藏后不再出现在 show=true 的列表里。
	if err := env.Service.UpdateBanner(ctx, created.ID.Uint64(), apptestInput(12, "/b2", false)); err != nil {
		t.Fatalf("更新轮播图失败: %v", err)
	}
	rows, _, err = env.Service.ListBanners(ctx, domain.BannerQuery{Show: true})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("隐藏后应只剩 1 条展示中的，得到 %d", len(rows))
	}

	n, err := env.Service.RemoveBanners(ctx, []uint64{created.ID.Uint64()})
	if err != nil || n != 1 {
		t.Fatalf("删除轮播图失败: n=%d err=%v", n, err)
	}
	if len(env.Banners.All()) != 1 {
		t.Fatalf("删除后应剩 1 条，得到 %d", len(env.Banners.All()))
	}
}

func TestBannerRejectsUnavailableImage(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	env.Images.MarkUnusable(11)

	if _, err := env.Service.CreateBanner(ctx, apptestInput(11, "/a", true)); !apperr.IsCode(err, apperr.CodeImageUnavailable) {
		t.Fatalf("不可用图片应报错，得到 %v", err)
	}
	if _, err := env.Service.CreateBanner(ctx, apptestInput(0, "/a", true)); !apperr.IsCode(err, apperr.CodeImageUnavailable) {
		t.Fatalf("缺少封面应报错，得到 %v", err)
	}
}

func TestBannerUpdateMissing(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	if err := env.Service.UpdateBanner(ctx, 999, apptestInput(11, "/a", true)); !apperr.IsCode(err, apperr.CodeBannerNotFound) {
		t.Fatalf("不存在的轮播图应报错，得到 %v", err)
	}
}

// apptestInput 构造轮播图入参。
func apptestInput(imageID uint64, href string, show bool) app.BannerInput {
	return app.BannerInput{CoverImageID: ctype.NewID(imageID), Href: href, Show: show}
}

// ── 新增的五个配置段 ────────────────────────────────────────────

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

func TestUpdateUploadReplacesWhitelist(t *testing.T) {
	// 白名单是整体替换而不是合并：合并会让"删掉一个后缀"永远删不掉。
	env := newEnv()
	if err := env.Service.UpdateUpload(conf.Upload{Whitelist: []string{"jpg", "png", "gif"}}); err != nil {
		t.Fatalf("更新上传策略失败: %v", err)
	}
	if err := env.Service.UpdateUpload(conf.Upload{Whitelist: []string{"webp"}}); err != nil {
		t.Fatalf("更新上传策略失败: %v", err)
	}
	got := env.Runtime.Upload().Whitelist
	if len(got) != 1 || got[0] != "webp" {
		t.Fatalf("白名单应整体替换，得到 %v", got)
	}
}

// ── 第 8 段 deco（装饰） ────────────────────────────────────────

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
		t.Fatalf("segments 应钳到 %d，实际 %d", conf.CharmSegmentsMax, got.Segments)
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
