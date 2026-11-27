package domain_test

import (
	"fmt"
	"strings"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 一篇文章最多 5 个标签（产品决定）。
//
// 这些用例钉住三件事：
//  1. 上限就是 5：第 6 个是错误，第 5 个不是；
//  2. **同一个标签传两次只算一个** —— 不去重会同时踩两个坑：计数虚高被误判超限，
//     以及仓储对重复 ID 只回一份、掉进"标签不存在或已停用"那条数量检查；
//  3. 中文文案里的数字与本常量一致（apperr 是最底层包，不能反向 import，
//     数字只能各写一份，所以必须有测试盯着漂移）。
func TestNormalizeTagIDsDedupesAndDropsEmpty(t *testing.T) {
	t.Parallel()

	first := ctype.NewID(1)
	second := ctype.NewID(2)
	got := domain.NormalizeTagIDs([]domain.TagID{first, first, domain.TagID(""), second, first})
	want := []domain.TagID{first, second}

	if len(got) != len(want) {
		t.Fatalf("去重后应有 %d 个，实际 %d（%v）", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序应保持首次出现的次序: 期望 %v，实际 %v", want, got)
		}
	}
}

func TestNormalizeTagIDsEmptyInput(t *testing.T) {
	t.Parallel()

	if got := domain.NormalizeTagIDs(nil); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}
	if got := domain.NormalizeTagIDs([]domain.TagID{domain.TagID(""), domain.TagID("")}); len(got) != 0 {
		t.Fatalf("全是空 ID 应归零，实际 %v", got)
	}
}

func TestValidateArticleTagsAcceptsUpToLimit(t *testing.T) {
	t.Parallel()

	ids := make([]domain.TagID, 0, domain.MaxArticleTags)
	for i := 1; i <= domain.MaxArticleTags; i++ {
		ids = append(ids, ctype.NewID(uint64(i)))
	}
	got, err := domain.ValidateArticleTags(ids)
	if err != nil {
		t.Fatalf("正好 %d 个应通过，实际 %v", domain.MaxArticleTags, err)
	}
	if len(got) != domain.MaxArticleTags {
		t.Fatalf("应原样返回 %d 个，实际 %d", domain.MaxArticleTags, len(got))
	}
}

func TestValidateArticleTagsRejectsOverLimit(t *testing.T) {
	t.Parallel()

	ids := make([]domain.TagID, 0, domain.MaxArticleTags+1)
	for i := 1; i <= domain.MaxArticleTags+1; i++ {
		ids = append(ids, ctype.NewID(uint64(i)))
	}
	if _, err := domain.ValidateArticleTags(ids); !apperr.IsCode(err, apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("第 %d 个应返回 tag_limit_exceeded，实际 %v", domain.MaxArticleTags+1, err)
	}
}

// 5 个标签里有一个重复：归一化后是 4 个，不该被判超限。
func TestValidateArticleTagsCountsAfterDedupe(t *testing.T) {
	t.Parallel()

	ids := []domain.TagID{
		ctype.NewID(1), ctype.NewID(2), ctype.NewID(3),
		ctype.NewID(4), ctype.NewID(4), // 重复
	}
	got, err := domain.ValidateArticleTags(ids)
	if err != nil {
		t.Fatalf("5 个里有 1 个重复应算 4 个、不该报错，实际 %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("归一化后应是 4 个，实际 %d", len(got))
	}
}

// 文案与常量必须对得上：改 MaxArticleTags 忘了改文案（或反过来）会在这里红。
func TestTagLimitMessageMatchesConstant(t *testing.T) {
	t.Parallel()

	msg := apperr.Message(apperr.CodeArticleTagLimitExceeded)
	if msg == "" || msg == string(apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("错误码没有中文文案: %q", msg)
	}
	// 文案里的数字（"最多只能选 5 个标签"）必须等于常量。
	if want := fmt.Sprintf("%d", domain.MaxArticleTags); !strings.Contains(msg, want) {
		t.Fatalf("文案 %q 里没有上限数字 %s（改了 domain.MaxArticleTags 要同步改 apperr 消息表）", msg, want)
	}
}
