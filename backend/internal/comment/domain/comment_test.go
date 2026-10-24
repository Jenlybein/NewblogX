package domain_test

import (
	"testing"

	"myblogx/internal/comment/domain"
)

func TestCanDelete(t *testing.T) {
	comment := domain.Comment{UserID: 10, ArticleID: 99}

	cases := []struct {
		name      string
		viewerID  uint64
		admin     bool
		authorID  uint64
		wantAllow bool
	}{
		{name: "管理员可删", viewerID: 1, admin: true, authorID: 2, wantAllow: true},
		{name: "评论作者可删", viewerID: 10, authorID: 2, wantAllow: true},
		{name: "文章作者可删", viewerID: 2, authorID: 2, wantAllow: true},
		{name: "路人不可删", viewerID: 3, authorID: 2, wantAllow: false},
		{name: "匿名不可删", viewerID: 0, authorID: 2, wantAllow: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := comment.CanDelete(tc.viewerID, tc.admin, tc.authorID); got != tc.wantAllow {
				t.Fatalf("CanDelete=%v want %v", got, tc.wantAllow)
			}
		})
	}
}

func TestIsRoot(t *testing.T) {
	if !(domain.Comment{}).IsRoot() {
		t.Fatal("ReplyID/RootID 都为 0 应是一级评论")
	}
	if (domain.Comment{ReplyID: 1}).IsRoot() {
		t.Fatal("有 ReplyID 不是一级评论")
	}
	if (domain.Comment{RootID: 1}).IsRoot() {
		t.Fatal("有 RootID 不是一级评论")
	}
}

func TestPageNormalize(t *testing.T) {
	p := domain.Page{Page: 0, Limit: 0}.Normalize()
	if p.Page != 1 || p.Limit != 10 {
		t.Fatalf("默认分页应为 page=1 limit=10，得到 %+v", p)
	}
	p = domain.Page{Page: 2, Limit: 500}.Normalize()
	if p.Limit != 10 {
		t.Fatalf("超大 limit 应被收敛到 10，得到 %d", p.Limit)
	}
	if got := (domain.Page{Page: 3, Limit: 20}).Offset(); got != 40 {
		t.Fatalf("offset 应为 40，得到 %d", got)
	}
}
