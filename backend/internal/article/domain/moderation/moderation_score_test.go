package moderation_test

import (
	"testing"

	"myblogx/internal/article/domain/moderation"
)

// 低分标签复用评分记录的总分，不重新调用模型（票 311 / spec #302 决策 20）。
func TestScoreSignalsLowScore(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		score int
		want  bool
	}{
		{name: "30 分属于低分", score: 30, want: true},
		{name: "59 分属于低分", score: 59, want: true},
		{name: "60 分不再标低分", score: 60, want: false},
		{name: "90 分不标低分", score: 90, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := moderation.ScoreSignals(&moderation.ScoreFacts{TotalScore: tc.score, ContentLength: 1000})
			hasLow := false
			for _, s := range got {
				if s.Code == moderation.CodeLowScore {
					hasLow = true
					if s.Level != moderation.LevelMedium {
						t.Fatalf("低分标签应为 medium，实际 %s", s.Level)
					}
				}
			}
			if hasLow != tc.want {
				t.Fatalf("低分标签=%v，期望 %v（信号 %+v）", hasLow, tc.want, got)
			}
		})
	}
}

// 维度塌陷：任一维度低于 30 分即视为塌陷，点名是哪个维度；JSON 坏掉不报错。
func TestScoreSignalsDimensionCollapse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		dimensions string
		wantDetail string
	}{
		{
			name:       "清晰度塌陷",
			dimensions: `[{"name":"clarity","score":20},{"name":"structure","score":80}]`,
			wantDetail: "clarity",
		},
		{
			name:       "两个维度同时塌陷按顺序点名",
			dimensions: `[{"name":"clarity","score":10},{"name":"language","score":0}]`,
			wantDetail: "clarity,language",
		},
		{
			name:       "全部健康",
			dimensions: `[{"name":"clarity","score":60},{"name":"structure","score":80}]`,
			wantDetail: "",
		},
		{
			name:       "不是 JSON 数组",
			dimensions: `{"clarity":20}`,
			wantDetail: "",
		},
		{
			name:       "空白",
			dimensions: "",
			wantDetail: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := moderation.ScoreSignals(&moderation.ScoreFacts{
				TotalScore:     80,
				DimensionsJSON: tc.dimensions,
				ContentLength:  1000,
			})
			hasCollapse := false
			for _, s := range got {
				if s.Code == moderation.CodeDimensionCollapse {
					hasCollapse = true
					if s.Level != moderation.LevelLow {
						t.Fatalf("维度塌陷应为 low，实际 %s", s.Level)
					}
					if s.Detail != tc.wantDetail {
						t.Fatalf("塌陷维度应为 %q，实际 %q", tc.wantDetail, s.Detail)
					}
				}
			}
			if want := tc.wantDetail != ""; hasCollapse != want {
				t.Fatalf("维度塌陷=%v，期望 %v（信号 %+v）", hasCollapse, want, got)
			}
		})
	}
}

// 篇幅异常用评分记录里的 content_length（评分时按 rune 计数），
// 两端各有明确边界；0 表示该条记录没有可用篇幅，不当作异常。
func TestScoreSignalsLengthAbnormal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		length int
		want   bool
	}{
		{name: "199 字过短", length: 199, want: true},
		{name: "200 字是下边界", length: 200, want: false},
		{name: "20000 字是上边界", length: 20000, want: false},
		{name: "20001 字过长", length: 20001, want: true},
		{name: "0 表示没有可用篇幅", length: 0, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := moderation.ScoreSignals(&moderation.ScoreFacts{TotalScore: 80, ContentLength: tc.length})
			hasLength := false
			for _, s := range got {
				if s.Code == moderation.CodeLengthAbnormal {
					hasLength = true
					if s.Level != moderation.LevelLow {
						t.Fatalf("篇幅异常应为 low，实际 %s", s.Level)
					}
				}
			}
			if hasLength != tc.want {
				t.Fatalf("篇幅异常=%v，期望 %v（信号 %+v）", hasLength, tc.want, got)
			}
		})
	}
}

// 没有评分记录的文章：只有词表层的结果，不报错（票 311 验收标准 5）。
func TestEvaluateWithoutScoreRecord(t *testing.T) {
	t.Parallel()

	got := moderation.Evaluate("一段普通正文", moderation.WordLists{}, nil)
	if got.Level != moderation.LevelNone {
		t.Fatalf("无词表命中、无评分时应为 none，实际 %s", got.Level)
	}
	if len(got.Signals) != 0 || len(got.Labels()) != 0 {
		t.Fatalf("不应有任何信号，实际 %+v", got.Signals)
	}
}

// 两层同时命中时取最严重等级，标签按首次出现顺序去重。
func TestEvaluateTakesHighestLevelAndDeduplicatesLabels(t *testing.T) {
	t.Parallel()

	lists := moderation.WordLists{
		Block: []string{"代开发票"},
		Watch: []string{"加微信", "私聊"},
	}
	score := &moderation.ScoreFacts{
		TotalScore:     45,
		DimensionsJSON: `[{"name":"clarity","score":10},{"name":"structure","score":70}]`,
		ContentLength:  120,
	}
	got := moderation.Evaluate("代开发票请加微信，也欢迎私聊。", lists, score)

	if got.Level != moderation.LevelHigh {
		t.Fatalf("阻断级命中应把整体风险抬到 high，实际 %s", got.Level)
	}
	want := []string{
		"block_word",
		"watch_word",
		"low_score",
		"dimension_collapse",
		"length_abnormal",
	}
	labels := got.Labels()
	if len(labels) != len(want) {
		t.Fatalf("标签应为 %v，实际 %v", want, labels)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("标签应为 %v，实际 %v", want, labels)
		}
	}
}
