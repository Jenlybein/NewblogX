//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/site/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// TestBannerRepoListFiltersByOptionalShow 是轮播展示状态三态的真 MySQL 契约。
// nil 必须完全省略 WHERE `show`，不能退化成零值 false 或 SQL NULL 比较。
func TestBannerRepoListFiltersByOptionalShow(t *testing.T) {
	t.Parallel()

	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.Site)
	base := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	rows := []schema.BannerRow{
		{
			BaseRow: schema.BaseRow{ID: ctype.NewID(101), CreatedAt: base, UpdatedAt: base},
			Show:    true,
			Cover:   "/shown.png",
			Href:    "/shown",
		},
		{
			BaseRow: schema.BaseRow{ID: ctype.NewID(102), CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)},
			Show:    false,
			Cover:   "/hidden.png",
			Href:    "/hidden",
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("预置轮播图失败: %v", err)
	}

	shown := true
	hidden := false
	cases := []struct {
		name       string
		show       *bool
		wantShown  int
		wantHidden int
	}{
		{name: "不传时不过滤", show: nil, wantShown: 1, wantHidden: 1},
		{name: "传真时只看展示中", show: &shown, wantShown: 1},
		{name: "传假时只看隐藏", show: &hidden, wantHidden: 1},
	}

	repo := sitemysql.NewBanner(db)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, hasMore, err := repo.List(context.Background(), domain.BannerQuery{Limit: 10, Show: tc.show})
			if err != nil {
				t.Fatalf("查轮播图失败: %v", err)
			}
			if hasMore {
				t.Fatal("两条种子数据不应有下一页")
			}
			var gotShown, gotHidden int
			for _, banner := range got {
				if banner.Show {
					gotShown++
				} else {
					gotHidden++
				}
			}
			if gotShown != tc.wantShown || gotHidden != tc.wantHidden {
				t.Fatalf("筛选结果错误: 展示中=%d 隐藏=%d", gotShown, gotHidden)
			}
		})
	}
}
