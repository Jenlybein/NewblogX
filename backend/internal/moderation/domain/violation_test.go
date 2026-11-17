package domain_test

import (
	"testing"
	"time"

	"myblogx/internal/moderation/domain"
)

// 本文件钉住 ADR-0035 的三条规则：分值、衰减、档位。
// 阈值是产品决策（100/120/150/200），所以每条边界都单独一条用例 ——
// 差一分就换了处罚档，这类错误在线上表现为"凭什么我被封 7 天"。

var now = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// event 造一条"刚发生"的事件。
func event(level domain.Level, ageDays float64) domain.Event {
	return domain.Event{
		Level:     level,
		Status:    domain.StatusActive,
		CreatedAt: now.Add(-time.Duration(ageDays * 24 * float64(time.Hour))),
	}
}

// TestLevelScore 钉住分值表。
func TestLevelScore(t *testing.T) {
	t.Parallel()

	cases := map[domain.Level]int{
		domain.LevelKeyword: 5,
		domain.LevelImage:   20,
		domain.LevelGeneral: 10,
		domain.LevelSevere:  50,
	}
	for level, want := range cases {
		if got := level.Score(); got != want {
			t.Fatalf("%s 分值 = %d，期望 %d", level, got, want)
		}
	}
	if got := domain.Level("nope").Score(); got != 0 {
		t.Fatalf("未知等级应得 0 分，实际 %d", got)
	}
}

// TestEffectiveScore_衰减边界 钉住半衰期与窗口。
func TestEffectiveScore_衰减边界(t *testing.T) {
	t.Parallel()

	// 刚发生：全额。
	if got := domain.EffectiveScore([]domain.Event{event(domain.LevelImage, 0)}, now); got != 20 {
		t.Fatalf("刚发生的 20 分事件应得 20，实际 %v", got)
	}
	// 一个半衰期（60 天）：一半。
	if got := domain.EffectiveScore([]domain.Event{event(domain.LevelImage, 60)}, now); got != 10 {
		t.Fatalf("60 天前的 20 分事件应得 10，实际 %v", got)
	}
	// 两个半衰期（120 天）：超出 90 天窗口，直接出局。
	if got := domain.EffectiveScore([]domain.Event{event(domain.LevelImage, 120)}, now); got != 0 {
		t.Fatalf("超出窗口的事件应得 0，实际 %v", got)
	}
}

// TestEffectiveScore_撤销不计 钉住"申诉成功后被扣的分能撤回"。
func TestEffectiveScore_撤销不计(t *testing.T) {
	t.Parallel()

	revoked := event(domain.LevelImage, 0)
	revoked.Status = domain.StatusRevoked

	if got := domain.EffectiveScore([]domain.Event{revoked}, now); got != 0 {
		t.Fatalf("撤销的事件不该计分，实际 %v", got)
	}
}

// TestDecide_档位边界 钉住 100 / 120 / 150 / 200 四条线。
func TestDecide_档位边界(t *testing.T) {
	t.Parallel()

	cases := []struct {
		score float64
		want  domain.Punishment
	}{
		{0, domain.PunishmentNone},
		{99.9, domain.PunishmentNone},
		{100, domain.PunishmentMute1Day},
		{119.9, domain.PunishmentMute1Day},
		{120, domain.PunishmentMute7Days},
		{149.9, domain.PunishmentMute7Days},
		{150, domain.PunishmentMute30Days},
		{199.9, domain.PunishmentMute30Days},
		{200, domain.PunishmentManualReview},
		{500, domain.PunishmentManualReview},
	}
	for _, c := range cases {
		if got := domain.Decide(c.score); got != c.want {
			t.Fatalf("分数 %v 应判 %s，实际 %s", c.score, c.want, got)
		}
	}
}

// TestPunishmentMuteDays 钉住"档位 → 天数"。
func TestPunishmentMuteDays(t *testing.T) {
	t.Parallel()

	if got := domain.PunishmentMute1Day.MuteDays(); got != 1 {
		t.Fatalf("应 1 天，实际 %d", got)
	}
	if got := domain.PunishmentMute7Days.MuteDays(); got != 7 {
		t.Fatalf("应 7 天，实际 %d", got)
	}
	if got := domain.PunishmentMute30Days.MuteDays(); got != 30 {
		t.Fatalf("应 30 天，实际 %d", got)
	}
	// 人工评估不是"自动禁言 0 天"，它根本不该走自动禁言那条路。
	if got := domain.PunishmentManualReview.MuteDays(); got != 0 {
		t.Fatalf("人工评估不该自动禁言，实际 %d 天", got)
	}
}

// TestEscalate_封禁期内再违规升一档 钉住"不在同一档循环"。
func TestEscalate_封禁期内再违规升一档(t *testing.T) {
	t.Parallel()

	if got := domain.Escalate(domain.PunishmentMute1Day, true); got != domain.PunishmentMute7Days {
		t.Fatalf("1 天档期内再违规应升到 7 天，实际 %s", got)
	}
	if got := domain.Escalate(domain.PunishmentMute7Days, true); got != domain.PunishmentMute30Days {
		t.Fatalf("7 天档期内再违规应升到 30 天，实际 %s", got)
	}
	if got := domain.Escalate(domain.PunishmentMute30Days, true); got != domain.PunishmentManualReview {
		t.Fatalf("30 天档期内再违规应转人工，实际 %s", got)
	}
	// 不在封禁期：按分数算出来的档位原样生效。
	if got := domain.Escalate(domain.PunishmentMute1Day, false); got != domain.PunishmentMute1Day {
		t.Fatalf("不在封禁期不应升档，实际 %s", got)
	}
	// 封禁期内的轻微违规也至少推一档。
	if got := domain.Escalate(domain.PunishmentNone, true); got != domain.PunishmentNone.Next() {
		t.Fatalf("封禁期内的轻微违规应升一档，实际 %s", got)
	}
}
