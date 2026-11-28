package app_test

import (
	"errors"
	"strings"
	"testing"

	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/apperr"
)

// 2026-11-28 / ADR-0037 §6.1：聚合端点必须拒绝改动「文章发布」页拥有的字段。
//
// 为什么后端也要守：请求体里 article 是整段结构（read-modify-write），它天然带着
// 兄弟页的四个字段。前端确实只覆盖自己的，但这条端点对外开放，不能靠前端守规矩。
//
// 四个非本页字段：allow_non_admin_publish / allow_non_admin_draft /
// show_non_admin_articles / max_tags_per_article。

func TestUpdateArticleReview拒绝改动发布页字段(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*confsite.Article)
		field  string
	}{
		{
			name:   "发布权限",
			field:  "allow_non_admin_publish",
			mutate: func(a *confsite.Article) { a.AllowNonAdminPublish = true },
		},
		{
			name:   "存草稿权限",
			field:  "allow_non_admin_draft",
			mutate: func(a *confsite.Article) { a.AllowNonAdminDraft = true },
		},
		{
			name:   "过滤非管理员文章",
			field:  "show_non_admin_articles",
			mutate: func(a *confsite.Article) { a.ShowNonAdminArticles = true },
		},
		{
			name:   "标签上限",
			field:  "max_tags_per_article",
			mutate: func(a *confsite.Article) { a.MaxTagsPerArticle = 9 },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := newEnv()
			// 现值：发布页那一组都是"收紧"的取值。
			env.Service.UpdateSite(conf.Site{Article: confsite.Article{
				AllowNonAdminPublish: false,
				AllowNonAdminDraft:   false,
				ShowNonAdminArticles: false,
				MaxTagsPerArticle:    5,
				ScoreEnabled:         true,
				AIComplianceEnabled:  true,
			}})

			incoming := env.Service.AdminSite()
			// 本页自己的字段照常改（这样用例失败时能区分"是越界拦住了"还是"整体没生效"）。
			incoming.Article.ScoreEnabled = false
			tc.mutate(&incoming.Article)

			err := env.Service.UpdateArticleReview(incoming, nil)
			if err == nil {
				t.Fatalf("改动 %s 必须被拒绝", tc.field)
			}
			if !apperr.IsCode(err, apperr.CodeSiteArticleInvalid) {
				t.Fatalf("应报 article 段错误码，实际 %v", err)
			}
			// 报错要点名是哪个字段变了，否则没法排查。
			// 注意读 UserMessage()：Error() 返回的是该错误码的**通用文案**，
			// 受控的具体文案在 UserMessage() 里（响应层用它覆盖通用文案）。
			if !strings.Contains(userMessage(err), tc.field) {
				t.Fatalf("报错应点名 %s，实际 %v", tc.field, err)
			}
		})
	}
}

// TestUpdateArticleReview本页字段照常可改 是对照组：白名单不能顺手把自己的字段也拦掉。
func TestUpdateArticleReview本页字段照常可改(t *testing.T) {
	t.Parallel()

	env := newEnv()
	env.Service.UpdateSite(conf.Site{Article: confsite.Article{
		AllowNonAdminPublish: false,
		AllowNonAdminDraft:   false,
		ShowNonAdminArticles: false,
		MaxTagsPerArticle:    5,
		ScoreEnabled:         true,
		AIComplianceEnabled:  true,
		SubmitDailyMax:       15,
		SkipExamining:        false,
		QualityGate:          confsite.QualityGate{Enabled: false, PassScore: 50, GrayBandWidth: 2},
	}})

	incoming := env.Service.AdminSite()
	incoming.Article.ScoreEnabled = false
	incoming.Article.AIComplianceEnabled = false
	incoming.Article.AIFailureAction = confsite.AIFailureDraft
	incoming.Article.SubmitDailyMax = 20
	incoming.Article.SkipExamining = true

	if err := env.Service.UpdateArticleReview(incoming, nil); err != nil {
		t.Fatalf("本页字段应可改，实际报错 %v", err)
	}

	got := env.Service.AdminSite().Article
	if got.ScoreEnabled {
		t.Fatal("score_enabled 应被改成 false")
	}
	if got.AIComplianceEnabled {
		t.Fatal("ai_compliance_enabled 应被改成 false")
	}
	if got.AIFailureAction != confsite.AIFailureDraft {
		t.Fatalf("ai_failure_action 应被改成 draft，实际 %q", got.AIFailureAction)
	}
	if got.SubmitDailyMax != 20 || !got.SkipExamining {
		t.Fatalf("投稿配额与免审核应被改动，实际 %+v", got)
	}
	// 兄弟页字段保持原值（本页没碰它们）。
	if got.AllowNonAdminPublish || got.AllowNonAdminDraft || got.ShowNonAdminArticles {
		t.Fatalf("发布页字段应保持收紧的取值，实际 %+v", got)
	}
	if got.MaxTagsPerArticle != 5 {
		t.Fatalf("标签上限应保持 5，实际 %d", got.MaxTagsPerArticle)
	}
}

// userMessage 取受控具体文案（没有时是空串）。
//
// `*apperr.Error` 的 Error() 给的是该码的通用文案；要断言"报错点名了哪个字段"，
// 必须读 UserMessage() —— 响应层就是用它覆盖通用文案的。
func userMessage(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		if message := appErr.UserMessage(); message != "" {
			return message
		}
	}
	// 没有受控文案时（apperr.New 创建的错误）落到码的通用文案，
	// 那时 Error() 本身就是调用方看到的那句话。
	return err.Error()
}
