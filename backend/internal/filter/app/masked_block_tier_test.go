package app_test

import (
	"context"
	"testing"

	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// BlockTierMasked 只报事实：这段文本里有没有 block 档的命中**真被打星**。
//
// 它是"命中即拒"的判据来源，所以三条边界都要钉住：
//   - block 命中且打星 → true；
//   - 只有 watch 命中（打星了也不拒）→ false；
//   - block 命中被白名单完整覆盖（文本没变）→ false（否则白名单等于失效）。
//
// 与引擎侧的 TestHitMaskedTracksActualReplacement 是两层：那边断言单条命中的事实，
// 这边断言用例层把它汇总成"这段文本要不要拦"的那个布尔。

func newMixedEngine(t *testing.T, whitelist []string) *contentfilter.Engine {
	t.Helper()
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{
			{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs},
			{Word: "加微信", Tier: contentfilter.TierWatch, Category: contentfilter.CategoryFraud},
		},
		whitelist,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	return engine
}

func TestFilterReportsMaskedBlockTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		whitelist []string
		text      string
		want      bool
		// wantHits 是这条用例是否应写入命中记录（拒绝 ≠ 不记录：这里记录总会写，
		// 拒发结论在调用方那一步才出现）。
		wantHits bool
	}{
		{
			name:     "block 命中被打星 → true",
			text:     "我这里有冰毒",
			want:     true,
			wantHits: true,
		},
		{
			name:     "只有 watch 命中 → false（watch 档本期不参与拒发）",
			text:     "加微信详聊",
			want:     false,
			wantHits: true,
		},
		{
			name:      "block 命中被短语白名单豁免 → false",
			whitelist: []string{"冰毒科普"},
			text:      "冰毒科普",
			want:      false,
			wantHits:  true,
		},
		{
			name:     "没有命中 → false",
			text:     "一句正常的话",
			want:     false,
			wantHits: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := app.New(newMixedEngine(t, tc.whitelist), repo, &fakeLogger{})

			out, err := svc.Filter(context.Background(), app.FilterInput{
				Surface:   domain.SurfaceArticle,
				UserID:    ctype.NewID(7),
				ContentID: ctype.NewID(9001),
				Text:      tc.text,
			})
			if err != nil {
				t.Fatalf("不该返回错误: %v", err)
			}
			if out.BlockTierMasked != tc.want {
				t.Fatalf("BlockTierMasked 应为 %v，实际 %v（文本 %q 被替换成 %q）",
					tc.want, out.BlockTierMasked, tc.text, out.Masked)
			}
			if got := len(repo.created) > 0; got != tc.wantHits {
				t.Fatalf("命中记录写入应为 %v，实际 %d 条", tc.wantHits, len(repo.created))
			}
		})
	}
}

// TestFilterRecordsHitEvenWhenBlockTierMasked 把"拒绝 ≠ 不记录"这条口径钉在用例层：
// 命中记录在返回结果之前就写好了，调用方后面怎么处置都不影响这次留痕。
func TestFilterRecordsHitEvenWhenBlockTierMasked(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{}
	svc := app.New(newMixedEngine(t, nil), repo, &fakeLogger{})

	out, err := svc.Filter(context.Background(), app.FilterInput{
		Surface:   domain.SurfaceArticle,
		UserID:    ctype.NewID(7),
		ContentID: ctype.NewID(9002),
		Text:      "这里卖冰毒",
	})
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if !out.BlockTierMasked {
		t.Fatal("block 命中被打星时 BlockTierMasked 应为 true")
	}
	if len(repo.created) != 1 {
		t.Fatalf("即使调用方之后要拒发，命中记录也必须已写好，实际 %d 条", len(repo.created))
	}
	if repo.created[0].Surface != domain.SurfaceArticle {
		t.Fatalf("文本面应为 article，实际 %q", repo.created[0].Surface)
	}
}
