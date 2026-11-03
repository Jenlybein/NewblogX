package viewhistory_test

import (
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
)

// TestPurgeCutoffIsNinetyDays 钉住清理口径：早于「固定时刻 - 90 天」的记录被清理。
//
// 用固定时刻而不是 time.Now()：90 天清理是**数据策略**（会真实抹掉用户的浏览历史），
// 一旦有人把保留天数改成 30 或把符号写反（now.Add(+90d)），这里必须红。
// 边界取等号的处置（恰好 90 天前）由 adapter 的 SQL 用例守（严格小于）。
func TestPurgeCutoffIsNinetyDays(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	cutoff := viewhistory.PurgeCutoff(now)

	want := time.Date(2026, 8, 6, 12, 0, 0, 0, now.Location())
	if !cutoff.Equal(want) {
		t.Fatalf("清理界限应为 %s（now-90d），实际 %s", want, cutoff)
	}
	// 90 天必须正好是 90*24h（不跨夏令时的时区也成立）：
	if got := now.Sub(cutoff); got != 90*24*time.Hour {
		t.Fatalf("保留期应为 90×24h，实际 %s", got)
	}
}

// TestListQueryNormalize 收敛非法分页参数：limit 缺省 10、上限 100、关键词去空白。
//
// 与 domain.Page.Normalize 的口径一致（同仓已有约定），避免"limit=0 查全表"。
func TestListQueryNormalize(t *testing.T) {
	t.Parallel()

	cursor := &viewhistory.Cursor{
		ViewedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		ArticleID: domain.ArticleID(ctype.NewID(7)),
	}

	cases := []struct {
		name      string
		in        viewhistory.ListQuery
		wantLimit int
		wantKw    string
	}{
		{name: "缺省 limit", in: viewhistory.ListQuery{}, wantLimit: 10},
		{name: "负数 limit", in: viewhistory.ListQuery{Limit: -3}, wantLimit: 10},
		{name: "超上限", in: viewhistory.ListQuery{Limit: 5000}, wantLimit: 100},
		{name: "正常值保留", in: viewhistory.ListQuery{Limit: 20}, wantLimit: 20},
		{name: "关键词去首尾空白", in: viewhistory.ListQuery{Keyword: "  Go 并发  "}, wantLimit: 10, wantKw: "Go 并发"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := c.in.Normalize()
			if got.Limit != c.wantLimit {
				t.Fatalf("limit 应收敛为 %d，实际 %d", c.wantLimit, got.Limit)
			}
			if got.Keyword != c.wantKw {
				t.Fatalf("keyword 应收敛为 %q，实际 %q", c.wantKw, got.Keyword)
			}
			// 游标与读过滤开关必须原样带下去：查询层拿不到就等于分页/过滤失效。
			if got.Before != c.in.Before {
				t.Fatal("游标不应被 Normalize 丢弃或替换")
			}
			if got.OnlyAdminAuthors != c.in.OnlyAdminAuthors {
				t.Fatal("读过滤开关不应被 Normalize 改动")
			}
		})
	}

	// 游标本身不改写（Negative case 的对照）：Normalize 只收敛 limit/keyword。
	if cursor.ViewedAt.IsZero() {
		t.Fatal("游标构造用例失效")
	}
}
