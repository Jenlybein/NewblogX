package app_test

import (
	"context"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
)

// 评论面的命中处置只有两档语义（ADR-0037 决策 D10 / 2026-11-28）。
//
// 为什么评论没有草稿态这一档：评论值 1「审核中」已经退场（票 400），也没有申诉
// 通道 —— 把命中的评论放进"待审队列"等于让作者以为发出去了，却永远看不到。
// 所以 draft 与 reject 对评论**等价于拒收**：不入库 + 提示作者改。
//
// 这一组用例钉住三件事：
//   - 放行档（release）：打星照发，与改造前一致；
//   - 其余档（draft / reject）：拒收，一条都不入库；
//   - 观察档命中也走同一套判定（处置档位由装配层按 block/watch 分别算好再交上来）。

// blockingFilter 是一个"报拦截档命中"的过滤替身。
type blockingFilter struct {
	masked string
}

func (b blockingFilter) Filter(_ context.Context, _ domain.FilterRequest) (domain.FilterResult, error) {
	return domain.FilterResult{
		Masked:          b.masked,
		FilteredCount:   1,
		BlockTierMasked: true,
	}, nil
}

func withDisposition(f *fixture, action string) *fixture {
	f.svc = f.svc.WithFilterDisposition(func() string { return action })
	return f
}

// TestCreateRejectsWhenBlockActionIsDraft 命中 + draft ⇒ 拒收（不入库）。
func TestCreateRejectsWhenBlockActionIsDraft(t *testing.T) {
	t.Parallel()

	f := withDisposition(newFilteredFixture(t, blockingFilter{masked: "我这里有**"}), conf.ContentFilterActionDraft)

	_, err := f.create(t, "我这里有冰毒")
	if err == nil {
		t.Fatal("拦截词命中且处置为 draft 时必须拒收")
	}
	if !apperr.IsCode(err, apperr.CodeCommentContentBlocked) {
		t.Fatalf("应返回 comment.content_blocked，实际 %v", err)
	}
	if len(f.repo.created) != 0 {
		t.Fatalf("被拒的评论一条都不该入库，实际 %d 条", len(f.repo.created))
	}
	if f.notifier.lastContent != "" {
		t.Fatalf("被拒的评论不该发通知，实际 %q", f.notifier.lastContent)
	}
}

// TestCreateRejectsWhenBlockActionIsReject 命中 + reject ⇒ 拒收（与 draft 等价）。
func TestCreateRejectsWhenBlockActionIsReject(t *testing.T) {
	t.Parallel()

	f := withDisposition(newFilteredFixture(t, blockingFilter{masked: "我这里有**"}), conf.ContentFilterActionReject)

	_, err := f.create(t, "我这里有冰毒")
	if err == nil {
		t.Fatal("拦截词命中且处置为 reject 时必须拒收")
	}
	if !apperr.IsCode(err, apperr.CodeCommentContentBlocked) {
		t.Fatalf("应返回 comment.content_blocked，实际 %v", err)
	}
	if len(f.repo.created) != 0 {
		t.Fatalf("被拒的评论一条都不该入库，实际 %d 条", len(f.repo.created))
	}
}

// TestCreatePublishesMaskedWhenBlockActionIsRelease 放行档：打星照发（改造前行为）。
func TestCreatePublishesMaskedWhenBlockActionIsRelease(t *testing.T) {
	t.Parallel()

	f := withDisposition(newFilteredFixture(t, blockingFilter{masked: "我这里有**"}), conf.ContentFilterActionRelease)

	result, err := f.create(t, "我这里有冰毒")
	if err != nil {
		t.Fatalf("放行档不该失败: %v", err)
	}
	if result.Content != "我这里有**" {
		t.Fatalf("入库前应已打星，实际 %q", result.Content)
	}
	if len(f.repo.created) != 1 {
		t.Fatalf("放行档应入库一条，实际 %d 条", len(f.repo.created))
	}
}

// TestCreatePublishesWhenNoDispositionInjected 没注入处置读函数时按放行（改造前行为）。
func TestCreatePublishesWhenNoDispositionInjected(t *testing.T) {
	t.Parallel()

	// 不加 WithFilterDisposition：默认 nil ⇒ 一律 release。
	f := newFilteredFixture(t, blockingFilter{masked: "我这里有**"})
	result, err := f.create(t, "我这里有冰毒")
	if err != nil {
		t.Fatalf("未注入处置时应照常发布: %v", err)
	}
	if result.Content != "我这里有**" {
		t.Fatalf("应打星入库，实际 %q", result.Content)
	}
}

// 编译期守卫：处置档位的取值域只有配置层一份（不在用例层另写字面量）。
var _ = conf.ContentFilterActionRelease
