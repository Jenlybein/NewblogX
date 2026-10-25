package domain_test

import (
	"testing"

	"myblogx/internal/social/domain"
)

func TestRelationValues(t *testing.T) {
	if domain.RelationStranger.Int8() != 1 ||
		domain.RelationFollowed.Int8() != 2 ||
		domain.RelationFans.Int8() != 3 ||
		domain.RelationFriend.Int8() != 4 {
		t.Fatal("关系枚举数值必须与旧契约一致（1..4）")
	}
}

func TestPageNormalize(t *testing.T) {
	p := domain.Page{Page: 0, Limit: 0}.Normalize()
	if p.Page != 1 || p.Limit != 10 {
		t.Fatalf("默认分页错误: %+v", p)
	}
	if got := (domain.Page{Page: 2, Limit: 20}).Offset(); got != 20 {
		t.Fatalf("offset=%d", got)
	}
}
