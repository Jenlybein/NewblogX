//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	filtermysql "myblogx/internal/filter/adapter/mysql"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// migrate 建命中表与用户表（昵称是现查的展示字段，需要 user_models）。
//
// 命中表没有外键，所以顺序不敏感；用户表必须在，否则"读面补昵称"那条用例
// 会因为表不存在而报错，看起来像仓储的 bug。
func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ContentFilter)
	return db
}

// seedUsers 插入两个用户（Top 榜要现查昵称）。
func seedUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{ID: 7, Username: "spammer", Nickname: "惯犯"})
	schema.InsertUser(t, db, schema.UserSpec{ID: 8, Username: "normal", Nickname: "正常人"})
}

// sampleHit 构造一条命中记录。
func sampleHit(surface domain.Surface, contentID, userID uint64, at time.Time) *domain.Hit {
	return &domain.Hit{
		Surface:   surface,
		ContentID: ctype.NewID(contentID),
		UserID:    ctype.NewID(userID),
		Words: []domain.Word{{
			Word:     "冰毒",
			Tier:     contentfilter.TierBlock,
			Category: contentfilter.CategoryDrugs,
			Snippets: []domain.Snippet{{Start: 2, End: 4, Text: "这里有冰毒"}},
		}},
		HitCount:  1,
		Status:    domain.StatusPending,
		CreatedAt: at,
	}
}

// TestCreateAndListRoundTripsHit 钉住"写进去的形状"与"读出来的形状"一致。
func TestCreateAndListRoundTripsHit(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	repo := filtermysql.New(db)
	ctx := context.Background()

	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	if err := repo.Create(ctx, sampleHit(domain.SurfaceComment, 9001, 7, at)); err != nil {
		t.Fatalf("写入命中记录失败（schema.sql 与行结构可能不一致）: %v", err)
	}

	rows, total, err := repo.List(ctx, domain.Query{})
	if err != nil {
		t.Fatalf("读取命中记录失败: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("应有 1 条记录，实际 total=%d rows=%d", total, len(rows))
	}
	got := rows[0]
	if got.Surface != domain.SurfaceComment {
		t.Fatalf("文本面没落对: %q", got.Surface)
	}
	if got.ContentID != ctype.NewID(9001) || got.UserID != ctype.NewID(7) {
		t.Fatalf("内容 / 用户 ID 没落对: %+v", got)
	}
	if got.Status != domain.StatusPending {
		t.Fatalf("新记录应是待处理，实际 %q", got.Status)
	}
	if got.HitCount != 1 {
		t.Fatalf("命中处数没落对: %d", got.HitCount)
	}
	if got.UserNickname != "惯犯" {
		t.Fatalf("读面应现查昵称，实际 %q", got.UserNickname)
	}
	if len(got.Words) != 1 || got.Words[0].Word != "冰毒" {
		t.Fatalf("命中明细没往返成功: %+v", got.Words)
	}
	if got.Words[0].Tier != contentfilter.TierBlock || got.Words[0].Category != contentfilter.CategoryDrugs {
		t.Fatalf("档位 / 类目没往返成功: %+v", got.Words[0])
	}
	if len(got.Words[0].Snippets) != 1 {
		t.Fatalf("上下文片段没往返成功: %+v", got.Words[0].Snippets)
	}
	if snippet := got.Words[0].Snippets[0]; snippet.Text != "这里有冰毒" || snippet.Start != 2 || snippet.End != 4 {
		t.Fatalf("上下文片段下标没往返成功: %+v", snippet)
	}
}

// TestListFiltersBySurfaceStatusAndUser 钉住风险页的三个筛选项与分页。
func TestListFiltersBySurfaceStatusAndUser(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	repo := filtermysql.New(db)
	ctx := context.Background()

	base := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	older := sampleHit(domain.SurfaceComment, 9001, 7, base.Add(-2*time.Hour))
	ignored := sampleHit(domain.SurfaceComment, 9002, 7, base.Add(-time.Hour))
	ignored.Status = domain.StatusIgnored
	newer := sampleHit(domain.SurfaceComment, 9003, 7, base)
	nickname := sampleHit(domain.SurfaceNickname, 8, 7, base.Add(-3*time.Hour))
	for _, hit := range []*domain.Hit{older, ignored, newer, nickname} {
		if err := repo.Create(ctx, hit); err != nil {
			t.Fatalf("写入命中记录失败: %v", err)
		}
	}

	cases := []struct {
		name string
		q    domain.Query
		want int64
	}{
		{"不筛选就是全部", domain.Query{}, 4},
		{"按文本面", domain.Query{Surface: domain.SurfaceNickname}, 1},
		{"按状态", domain.Query{Status: domain.StatusIgnored}, 1},
		{"按用户", domain.Query{UserID: ctype.NewID(7)}, 4},
		{"按用户与面组合", domain.Query{UserID: ctype.NewID(7), Surface: domain.SurfaceComment}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := repo.List(ctx, tc.q)
			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}
			if total != tc.want || int64(len(rows)) != tc.want {
				t.Fatalf("应有 %d 条，实际 total=%d rows=%d", tc.want, total, len(rows))
			}
		})
	}

	// 分页：按 created_at 倒序，第一页拿到最新两条，第二页接着往下。
	rows, total, err := repo.List(ctx, domain.Query{Page: domain.Page{Page: 1, Limit: 2}})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if total != 4 || len(rows) != 2 {
		t.Fatalf("第一页应有 2 条（总数 4），实际 total=%d rows=%d", total, len(rows))
	}
	if rows[0].ContentID != ctype.NewID(9003) || rows[1].ContentID != ctype.NewID(9002) {
		t.Fatalf("第一页应是时间最新的两条（9003、9002），实际 %v、%v",
			rows[0].ContentID, rows[1].ContentID)
	}
	second, _, err := repo.List(ctx, domain.Query{Page: domain.Page{Page: 2, Limit: 2}})
	if err != nil {
		t.Fatalf("第二页查询失败: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("第二页应有 2 条，实际 %d", len(second))
	}
	if second[0].ContentID != ctype.NewID(9001) || second[1].ContentID != ctype.NewID(8) {
		t.Fatalf("第二页应是接着的两条（9001、8），实际 %v、%v",
			second[0].ContentID, second[1].ContentID)
	}
}

// TestTopUsersAggregatesWindowAndHydratesNickname 钉住"命中用户 Top"的口径。
func TestTopUsersAggregatesWindowAndHydratesNickname(t *testing.T) {
	db := migrate(t)
	seedUsers(t, db)
	repo := filtermysql.New(db)
	ctx := context.Background()

	now := time.Now()
	// 用户 7：窗口内两处 + 窗口外一处（100 天前，不该计入）。
	hits := []*domain.Hit{
		sampleHit(domain.SurfaceComment, 9001, 7, now.Add(-time.Hour)),
		sampleHit(domain.SurfaceComment, 9002, 7, now.Add(-2*time.Hour)),
		sampleHit(domain.SurfaceComment, 9003, 7, now.AddDate(0, 0, -100)),
		sampleHit(domain.SurfaceNickname, 8, 8, now.Add(-time.Hour)),
	}
	hits[0].HitCount = 3
	hits[1].HitCount = 2
	hits[2].HitCount = 100
	for _, hit := range hits {
		if err := repo.Create(ctx, hit); err != nil {
			t.Fatalf("写入命中记录失败: %v", err)
		}
	}

	since := time.Now().AddDate(0, 0, -domain.TopWindowDays)
	top, err := repo.TopUsers(ctx, since, 5)
	if err != nil {
		t.Fatalf("读取 Top 失败: %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("应有 2 个用户上榜，实际 %d", len(top))
	}
	if top[0].UserID != ctype.NewID(7) || top[0].HitCount != 5 {
		t.Fatalf("命中处数最多的应是用户 7（3+2=5，不含窗口外的 100），实际 %+v", top[0])
	}
	if top[0].Nickname != "惯犯" {
		t.Fatalf("Top 榜应带上昵称，实际 %q", top[0].Nickname)
	}
	if top[1].UserID != ctype.NewID(8) || top[1].HitCount != 1 {
		t.Fatalf("榜上第二名应是用户 8 的 1 处，实际 %+v", top[1])
	}

	// limit 生效（Top 固定取前几条）。
	head, err := repo.TopUsers(ctx, since, 1)
	if err != nil {
		t.Fatalf("读取 Top 失败: %v", err)
	}
	if len(head) != 1 || head[0].UserID != ctype.NewID(7) {
		t.Fatalf("limit=1 应只返回第一名，实际 %+v", head)
	}
}
