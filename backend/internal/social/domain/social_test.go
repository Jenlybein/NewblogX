package domain_test

import (
	"testing"

	"myblogx/internal/kernel/ctype"
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

// TestParseRelationKind 钉住三个视图的取值：多一个少一个都算接口变了。
func TestParseRelationKind(t *testing.T) {
	ok := []struct {
		raw  string
		want domain.RelationKind
	}{
		{"follow", domain.KindFollow},
		{"fans", domain.KindFans},
		{"mutual", domain.KindMutual},
	}
	for _, c := range ok {
		if got, valid := domain.ParseRelationKind(c.raw); !valid || got != c.want {
			t.Fatalf("ParseRelationKind(%q) = %q,%v；期望 %q,true", c.raw, got, valid, c.want)
		}
	}

	// 不认识的取值一律拒绝：猜一个默认视图会让调用方静默拿到错的数据。
	for _, raw := range []string{"", "followers", "FOLLOW", "mutual ", "0"} {
		if got, valid := domain.ParseRelationKind(raw); valid {
			t.Fatalf("ParseRelationKind(%q) 应拒绝，实际返回 %q", raw, got)
		}
	}
}

// TestFollowCounterpart 钉住"每行出现哪一端"：粉丝视图取粉丝，其余取被关注者。
func TestFollowCounterpart(t *testing.T) {
	row := domain.Follow{
		FollowedUserID:   ctype.NewID(200),
		FollowedNickname: "被关注者",
		FansUserID:       ctype.NewID(300),
		FansNickname:     "粉丝",
	}

	id, nickname, _, _ := row.Counterpart(domain.KindFollow)
	if id.Uint64() != 200 || nickname != "被关注者" {
		t.Fatalf("关注视图应取被关注者，实际 %s/%s", id, nickname)
	}

	id, nickname, _, _ = row.Counterpart(domain.KindMutual)
	if id.Uint64() != 200 || nickname != "被关注者" {
		t.Fatalf("互关视图取被关注者（互关行来自关注列表），实际 %s/%s", id, nickname)
	}

	id, nickname, _, _ = row.Counterpart(domain.KindFans)
	if id.Uint64() != 300 || nickname != "粉丝" {
		t.Fatalf("粉丝视图应取粉丝，实际 %s/%s", id, nickname)
	}
}
