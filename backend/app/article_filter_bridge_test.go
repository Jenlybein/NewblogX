package app

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	articleapp "myblogx/internal/article/app"
	filterapp "myblogx/internal/filter/app"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// 装配层这一座桥的职责只有一个：把"命中事实"与"站点处置口径"合取成给文章模块
// 的结论。这里钉住三件事：
//   - block 档真被打星的命中 + 配了 reject → 报"命中即拒"；
//   - 没配 reject（默认 / 空串 / 读不到运行时配置）→ 与改造前一样只打星；
//   - 无论报不报拒发，命中记录都已经写好（拒绝 ≠ 不记录）。
//
// 最后一件事是这张票据里最容易做错的地方：把记录写在"判定之后"，
// 一次拒发就等于把证据也丢掉，风险页上什么都看不到。

// fakeHitRepo 是命中记录仓储的替身：只实现 Create，其余方法用"查不到 / 不更新"占位。
type fakeHitRepo struct {
	created []filterdomain.Hit
}

func (r *fakeHitRepo) Create(_ context.Context, hit *filterdomain.Hit) error {
	r.created = append(r.created, *hit)
	return nil
}

func (r *fakeHitRepo) List(context.Context, filterdomain.Query) ([]filterdomain.Hit, int64, error) {
	return nil, 0, nil
}

func (r *fakeHitRepo) TopUsers(context.Context, time.Time, int) ([]filterdomain.TopUser, error) {
	return nil, nil
}

func (r *fakeHitRepo) FindByID(context.Context, ctype.ID) (filterdomain.Hit, error) {
	return filterdomain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
}

func (r *fakeHitRepo) MarkHandled(context.Context, ctype.ID, filterdomain.Handle) (bool, error) {
	return false, nil
}

func (r *fakeHitRepo) PurgeBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func (r *fakeHitRepo) WordStats(context.Context, time.Time) ([]filterdomain.WordStat, error) {
	return nil, nil
}

// quietLogger 只用来满足 domain.Logger；这条路不该产生 error 日志。
type quietLogger struct{ errors int }

func (l *quietLogger) Errorf(string, ...any) { l.errors++ }

// newBridge 用给定词表、白名单与处置口径构造桥（处置口径为 nil 表示没注入）。
func newBridge(t *testing.T, words []contentfilter.Word, whitelist []string,
	disposition func() bool,
) (articleTextFilterBridge, *fakeHitRepo) {
	t.Helper()
	engine, err := contentfilter.NewEngine(words, whitelist)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	repo := &fakeHitRepo{}
	svc := filterapp.New(engine, repo, &quietLogger{})
	return articleTextFilterBridge{svc: svc, disposition: disposition}, repo
}

// blockWord 是"拦截词"，watchWord 是"观察词"（两档都打星，只有 block 参与拒发）。
func blockWord() contentfilter.Word {
	return contentfilter.Word{Word: "代开发票", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryFakeDocs}
}

func watchWord() contentfilter.Word {
	return contentfilter.Word{Word: "加微信", Tier: contentfilter.TierWatch, Category: contentfilter.CategoryFraud}
}

func TestBridgeReportsBlockRejectOnlyWhenConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		disposition func() bool
		whitelist   []string
		text        string
		want        bool
	}{
		{
			name:        "配了 reject + block 命中被打星 → 报命中即拒",
			disposition: func() bool { return true },
			text:        "代开发票找我",
			want:        true,
		},
		{
			name:        "默认（disposition 为 false）→ 只打星，不拒发",
			disposition: func() bool { return false },
			text:        "代开发票找我",
			want:        false,
		},
		{
			name:        "没注入 disposition（装配遗漏）→ 按默认 mask 处理，不拒发",
			disposition: nil,
			text:        "代开发票找我",
			want:        false,
		},
		{
			name:        "配了 reject 但只有 watch 档命中 → 不拒发（watch 本期只有 mask）",
			disposition: func() bool { return true },
			text:        "想合作请加微信",
			want:        false,
		},
		{
			name:        "配了 reject 但 block 命中被短语白名单完整覆盖 → 不拒发（白名单要生效）",
			disposition: func() bool { return true },
			whitelist:   []string{"代开发票的科普"},
			text:        "代开发票的科普",
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bridge, repo := newBridge(t,
				[]contentfilter.Word{blockWord(), watchWord()}, tc.whitelist, tc.disposition)

			got, err := bridge.FilterArticleText(context.Background(), articleapp.FilterText{
				UserID:    ctype.NewID(7),
				ContentID: ctype.NewID(9001),
				Text:      tc.text,
			})
			if err != nil {
				t.Fatalf("过滤不该返回错误: %v", err)
			}
			if got.BlockTierMasked != tc.want {
				t.Fatalf("BlockTierMasked 应为 %v，实际 %v（替换后文本 %q）",
					tc.want, got.BlockTierMasked, got.Masked)
			}
			// 拒绝 ≠ 不记录：不管这条最终拒不拒，命中记录都要在。
			if len(repo.created) != 1 {
				t.Fatalf("命中记录应已写好，实际 %d 条", len(repo.created))
			}
			if repo.created[0].Surface != filterdomain.SurfaceArticle {
				t.Fatalf("文本面应钉死为 article，实际 %q", repo.created[0].Surface)
			}
		})
	}
}

// TestBridgeBlockRejectIsReadLive 处置口径必须**每次现读**：后台改完配置之后，
// 下一次过滤就该按新值走，而不是"改配置要重启"。
func TestBridgeBlockRejectIsReadLive(t *testing.T) {
	t.Parallel()

	reject := true
	bridge, _ := newBridge(t,
		[]contentfilter.Word{blockWord()}, nil, func() bool { return reject })

	in := articleapp.FilterText{UserID: ctype.NewID(7), ContentID: ctype.NewID(9001), Text: "代开发票找我"}
	got, err := bridge.FilterArticleText(context.Background(), in)
	if err != nil {
		t.Fatalf("过滤不该返回错误: %v", err)
	}
	if !got.BlockTierMasked {
		t.Fatal("配了 reject 时应报命中即拒")
	}

	// 后台把配置改回 mask：同一条桥的下一次调用必须立刻跟着变。
	reject = false
	got, err = bridge.FilterArticleText(context.Background(), in)
	if err != nil {
		t.Fatalf("过滤不该返回错误: %v", err)
	}
	if got.BlockTierMasked {
		t.Fatal("改成 mask 之后不该再拒发（说明口径是启动时捕获的，不是现读）")
	}
}
