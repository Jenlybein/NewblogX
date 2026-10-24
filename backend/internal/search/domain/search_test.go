package domain_test

import (
	"testing"

	"myblogx/internal/search/domain"
)

func TestRequestNormalize(t *testing.T) {
	req := domain.Request{}.Normalize()
	if req.Type != 1 || req.Sort != 1 {
		t.Fatalf("默认 type/sort 应为 1/1，得到 %d/%d", req.Type, req.Sort)
	}
	if req.PageMode != domain.PageModeHasMore {
		t.Fatalf("公共搜索默认 has_more，得到 %s", req.PageMode)
	}
}

func TestPageModeByType(t *testing.T) {
	for _, typ := range []int8{4, 5} {
		req := domain.Request{Type: typ}.Normalize()
		if req.PageMode != domain.PageModeCount {
			t.Fatalf("type=%d 默认应为 count，得到 %s", typ, req.PageMode)
		}
	}
}

func TestPageInfo(t *testing.T) {
	p := domain.PageInfo{Page: 0, Limit: 999}
	if p.NormalizePage() != 1 || p.GetLimit() != 10 {
		t.Fatalf("分页收敛错误: %+v", p)
	}
}
