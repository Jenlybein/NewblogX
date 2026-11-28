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

// newBridge 用给定词表与白名单构造桥。
//
// 桥**不再**接处置口径（2026-11-28 三档化）：它只报"哪一档真的被打星"这个事实，
// 命中之后文章去哪由用例层按站点档位决定 —— 这样 draft 与 reject 才能共用同一份事实。
func newBridge(t *testing.T, words []contentfilter.Word, whitelist []string,
) (articleTextFilterBridge, *fakeHitRepo) {
	t.Helper()
	engine, err := contentfilter.NewEngine(words, whitelist)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	repo := &fakeHitRepo{}
	svc := filterapp.New(engine, repo, &quietLogger{})
	return articleTextFilterBridge{svc: svc}, repo
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
		name      string
		whitelist []string
		text      string
		wantBlock bool
		wantWatch bool
		// wantRecords 是命中记录的条数：白名单覆盖的命中也会记一条（用于统计误伤），
		// 只有完全没命中才是 0。
		wantRecords int
	}{
		{
			name:        "block 命中被打星 → 报 block 档事实",
			text:        "代开发票找我",
			wantBlock:   true,
			wantRecords: 1,
		},
		{
			name:        "watch 命中被打星 → 报 watch 档事实（与 block 分开报）",
			text:        "想合作请加微信",
			wantWatch:   true,
			wantRecords: 1,
		},
		{
			name:        "两档同时命中 → 两个事实都为真（仍只记一条命中记录）",
			text:        "代开发票找我，也可以加微信",
			wantBlock:   true,
			wantWatch:   true,
			wantRecords: 1,
		},
		{
			name:        "block 命中被短语白名单完整覆盖 → 报未命中，但记录照写（用于统计误伤）",
			whitelist:   []string{"代开发票的科普"},
			text:        "代开发票的科普",
			wantRecords: 1,
		},
		{
			name: "没有命中 → 两个事实都为假",
			text: "这是一篇普通的科普文章",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bridge, repo := newBridge(t,
				[]contentfilter.Word{blockWord(), watchWord()}, tc.whitelist)

			got, err := bridge.FilterArticleText(context.Background(), articleapp.FilterText{
				UserID:    ctype.NewID(7),
				ContentID: ctype.NewID(9001),
				Text:      tc.text,
			})
			if err != nil {
				t.Fatalf("过滤不该返回错误: %v", err)
			}
			if got.BlockTierMasked != tc.wantBlock {
				t.Fatalf("BlockTierMasked 应为 %v，实际 %v（替换后文本 %q）",
					tc.wantBlock, got.BlockTierMasked, got.Masked)
			}
			if got.WatchTierMasked != tc.wantWatch {
				t.Fatalf("WatchTierMasked 应为 %v，实际 %v（替换后文本 %q）",
					tc.wantWatch, got.WatchTierMasked, got.Masked)
			}
			// 拒绝 ≠ 不记录：只要真有命中，命中记录就要在（且与档位无关）。
			// 没有命中的文本不该产生任何记录 —— 记录一条空的"命中"会让风险页多出噪音。
			if len(repo.created) != tc.wantRecords {
				t.Fatalf("命中记录应为 %d 条，实际 %d 条", tc.wantRecords, len(repo.created))
			}
			if tc.wantRecords > 0 && repo.created[0].Surface != filterdomain.SurfaceArticle {
				t.Fatalf("文本面应钉死为 article，实际 %q", repo.created[0].Surface)
			}
		})
	}
}

// TestBridgeReportsFactRegardlessOfDisposition 桥只报事实，不受站点档位影响。
//
// 三档化之后"拒不拒、退不退草稿"是用例层按现读档位决定的（见
// internal/article/app/filter.go 的 dispositionAfterFilter），桥这一层必须与档位
// **无关** —— 否则同一次命中在 draft 档下会被判成"没命中"，命中处置就整条失效。
func TestBridgeReportsFactRegardlessOfDisposition(t *testing.T) {
	t.Parallel()

	bridge, _ := newBridge(t, []contentfilter.Word{blockWord()}, nil)

	in := articleapp.FilterText{UserID: ctype.NewID(7), ContentID: ctype.NewID(9001), Text: "代开发票找我"}
	got, err := bridge.FilterArticleText(context.Background(), in)
	if err != nil {
		t.Fatalf("过滤不该返回错误: %v", err)
	}
	if !got.BlockTierMasked {
		t.Fatal("block 档真被打星时必须报事实（档位由用例层按现读配置判）")
	}
}
