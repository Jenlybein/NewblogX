package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 评论接入内容过滤（票 396 / ADR-0032 决策 5）的用例。
//
// 这一层只钉外部可观察行为：
//   - 入库、通知、响应里都是**替换后**的文本，响应带上 filtered_count；
//   - 命中记录拿得到新评论 ID；
//   - 过滤器报错时照常发布（fail-open），只记日志；
//   - 没接入过滤器时退回改造前的行为。

// fakeFilterer 是内容过滤端口的替身：替换结果与失败由用例指定。
type fakeFilterer struct {
	masked  string
	count   int
	err     error
	calls   int
	lastReq domain.FilterRequest
}

func (f *fakeFilterer) Filter(_ context.Context, in domain.FilterRequest) (domain.FilterResult, error) {
	f.calls++
	f.lastReq = in
	if f.err != nil {
		return domain.FilterResult{}, f.err
	}
	return domain.FilterResult{Masked: f.masked, FilteredCount: f.count}, nil
}

// newFilteredFixture 构造一个"已接入过滤"的评论服务。
func newFilteredFixture(t *testing.T, filter domain.ContentFilterer) *fixture {
	t.Helper()
	f := newFixture(t)
	if filter != nil {
		f.svc = f.svc.WithContentFilter(filter)
	}
	return f
}

func TestCreateUsesMaskedTextForStorageAndNotice(t *testing.T) {
	filter := &fakeFilterer{masked: "我这里有**", count: 1}
	f := newFilteredFixture(t, filter)

	result, err := f.create(t, "我这里有冰毒")
	if err != nil {
		t.Fatalf("不该失败: %v", err)
	}

	if filter.calls != 1 {
		t.Fatalf("创建评论必须过一次过滤器，实际调用 %d 次", filter.calls)
	}
	if result.Content != "我这里有**" {
		t.Fatalf("响应里应是替换后的文本，实际 %q", result.Content)
	}
	if result.FilteredCount != 1 {
		t.Fatalf("响应应带 filtered_count=1，实际 %d", result.FilteredCount)
	}
	if len(f.repo.created) != 1 {
		t.Fatalf("应有 1 条评论入库，实际 %d", len(f.repo.created))
	}
	if got := f.repo.created[0].Content; got != "我这里有**" {
		t.Fatalf("入库的必须是替换后的文本，实际 %q", got)
	}
	// 通知里的内容同样是替换后的版本：通知是对外可见面之一（ADR-0032 决策 5）。
	if f.notifier.lastContent != "我这里有**" {
		t.Fatalf("通知里必须是替换后的文本，实际 %q", f.notifier.lastContent)
	}
	// 命中记录要能指回新评论（处置动作靠它），用户也要对得上。
	if filter.lastReq.ContentID != result.ID {
		t.Fatalf("命中记录的内容 ID 应是新评论 ID %v，实际 %v", result.ID, filter.lastReq.ContentID)
	}
	if filter.lastReq.UserID != ctype.NewID(7) {
		t.Fatalf("命中记录的用户 ID 不对: %v", filter.lastReq.UserID)
	}
	if f.logger.errors != 0 {
		t.Fatalf("成功路径不该写 error 日志，实际 %d 条", f.logger.errors)
	}
}

// TestCreatePublishesWhenFilterFails 是验收条"命中记录写入失败不阻断评论发布"。
//
// 端口层把"记录写失败"收成了 fail-open（记录失败只记日志、仍返回替换后文本），
// 所以这里模拟的是更坏的一种：过滤器本身返回错误。评论仍必须发布，
// 只是退回原文并留下一条 error 日志。
func TestCreatePublishesWhenFilterFails(t *testing.T) {
	filter := &fakeFilterer{err: errors.New("命中表不可用")}
	f := newFilteredFixture(t, filter)

	result, err := f.create(t, "我这里有冰毒")
	if err != nil {
		t.Fatalf("过滤器失败不该让发评论失败: %v", err)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("过滤器失败时仍应照常发布，实际状态 %d", result.Status)
	}
	if len(f.repo.created) != 1 || f.repo.created[0].Content != "我这里有冰毒" {
		t.Fatalf("过滤器失败时按原文入库，实际 %+v", f.repo.created)
	}
	if result.FilteredCount != 0 {
		t.Fatalf("没替换成功时 filtered_count 应为 0，实际 %d", result.FilteredCount)
	}
	if f.logger.errors == 0 {
		t.Fatal("过滤器失败必须留下 error 日志（否则问题永远没人知道）")
	}
}

// TestCreateWithoutFilterKeepsOldBehaviour：没接入过滤器的调用点行为不变。
func TestCreateWithoutFilterKeepsOldBehaviour(t *testing.T) {
	f := newFilteredFixture(t, nil)

	result, err := f.create(t, "我这里有冰毒")
	if err != nil {
		t.Fatalf("不该失败: %v", err)
	}
	if result.Content != "我这里有冰毒" {
		t.Fatalf("未接入过滤时应按原文返回，实际 %q", result.Content)
	}
	if result.FilteredCount != 0 {
		t.Fatalf("未接入过滤时 filtered_count 应为 0，实际 %d", result.FilteredCount)
	}
	if f.repo.created[0].Content != "我这里有冰毒" {
		t.Fatalf("未接入过滤时应按原文入库，实际 %q", f.repo.created[0].Content)
	}
}

// TestCreateFilteredTextIsStillPublished 钉住 #396 的顺序在新形态下没变：
// 先替换、再落库，替换后的文本照常发布 —— 没有任何东西会因为它命中了词而被拦。
func TestCreateFilteredTextIsStillPublished(t *testing.T) {
	f := newFilteredFixture(t, &fakeFilterer{masked: "需要****", count: 1})

	result, err := f.create(t, "需要代开发票")
	if err != nil {
		t.Fatalf("命中词只替换、不该被拦: %v", err)
	}
	if result.Content != "需要****" {
		t.Fatalf("入库应是替换后的文本，实际 %q", result.Content)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("过滤后的评论应立即发布，实际状态 %d", result.Status)
	}
	if f.cache.added != 1 || f.notifier.count != 1 {
		t.Fatal("过滤后的评论同样要计数、要通知")
	}
}
