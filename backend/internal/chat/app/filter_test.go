package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 内容过滤（票 397 / ADR-0032）：私信正文在落库前把命中词替换成等长 `*`。
//
// 钉住的是"落库那份文本"而不是"过滤被调用了"：HTTP 落库是唯一真相，
// WS 只是通知，会话摘要也由落库内容派生 —— 只要落库是星号版，三处就同时干净。

// fakeChatFilter 按固定词替换，并记录被调用过的文本（用来钉住"哪些消息该过滤"）。
type fakeChatFilter struct {
	words  []string
	calls  []string
	broken bool
}

func (f *fakeChatFilter) FilterChatText(_ context.Context, in app.FilterText) (app.FilteredText, error) {
	f.calls = append(f.calls, in.Text)
	if f.broken {
		return app.FilteredText{}, errors.New("filter broken")
	}
	out := []rune(in.Text)
	replaced := 0
	for _, w := range f.words {
		word := []rune(w)
		for i := 0; i+len(word) <= len(out); i++ {
			if string(out[i:i+len(word)]) != w {
				continue
			}
			for j := range word {
				out[i+j] = '*'
			}
			replaced++
		}
	}
	return app.FilteredText{Masked: string(out), FilteredCount: replaced}, nil
}

// TestSendMasksContentInStoredMessage 私信正文命中词应替换后落库。
func TestSendMasksContentInStoredMessage(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t, 1, domain.StrangerAllowed)
	filter := &fakeChatFilter{words: []string{"冰毒"}}
	svc.WithContentFilter(filter)

	if _, err := svc.Send(context.Background(), ctype.NewID(1), ctype.NewID(2),
		domain.TypeText, "有人想做冰毒吗", ""); err != nil {
		t.Fatalf("发送应成功: %v", err)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库一条消息，实际 %d", len(repo.sent))
	}
	want := "有人想做**吗"
	if repo.sent[0].Content != want {
		t.Fatalf("落库正文应为替换后文本: want=%q got=%q", want, repo.sent[0].Content)
	}
}

// TestSendSkipsFilterForImageMessage 图片消息的正文是 JSON，替换会打坏 image_url。
func TestSendSkipsFilterForImageMessage(t *testing.T) {
	t.Parallel()

	// 这条用例要放过图片校验，所以单独构造一个"认定图片是本站产物"的替身：
	// 本文件关注的是"图片消息不该进过滤器"，不是图片来源校验本身。
	repo := &fakeRepo{}
	svc := app.New(repo, repo, fakeRelations{relation: 1}, fakeUsers{}, &fakeStranger{verdict: domain.StrangerAllowed},
		permitLimiter{}, &allowOwnedImage{owned: true}, &fakeBlocks{}, nil)
	filter := &fakeChatFilter{words: []string{"冰毒"}}
	svc.WithContentFilter(filter)

	raw := "https://cdn.example.com/myblogx/images/x.png"
	if _, err := svc.Send(context.Background(), ctype.NewID(1), ctype.NewID(2),
		domain.TypeImage, raw, ""); err != nil {
		t.Fatalf("发送图片消息应成功: %v", err)
	}
	if len(filter.calls) != 0 {
		t.Fatalf("图片消息不该送进过滤器，实际调用了 %d 次", len(filter.calls))
	}
	// 落库的是 guard 包装后的 JSON；过滤器没动它，所以 image_url 仍然完整。
	if len(repo.sent) != 1 || repo.sent[0].Content != `{"image_url":"`+raw+`","kind":"image"}` {
		t.Fatalf("图片消息正文应原样落库，实际 %q", repo.sent[0].Content)
	}
}

// TestSendFailOpenWhenFilterBroken 过滤报错时按原文发送（审计层不该变成可用性故障）。
func TestSendFailOpenWhenFilterBroken(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t, 1, domain.StrangerAllowed)
	svc.WithContentFilter(&fakeChatFilter{words: []string{"冰毒"}, broken: true})

	if _, err := svc.Send(context.Background(), ctype.NewID(1), ctype.NewID(2),
		domain.TypeText, "冰毒", ""); err != nil {
		t.Fatalf("过滤失败不该阻断发送: %v", err)
	}
	if len(repo.sent) != 1 || repo.sent[0].Content != "冰毒" {
		t.Fatalf("过滤失败应按原文落库")
	}
}
