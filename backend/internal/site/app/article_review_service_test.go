package app_test

import (
	"strings"
	"testing"

	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/apperr"
	siteapp "myblogx/internal/site/app"
	"myblogx/internal/site/apptest"
)

// 2026-11-28 / ADR-0037 §6.1：聚合端点先校验后写。
//
// 这条不是"多一层防御"：两段配置由两个页面各写一半，跨段事务做不到
// （运行期配置服务是两段各自写的），"先校验后写"是唯一能保证
// "填错了值不会留下半套配置"的手段。

// TestUpdateArticleReview评分关时强制关闸门 钉住 D3（闸门强写在后端）。
func TestUpdateArticleReview评分关时强制关闸门(t *testing.T) {
	env := newEnv()
	env.Service.UpdateSite(conf.Site{
		Article: confsite.Article{
			QualityGate: confsite.QualityGate{Enabled: true, PassScore: 50},
			// 评分关、闸门开：这是页面之外（直接改配置）才会出现的矛盾值。
			ScoreEnabled:        false,
			AIComplianceEnabled: true,
		},
	})

	if err := env.Service.UpdateArticleReview(conf.Site{
		Article: confsite.Article{
			QualityGate:         confsite.QualityGate{Enabled: true, PassScore: 50},
			ScoreEnabled:        false,
			AIComplianceEnabled: true,
		},
	}, nil); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	got := env.Service.AdminSite().Article
	if got.ScoreEnabled {
		t.Fatal("score_enabled=false 不该被写回 true")
	}
	if got.QualityGate.Enabled {
		t.Fatal("评分关掉时闸门必须被强写为 false（没有判据的闸门只会把所有投稿拒掉）")
	}
}

// TestUpdateArticleReview评分关时重新开闸门也不生效 钉住"重新开评分时闸门不自动复活"。
func TestUpdateArticleReview评分关时重新开闸门也不生效(t *testing.T) {
	env := newEnv()
	// 这次传"闸门开"，但评分仍是关 —— 归一化必须仍然把闸门写成 false。
	if err := env.Service.UpdateArticleReview(conf.Site{
		Article: confsite.Article{
			QualityGate:  confsite.QualityGate{Enabled: true},
			ScoreEnabled: false,
		},
	}, nil); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if env.Service.AdminSite().Article.QualityGate.Enabled {
		t.Fatal("评分关闭时，即便请求体写 true，闸门也必须落库为 false")
	}
}

// TestUpdateArticleReview闸门开但AI不可用时报错 钉住 D3 的第二半：
// 闸门失败关闭，叠上 AI 不可用就等于"所有投稿都被拒"。
func TestUpdateArticleReview闸门开但AI不可用时报错(t *testing.T) {
	// aiReady 是 app.New 的注入参数，newEnv 传 nil（不校验 AI 可用性）。
	// 这条用例验的正是"AI 不可用时拒绝开闸门"，所以自己装一套 aiReady=false 的服务。
	aiDown := apptest.NewEnvWithAIReady(func() bool { return false })
	env := aiDown

	err := env.Service.UpdateArticleReview(conf.Site{
		Article: confsite.Article{
			QualityGate:  confsite.QualityGate{Enabled: true},
			ScoreEnabled: true,
		},
	}, nil)
	if err == nil {
		t.Fatal("AI 不可用 + 闸门开启时必须拒绝保存")
	}
	// 这条拒绝来自「AI 可用性」校验，不是字段格式错误，所以不带 article 段的
	// 格式错误码 —— 断言看文案，因为"为什么拒"才是这条校验存在的理由。
	if !strings.Contains(userMessage(err), "需要先启用 AI") {
		t.Fatalf("拒绝理由应说清 AI 不可用，实际 %v", err)
	}
}

// TestUpdateArticleReview非法落点被拒 钉住四档校验。
func TestUpdateArticleReview非法落点被拒(t *testing.T) {
	env := newEnv()
	err := env.Service.UpdateArticleReview(conf.Site{
		Article: confsite.Article{
			ScoreEnabled:        true,
			AIComplianceEnabled: true,
			AIFailureAction:     "auto", // 不在四档之内
		},
	}, nil)
	if err == nil {
		t.Fatal("非法 ai_failure_action 必须被拒绝")
	}
	if !apperr.IsCode(err, apperr.CodeSiteArticleInvalid) {
		t.Fatalf("应报 article 段错误码，实际 %v", err)
	}
}

// TestUpdateArticleReview两段非法时一段都不写 钉住"先校验后写"。
func TestUpdateArticleReview两段非法时一段都不写(t *testing.T) {
	env := newEnv()
	before := env.Service.AdminSite().Article.ScoreEnabled

	err := env.Service.UpdateArticleReview(
		conf.Site{Article: confsite.Article{ScoreEnabled: true, AIComplianceEnabled: true}},
		&conf.ContentFilter{AutoDispose: conf.ContentFilterAutoDispose{
			// 观察档没有 reject：合法值是 release / draft。
			BlockAction: "release",
			WatchAction: "reject",
		}},
	)
	if err == nil {
		t.Fatal("观察档 reject 必须被拒绝（观察词没有禁用这一档）")
	}
	if !apperr.IsCode(err, apperr.CodeSiteContentFilterInvalid) {
		t.Fatalf("应报 content_filter 段错误码，实际 %v", err)
	}
	// site 段必须先校验通过再写；这里 content_filter 非法，所以 site 段一个字节都不该变。
	if got := env.Service.AdminSite().Article.ScoreEnabled; got != before {
		t.Fatalf("site 段不该被写入（先校验后写），实际 %v", got)
	}
}

// TestUpdateArticleReview只传处置时词表不动 钉住两段互不覆盖。
func TestUpdateArticleReview只传处置时词表不动(t *testing.T) {
	env := newEnv()
	env.Service.UpdateContentFilter(conf.ContentFilter{
		Words: []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
		AutoDispose: conf.ContentFilterAutoDispose{
			BlockAction: "release",
			WatchAction: "release",
		},
	})

	if err := env.Service.UpdateArticleReview(
		env.Service.AdminSite(),
		&conf.ContentFilter{
			Words: []conf.ContentFilterWord{{Word: "冰毒", Tier: "block", Category: "drugs"}},
			AutoDispose: conf.ContentFilterAutoDispose{
				BlockAction: "draft",
				WatchAction: "draft",
			},
		},
	); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	got := env.Service.AdminContentFilter()
	if got.AutoDispose.BlockAction != "draft" || got.AutoDispose.WatchAction != "draft" {
		t.Fatalf("处置应被写入，实际 %+v", got.AutoDispose)
	}
	if len(got.Words) != 1 {
		t.Fatalf("词表应原样保留，实际 %v", got.Words)
	}
}

// TestUpdateArticleReview旧值mask升级成release 钉住 D7 的保存侧升级。
func TestUpdateArticleReview旧值mask升级成release(t *testing.T) {
	env := newEnv()
	if err := env.Service.UpdateArticleReview(
		env.Service.AdminSite(),
		&conf.ContentFilter{AutoDispose: conf.ContentFilterAutoDispose{
			BlockAction: "mask",
			WatchAction: "mask",
		}},
	); err != nil {
		t.Fatalf("旧值 mask 应被接受并升级，实际报错 %v", err)
	}
	got := env.Service.AdminContentFilter().AutoDispose
	if got.BlockAction != siteapp.ContentFilterActionRelease || got.WatchAction != siteapp.ContentFilterActionRelease {
		t.Fatalf("旧值 mask 应升级成 release，实际 %+v", got)
	}
}
