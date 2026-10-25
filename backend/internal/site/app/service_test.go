package app_test

import (
	"context"
	"testing"

	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/apperr"
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
		Enable:    true,
		SecretKey: "ai-secret-origin",
		BaseURL:   "https://ai.example.com/v1/chat/completions",
		Nickname:  "AI 助手",
		Avatar:    "/ai.png",
		Abstract:  "你好",
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
	if got := env.Service.AdminAI().SecretKey; got != domain.SensitivePlaceholder {
		t.Fatalf("管理端应遮蔽密钥，得到 %s", got)
	}
	// 公开接口不带密钥字段，这里只确认它没有意外暴露。
	if env.Service.PublicAI().Nickname == "" {
		t.Fatal("公开 AI 信息应包含昵称")
	}
}

func TestUpdateAIKeepsSecretOnPlaceholder(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	err := env.Service.UpdateAI(conf.AI{
		Enable:    true,
		SecretKey: domain.SensitivePlaceholder,
		BaseURL:   "https://new-ai.example.com",
		Nickname:  "新助手",
	})
	if err != nil {
		t.Fatalf("更新 AI 配置失败: %v", err)
	}
	current := env.Runtime.AI()
	if current.SecretKey != "ai-secret-origin" {
		t.Fatalf("占位符应保留原密钥，得到 %s", current.SecretKey)
	}
	if current.BaseURL != "https://new-ai.example.com" || current.Nickname != "新助手" {
		t.Fatalf("AI 配置未更新: %+v", current)
	}
	_ = ctx
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
	if created.Cover != "/a.png" || created.ID == 0 {
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
	if rows[0].CoverImageID == 0 {
		t.Fatalf("列表应回填封面图片 ID: %+v", rows[0])
	}

	// 隐藏后不再出现在 show=true 的列表里。
	if err := env.Service.UpdateBanner(ctx, created.ID, apptestInput(12, "/b2", false)); err != nil {
		t.Fatalf("更新轮播图失败: %v", err)
	}
	rows, _, err = env.Service.ListBanners(ctx, domain.BannerQuery{Show: true})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("隐藏后应只剩 1 条展示中的，得到 %d", len(rows))
	}

	n, err := env.Service.RemoveBanners(ctx, []uint64{created.ID})
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
	return app.BannerInput{CoverImageID: imageID, Href: href, Show: show}
}
