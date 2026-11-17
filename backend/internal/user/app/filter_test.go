package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// 内容过滤（票 397 / ADR-0032）：昵称与个人简介在落库前把命中词替换成等长 `*`。
//
// 这两个面是 2026-11 那次腾讯云判定里唯一被认定为"发布传播"的文本面：
// 昵称会渲染在公开主页与评论区作者名上，而在改造前它们只有长度校验。
// 所以这里钉住两件事：替换后的值是落库的值；替换不改变长度，格式校验结论不变。

// fakeProfileFilter 按固定词替换，并记录调用。
type fakeProfileFilter struct {
	words  []string
	calls  []string
	broken bool
}

func (f *fakeProfileFilter) mask(text string) string {
	out := []rune(text)
	for _, w := range f.words {
		word := []rune(w)
		for i := 0; i+len(word) <= len(out); i++ {
			if string(out[i:i+len(word)]) != w {
				continue
			}
			for j := range word {
				out[i+j] = '*'
			}
		}
	}
	return string(out)
}

func (f *fakeProfileFilter) filter(_ context.Context, in app.FilterText) (app.FilteredText, error) {
	f.calls = append(f.calls, in.Text)
	if f.broken {
		return app.FilteredText{}, errors.New("filter broken")
	}
	return app.FilteredText{Masked: f.mask(in.Text)}, nil
}

type fakeNicknameFilter struct{ inner *fakeProfileFilter }

func (f fakeNicknameFilter) FilterNicknameText(ctx context.Context, in app.FilterText) (app.FilteredText, error) {
	return f.inner.filter(ctx, in)
}

type fakeAbstractFilter struct{ inner *fakeProfileFilter }

func (f fakeAbstractFilter) FilterAbstractText(ctx context.Context, in app.FilterText) (app.FilteredText, error) {
	return f.inner.filter(ctx, in)
}

// TestUpdateProfileMasksNicknameAndAbstract 昵称与简介命中词应替换后保存。
func TestUpdateProfileMasksNicknameAndAbstract(t *testing.T) {
	t.Parallel()

	filter := &fakeProfileFilter{words: []string{"冰毒"}}
	r := apptest.NewRepos(baseTime)
	u := seed(t, r)
	svc := r.ServiceWith(app.WithContentFilter(
		fakeNicknameFilter{inner: filter}, fakeAbstractFilter{inner: filter}))

	nickname := "卖冰毒的"
	abstract := "聊聊冰毒"
	if _, err := svc.Update(testCtx(), alice, aliceID, domain.ProfileUpdate{
		Nickname: &nickname,
		Abstract: &abstract,
	}); err != nil {
		t.Fatalf("更新资料应成功: %v", err)
	}

	got, err := r.Users().FindByID(testCtx(), aliceID)
	if err != nil {
		t.Fatalf("读回用户失败: %v", err)
	}
	if got.Nickname != "卖**的" {
		t.Fatalf("昵称应为替换后文本，实际 %q", got.Nickname)
	}
	if got.Abstract != "聊聊**" {
		t.Fatalf("简介应为替换后文本，实际 %q", got.Abstract)
	}
	_ = u
}

// TestUpdateProfileFailOpenWhenFilterBroken 过滤报错时按原文保存，不阻断用户。
func TestUpdateProfileFailOpenWhenFilterBroken(t *testing.T) {
	t.Parallel()

	filter := &fakeProfileFilter{words: []string{"冰毒"}, broken: true}
	r := apptest.NewRepos(baseTime)
	seed(t, r)
	svc := r.ServiceWith(app.WithContentFilter(
		fakeNicknameFilter{inner: filter}, fakeAbstractFilter{inner: filter}))

	nickname := "冰毒"
	if _, err := svc.Update(testCtx(), alice, aliceID, domain.ProfileUpdate{
		Nickname: &nickname,
	}); err != nil {
		t.Fatalf("过滤失败不该阻断资料保存: %v", err)
	}
	got, _ := r.Users().FindByID(testCtx(), aliceID)
	if got.Nickname != "冰毒" {
		t.Fatalf("过滤失败应按原文保存，实际 %q", got.Nickname)
	}
}

// TestUpdateProfileSkipsEmptyAbstract 未提供的字段不该进过滤器（nil ≠ 清空）。
func TestUpdateProfileSkipsEmptyAbstract(t *testing.T) {
	t.Parallel()

	filter := &fakeProfileFilter{words: []string{"冰毒"}}
	r := apptest.NewRepos(baseTime)
	seed(t, r)
	svc := r.ServiceWith(app.WithContentFilter(
		fakeNicknameFilter{inner: filter}, fakeAbstractFilter{inner: filter}))

	nickname := "新昵称"
	if _, err := svc.Update(testCtx(), alice, aliceID, domain.ProfileUpdate{
		Nickname: &nickname,
	}); err != nil {
		t.Fatalf("更新昵称应成功: %v", err)
	}
	if len(filter.calls) != 1 {
		t.Fatalf("只应过滤昵称一次，实际 %d 次", len(filter.calls))
	}
	got, _ := r.Users().FindByID(testCtx(), aliceID)
	if got.Abstract != "hi" {
		t.Fatalf("未提供的简介不该被清空，实际 %q", got.Abstract)
	}
}
