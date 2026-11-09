//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/social/adapter/mysql"
	"myblogx/internal/social/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是关系三视图在**查询层**的契约测试（票 327）。
//
// 为什么必须真 MySQL：三件关键行为都长在 SQL 上，内存 fake 验不到。
//  1. 互关是"我关注的人里也关注我的"，靠一条相关子查询筛；筛的位置错了
//     （先分页后过滤）每页条数就会随关系分布抖动。
//  2. 软删的行不能出现在任何视图或计数里（GORM 只会给 Model 查询加
//     deleted_at IS NULL，子查询、Count 都得自己落在正确的 builder 上）。
//  3. 移除粉丝只是删掉**一个方向**的行，另一个方向必须还在。

const (
	relMe   uint64 = 100
	relBee  uint64 = 200
	relCat  uint64 = 300
	relDee  uint64 = 400
	relFake uint64 = 500
)

func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.Social)
	return db
}

// seedRelations 造出四种关系：
//
//	我 → B        单向关注
//	我 ↔ C        互关
//	D → 我        单向粉丝
//
// 另有一条软删过的 "我 → Fake"，用来证明它不进任何视图与计数。
func seedRelations(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, spec := range []struct {
		id   uint64
		name string
	}{
		{relMe, "我"},
		{relBee, "B"},
		{relCat, "C"},
		{relDee, "D"},
		{relFake, "Fake"},
	} {
		schema.InsertUser(t, db, schema.UserSpec{ID: spec.id, Username: "rel_" + spec.name, Nickname: spec.name})
	}

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	// 我关注 B、C（B 在前，用于验证排序）。
	schema.InsertUserFollow(t, db, schema.UserFollowSpec{
		ID: 1, FollowedUserID: relBee, FollowedName: "B", FansUserID: relMe, FansName: "我", CreatedAt: base,
	})
	schema.InsertUserFollow(t, db, schema.UserFollowSpec{
		ID: 2, FollowedUserID: relCat, FollowedName: "C", FansUserID: relMe, FansName: "我", CreatedAt: base.Add(time.Minute),
	})
	// C 回关我 → 互关；D 关注我 → 单向粉丝。
	schema.InsertUserFollow(t, db, schema.UserFollowSpec{
		ID: 3, FollowedUserID: relMe, FollowedName: "我", FansUserID: relCat, FansName: "C", CreatedAt: base.Add(2 * time.Minute),
	})
	schema.InsertUserFollow(t, db, schema.UserFollowSpec{
		ID: 4, FollowedUserID: relMe, FollowedName: "我", FansUserID: relDee, FansName: "D", CreatedAt: base.Add(3 * time.Minute),
	})
	// 软删过的一条：不能出现在任何视图或计数里。
	schema.InsertUserFollow(t, db, schema.UserFollowSpec{
		ID: 5, FollowedUserID: relFake, FollowedName: "Fake", FansUserID: relMe, FansName: "我", CreatedAt: base.Add(4 * time.Minute),
	})
	if err := db.Exec("UPDATE user_follow_models SET deleted_at = NOW() WHERE id = ?", 5).Error; err != nil {
		t.Fatalf("软删关系行失败: %v", err)
	}
}

func counterpartIDs(rows []domain.Follow, kind domain.RelationKind) []uint64 {
	out := make([]uint64, 0, len(rows))
	for _, row := range rows {
		id, _, _, _ := row.Counterpart(kind)
		out = append(out, id.Uint64())
	}
	return out
}

// TestListRelationsThreeViews 验证三个视图各取各的方向，且互关在 SQL 里筛。
func TestListRelationsThreeViews(t *testing.T) {
	db := migrate(t)
	seedRelations(t, db)
	repo := mysql.New(db)
	ctx := context.Background()
	page := domain.Page{Page: 1, Limit: 10}

	following, hasMore, err := repo.ListRelations(ctx, relMe, domain.KindFollow, page)
	if err != nil {
		t.Fatalf("列关注失败: %v", err)
	}
	if got := counterpartIDs(following, domain.KindFollow); len(got) != 2 || got[0] != relBee || got[1] != relCat {
		t.Fatalf("关注视图应按创建时间升序给出 B、C，实际 %v", got)
	}
	if hasMore {
		t.Fatal("关注视图只有 2 条，不该 has_more")
	}

	fans, _, err := repo.ListRelations(ctx, relMe, domain.KindFans, page)
	if err != nil {
		t.Fatalf("列粉丝失败: %v", err)
	}
	if got := counterpartIDs(fans, domain.KindFans); len(got) != 2 || got[0] != relCat || got[1] != relDee {
		t.Fatalf("粉丝视图应给出 C、D（对方是粉丝那一端），实际 %v", got)
	}

	mutual, _, err := repo.ListRelations(ctx, relMe, domain.KindMutual, page)
	if err != nil {
		t.Fatalf("列互关失败: %v", err)
	}
	if got := counterpartIDs(mutual, domain.KindMutual); len(got) != 1 || got[0] != relCat {
		t.Fatalf("互关视图只该有 C，实际 %v", got)
	}

	// 分页发生在过滤之后：limit=1 时互关第一页有 1 条、且没有更多。
	first, more, err := repo.ListRelations(ctx, relMe, domain.KindMutual, domain.Page{Page: 1, Limit: 1})
	if err != nil {
		t.Fatalf("列互关（分页）失败: %v", err)
	}
	if len(first) != 1 || more {
		t.Fatalf("互关 limit=1 应返回 1 条且无更多，实际 len=%d hasMore=%v", len(first), more)
	}
}

// TestCountRelationsMatchesViews 验证三个计数与三个视图一致。
func TestCountRelationsMatchesViews(t *testing.T) {
	db := migrate(t)
	seedRelations(t, db)
	repo := mysql.New(db)

	counts, err := repo.CountRelations(context.Background(), relMe)
	if err != nil {
		t.Fatalf("取关系计数失败: %v", err)
	}
	if counts.Follow != 2 || counts.Fans != 2 || counts.Mutual != 1 {
		t.Fatalf("计数应为 follow=2 fans=2 mutual=1，实际 %+v", counts)
	}
}

// TestRemoveFanDropsOneDirection 验证移除粉丝只删一个方向。
func TestRemoveFanDropsOneDirection(t *testing.T) {
	db := migrate(t)
	seedRelations(t, db)
	repo := mysql.New(db)
	ctx := context.Background()

	// Unfollow(我, D) 删的是"D 关注我"这一条——用例层就是移除粉丝。
	removed, err := repo.Unfollow(ctx, relMe, relDee)
	if err != nil {
		t.Fatalf("移除粉丝失败: %v", err)
	}
	if !removed {
		t.Fatal("D 本来就关注我，移除应当成功")
	}

	fans, _, err := repo.ListRelations(ctx, relMe, domain.KindFans, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("列粉丝失败: %v", err)
	}
	if got := counterpartIDs(fans, domain.KindFans); len(got) != 1 || got[0] != relCat {
		t.Fatalf("移除后粉丝只剩 C，实际 %v", got)
	}

	// 反向没有被误删：我关注 B、C 仍在。
	following, _, err := repo.ListRelations(ctx, relMe, domain.KindFollow, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("列关注失败: %v", err)
	}
	if len(following) != 2 {
		t.Fatalf("移除粉丝不该影响我关注的人，实际 %d 条", len(following))
	}

	counts, err := repo.CountRelations(ctx, relMe)
	if err != nil {
		t.Fatalf("取关系计数失败: %v", err)
	}
	if counts.Fans != 1 || counts.Mutual != 1 {
		t.Fatalf("移除后粉丝应为 1（互关不变），实际 %+v", counts)
	}

	// 再移除一次同一个粉丝：已经没有了，必须返回 false（用例层据此报「不是你的粉丝」）。
	again, err := repo.Unfollow(ctx, relMe, relDee)
	if err != nil {
		t.Fatalf("重复移除失败: %v", err)
	}
	if again {
		t.Fatal("重复移除同一个人不该再删掉任何行")
	}
}
