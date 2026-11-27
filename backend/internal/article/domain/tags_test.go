package domain_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 一篇文章默认最多 5 个标签（产品决定），上限本身是站点运行时配置。
//
// 这些用例钉住四件事：
//  1. 默认上限就是 5：第 6 个是错误，第 5 个不是；
//  2. **同一个标签传两次只算一个** —— 不去重会同时踩两个坑：计数虚高被误判超限，
//     以及仓储对重复 ID 只回一份、掉进"标签不存在或已停用"那条数量检查；
//  3. 上限可配：配置里的 3 就是生效的 3（不再回头看默认值），且非法值（0 / 负数 /
//     超过 20）回落默认值，不让一次误填把全站锁死；
//  4. 中文文案里**不得**再出现写死的数字（上限已经是后台可改的运行值，
//     服务端再抄一个数字就是第二份会漂的副本）。
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

// 配置里的上限先归一化：0 / 负数 / 超过上界一律回落默认值。
//
// 为什么 0 必须回落而不是"0 个都不许选"：这个字段是"最多可选几个"，
// 填 0 更可能是"配置没填好"而不是"想关掉标签功能"；按字面执行会让全站
// 连草稿都存不了（草稿也带标签）。真正想关标签该用别的开关表达。
func TestNormalizeTagLimitFallsBackOnIllegalValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int
		want int
	}{
		{"0 回落默认值", 0, domain.MaxArticleTags},
		{"负数回落默认值", -3, domain.MaxArticleTags},
		{"超过上界回落默认值", domain.MaxArticleTagLimit + 1, domain.MaxArticleTags},
		{"下界内原样", domain.MinArticleTagLimit, domain.MinArticleTagLimit},
		{"上界内原样", domain.MaxArticleTagLimit, domain.MaxArticleTagLimit},
		{"默认值本身合法", domain.MaxArticleTags, domain.MaxArticleTags},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.NormalizeTagLimit(c.in); got != c.want {
				t.Fatalf("NormalizeTagLimit(%d) = %d，期望 %d", c.in, got, c.want)
			}
		})
	}
}

func TestValidateArticleTagsAcceptsUpToLimit(t *testing.T) {
	t.Parallel()

	ids := make([]domain.TagID, 0, domain.MaxArticleTags)
	for i := 1; i <= domain.MaxArticleTags; i++ {
		ids = append(ids, ctype.NewID(uint64(i)))
	}
	got, err := domain.ValidateArticleTags(ids, domain.MaxArticleTags)
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
	if _, err := domain.ValidateArticleTags(ids, domain.MaxArticleTags); !apperr.IsCode(err, apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("第 %d 个应返回 tag_limit_exceeded，实际 %v", domain.MaxArticleTags+1, err)
	}
}

// 上限可配：传 3 时第 4 个就被拒，第 3 个通过 —— 判定跟着配置走，不是跟着默认值走。
func TestValidateArticleTagsHonorsConfiguredLimit(t *testing.T) {
	t.Parallel()

	const limit = 3
	ids := []domain.TagID{
		ctype.NewID(1), ctype.NewID(2), ctype.NewID(3), ctype.NewID(4),
	}
	if got, err := domain.ValidateArticleTags(ids[:limit], limit); err != nil {
		t.Fatalf("正好 %d 个应通过，实际 %v", limit, err)
	} else if len(got) != limit {
		t.Fatalf("应原样返回 %d 个，实际 %d", limit, len(got))
	}
	if _, err := domain.ValidateArticleTags(ids, limit); !apperr.IsCode(err, apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("配置上限 %d 时第 %d 个应返回 tag_limit_exceeded，实际 %v", limit, limit+1, err)
	}
}

// 5 个标签里有一个重复：归一化后是 4 个，不该被判超限。
func TestValidateArticleTagsCountsAfterDedupe(t *testing.T) {
	t.Parallel()

	ids := []domain.TagID{
		ctype.NewID(1), ctype.NewID(2), ctype.NewID(3),
		ctype.NewID(4), ctype.NewID(4), // 重复
	}
	got, err := domain.ValidateArticleTags(ids, domain.MaxArticleTags)
	if err != nil {
		t.Fatalf("5 个里有 1 个重复应算 4 个、不该报错，实际 %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("归一化后应是 4 个，实际 %d", len(got))
	}
}

// 反向守卫：上限已经是后台可改的运行值，服务端文案里**不得**再出现写死的数字。
//
// 旧用例是正向的（"文案里必须有 5"），配置化之后它恰好会把 bug 当成正确行为：
// 管理员把上限改成 8，而文案还写着"最多 5 个"，作者照着取消到 5 个仍然被拒。
// 数字由前端按站点配置显示（GET /api/site/site 带这个字段），所以这里要求
// 文案里一个数字都没有 —— 只要出现数字，它就会被读者当成"当前生效的上限"。
func TestTagLimitMessageHasNoHardcodedNumber(t *testing.T) {
	t.Parallel()

	msg := apperr.Message(apperr.CodeArticleTagLimitExceeded)
	if msg == "" || msg == string(apperr.CodeArticleTagLimitExceeded) {
		t.Fatalf("错误码没有中文文案: %q", msg)
	}
	for _, r := range msg {
		if r >= '0' && r <= '9' {
			t.Fatalf("文案 %q 里出现了写死的数字 %q：上限已是站点配置，服务端文案不得再抄数字（数字由前端显示）", msg, string(r))
		}
	}
}
