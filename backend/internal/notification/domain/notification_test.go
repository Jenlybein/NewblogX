package domain_test

import (
	"testing"
	"time"

	"myblogx/internal/notification/domain"
)

func TestMessageGroupTypes(t *testing.T) {
	cases := map[domain.Group][]domain.MessageType{
		domain.GroupComment:   {domain.TypeCommentArticle, domain.TypeCommentReply},
		domain.GroupDiggFavor: {domain.TypeDiggArticle, domain.TypeDiggComment, domain.TypeFavorArticle},
		domain.GroupSystem:    {domain.TypeSystem},
	}
	for group, want := range cases {
		got := group.Types()
		if len(got) != len(want) {
			t.Fatalf("分组 %d 类型数量不符: %v", group, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("分组 %d 类型不符: %v", group, got)
			}
		}
	}
	if (domain.Group(0)).Valid() || (domain.Group(4)).Valid() || !(domain.Group(1)).Valid() {
		t.Fatal("分组合法性判断错误")
	}
}

func TestMessageTypeValues(t *testing.T) {
	// 这些数字是前端契约，改一个都会让历史消息落到别的分组。
	checks := map[domain.MessageType]int8{
		domain.TypeCommentArticle: 1,
		domain.TypeCommentReply:   2,
		domain.TypeDiggArticle:    3,
		domain.TypeUnDiggArticle:  4,
		domain.TypeDiggComment:    5,
		domain.TypeUnDiggComment:  6,
		domain.TypeFavorArticle:   7,
		domain.TypeUnFavorArticle: 8,
		domain.TypeSystem:         9,
	}
	for typ, want := range checks {
		if int8(typ) != want {
			t.Fatalf("消息类型 %d 应为 %d", typ, want)
		}
	}
}

func TestPageNormalize(t *testing.T) {
	p := domain.Page{Page: 0, Limit: 0}.Normalize()
	if p.Page != 1 || p.Limit != 10 {
		t.Fatalf("分页默认值错误: %+v", p)
	}
	if got := (domain.Page{Limit: 500}).Normalize().Limit; got != domain.MaxLimit {
		t.Fatalf("单页上限未收敛: %d", got)
	}
	if got := (domain.Page{Page: 3, Limit: 20}).Offset(); got != 40 {
		t.Fatalf("offset 计算错误: %d", got)
	}
}

func TestGlobalNotifVisibleAt(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	published := now.Add(-24 * time.Hour)
	registered := now.Add(-48 * time.Hour) // 公告发布前就注册，属"老用户"
	newUser := now.Add(-1 * time.Hour)     // 公告发布后才注册，属"新用户"

	base := domain.GlobalNotif{
		CreatedAt: published,
		ExpireAt:  now.Add(24 * time.Hour),
	}

	all := base
	all.VisibleRule = domain.VisibleAll
	if !all.VisibleAt(registered, now) || !all.VisibleAt(newUser, now) {
		t.Fatal("所有用户规则应对任意注册时间可见")
	}

	oldUsers := base
	oldUsers.VisibleRule = domain.VisibleRegistered
	if !oldUsers.VisibleAt(registered, now) {
		t.Fatal("老用户规则对发布前注册的用户应可见")
	}
	if oldUsers.VisibleAt(newUser, now) {
		t.Fatal("老用户规则对发布后注册的用户不应可见")
	}

	newUsers := base
	newUsers.VisibleRule = domain.VisibleNew
	if newUsers.VisibleAt(registered, now) {
		t.Fatal("新用户规则对发布前注册的用户不应可见")
	}
	if !newUsers.VisibleAt(newUser, now) {
		t.Fatal("新用户规则对发布后注册的用户应可见")
	}

	expired := all
	expired.ExpireAt = now.Add(-time.Second)
	if expired.VisibleAt(registered, now) {
		t.Fatal("已过期公告不应可见")
	}
}

func TestVisibleRuleEffective(t *testing.T) {
	if domain.VisibleRule(0).Effective() != domain.VisibleRegistered {
		t.Fatal("零值应收敛为默认规则")
	}
	if domain.VisibleRule(9).Effective() != domain.VisibleRegistered {
		t.Fatal("非法值应收敛为默认规则")
	}
	if domain.VisibleNew.Effective() != domain.VisibleNew || domain.VisibleAll.Effective() != domain.VisibleAll {
		t.Fatal("合法值应原样返回")
	}
}
