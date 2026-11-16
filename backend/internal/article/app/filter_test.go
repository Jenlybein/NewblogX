package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
)

// 内容过滤（票 397 / ADR-0032）：文章与合集在落库前替换命中词，
// 替换后的文本是闸门、同步风险扫描、落库与 ES 投影读到的唯一版本。

// fakeTextFilter 按固定词替换，并记录每次收到的文本（用来钉住面与字段）。
type fakeTextFilter struct {
	words  []string
	calls  []string
	broken bool
}

func (f *fakeTextFilter) apply(text string) (app.FilteredText, error) {
	f.calls = append(f.calls, text)
	if f.broken {
		return app.FilteredText{}, errors.New("filter broken")
	}
	out := []rune(text)
	count := 0
	for _, w := range f.words {
		word := []rune(w)
		for i := 0; i+len(word) <= len(out); i++ {
			if string(out[i:i+len(word)]) != w {
				continue
			}
			for j := range word {
				out[i+j] = '*'
			}
			count++
		}
	}
	return app.FilteredText{Masked: string(out), FilteredCount: count}, nil
}

type fakeArticleTextFilter struct{ inner *fakeTextFilter }

func (f fakeArticleTextFilter) FilterArticleText(_ context.Context, in app.FilterText) (app.FilteredText, error) {
	return f.inner.apply(in.Text)
}

type fakeCollectionTextFilter struct{ inner *fakeTextFilter }

func (f fakeCollectionTextFilter) FilterCollectionText(_ context.Context, in app.FilterText) (app.FilteredText, error) {
	return f.inner.apply(in.Text)
}

// newFilteredService 构造接入内容过滤的文章服务。
func newFilteredService(t *testing.T, filter *fakeTextFilter) (*app.Service, *apptest.Repos) {
	t.Helper()
	svc, repos := newModerationService(t, nil, nil)
	svc.WithContentFilter(fakeArticleTextFilter{inner: filter}).
		WithCollectionContentFilter(fakeCollectionTextFilter{inner: filter})
	return svc, repos
}

// TestCreateMasksTitleAndContent 文章标题与正文都应在落库前被替换。
func TestCreateMasksTitleAndContent(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"冰毒"}}
	svc, _ := newFilteredService(t, filter)

	created, err := svc.Create(context.Background(), app.CreateArticleCmd{
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "关于冰毒的科普",
		Content:       "本文讨论冰毒成瘾",
		TargetStatus:  domain.StatusDraft,
		SkipExamining: true,
	})
	if err != nil {
		t.Fatalf("创建应成功: %v", err)
	}
	if created.FilteredCount != 2 {
		t.Fatalf("标题与正文各命中一处，filtered_count 应为 2，实际 %d", created.FilteredCount)
	}
	if created.Article.Title() != "关于**的科普" {
		t.Fatalf("标题应为替换后文本，实际 %q", created.Article.Title())
	}
	if created.Article.Content() != "本文讨论**成瘾" {
		t.Fatalf("正文应为替换后文本，实际 %q", created.Article.Content())
	}
}

// TestCreateFailOpenWhenFilterBroken 过滤报错时按原文落库，不阻断作者。
func TestCreateFailOpenWhenFilterBroken(t *testing.T) {
	t.Parallel()

	svc, _ := newFilteredService(t, &fakeTextFilter{words: []string{"冰毒"}, broken: true})

	created, err := svc.Create(context.Background(), app.CreateArticleCmd{
		Actor:         authorViewer(),
		Policy:        domain.OpenWritePolicy(),
		Title:         "冰毒",
		Content:       "冰毒",
		TargetStatus:  domain.StatusDraft,
		SkipExamining: true,
	})
	if err != nil {
		t.Fatalf("过滤失败不该阻断创建: %v", err)
	}
	if created.Article.Title() != "冰毒" || created.FilteredCount != 0 {
		t.Fatalf("过滤失败应按原文落库且处数为 0")
	}
}

// TestCreateCollectionFiltersTitleAndDescription 合集标题与描述应各过滤一次。
func TestCreateCollectionFiltersTitleAndDescription(t *testing.T) {
	t.Parallel()

	filter := &fakeTextFilter{words: []string{"博彩"}}
	svc, _ := newFilteredService(t, filter)

	if _, err := svc.CreateCollection(context.Background(), app.CreateCollectionCmd{
		Actor:       adminViewer(),
		Title:       "博彩合集",
		Description: "关于博彩的一切",
		Displayed:   true,
	}); err != nil {
		t.Fatalf("创建合集应成功: %v", err)
	}
	if len(filter.calls) != 2 {
		t.Fatalf("合集应过滤标题与描述各一次，实际 %d 次", len(filter.calls))
	}
	for _, call := range filter.calls {
		if call == "" {
			t.Fatalf("不该对空串调用过滤器")
		}
	}
}
