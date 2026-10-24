package domain_test

import (
	"testing"

	"myblogx/internal/media/domain"
)

func TestStatusFromSuggestion(t *testing.T) {
	cases := map[string]domain.Status{
		"pass":     domain.StatusPass,
		"review":   domain.StatusReviewing,
		"block":    domain.StatusBlocked,
		"deleted":  domain.StatusDeleted,
		"orphaned": domain.StatusOrphaned,
		" PASS ":   domain.StatusPass,
		"other":    domain.StatusUnknown,
	}
	for input, want := range cases {
		if got := domain.StatusFromSuggestion(input); got != want {
			t.Fatalf("StatusFromSuggestion(%q)=%v want %v", input, got, want)
		}
	}
}

func TestConfigMaxSize(t *testing.T) {
	cfg := domain.Config{SizeMB: 5}
	if got := cfg.MaxSize(); got != 5*1024*1024 {
		t.Fatalf("MaxSize=%d", got)
	}
}

func TestQueryNormalize(t *testing.T) {
	q := domain.Query{Page: 0, Limit: 0}.Normalize()
	if q.Page != 1 || q.Limit != 10 {
		t.Fatalf("默认分页错误: %+v", q)
	}
	if got := (domain.Query{Page: 3, Limit: 20}).Offset(); got != 40 {
		t.Fatalf("offset=%d", got)
	}
}
