package common_test

import (
	"myblogx/common"
	"myblogx/models"
	"myblogx/test/testutil"
	"testing"
)

func TestPageInfoHelpers(t *testing.T) {
	p := common.PageInfo{Page: -1, Limit: 1000}
	if p.GetPage(100) != 1 {
		t.Fatalf("GetPage 默认值错误: %d", p.GetPage(100))
	}
	if p.GetLimit() != 10 {
		t.Fatalf("GetLimit 默认值错误: %d", p.GetLimit())
	}
	if p.GetOffset(100) != 0 {
		t.Fatalf("GetOffset 错误: %d", p.GetOffset(100))
	}

	p = common.PageInfo{Page: 2, Limit: 5}
	if p.GetOffset(100) != 5 {
		t.Fatalf("GetOffset 计算错误: %d", p.GetOffset(100))
	}
}

func TestListQueryBasic(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})

	records := []models.BannerModel{
		{Show: true, Cover: "alpha-cover", Href: "/a"},
		{Show: true, Cover: "beta-cover", Href: "/b"},
		{Show: false, Cover: "gamma", Href: "/c"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	list, count, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Page: 1, Limit: 2, Key: "cover"},
			Likes:    []string{"cover"},
		},
	)
	if err != nil {
		t.Fatalf("ListQuery 查询失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("count 错误: %d", count)
	}
	if len(list) != 2 {
		t.Fatalf("分页长度错误: %d", len(list))
	}
}

func TestListQueryInvalidOrder(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})

	_, _, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Order: "created_at desc"},
			OrderMap: map[string]bool{
				"id desc": true,
			},
		},
	)
	if err == nil {
		t.Fatal("非法排序字段应报错")
	}
}

func TestListQuerySelect(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})
	item := models.BannerModel{Show: true, Cover: "cover-x", Href: "/x"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("创建数据失败: %v", err)
	}

	list, count, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Select:   []string{"id", "cover"},
		},
	)
	if err != nil {
		t.Fatalf("ListQuery Select 查询失败: %v", err)
	}
	if count != 1 || len(list) != 1 {
		t.Fatalf("查询数量异常 count=%d len=%d", count, len(list))
	}
	if list[0].Cover != "cover-x" {
		t.Fatalf("Cover 字段未正确返回: %+v", list[0])
	}
	if list[0].Href != "" {
		t.Fatalf("未选中字段 Href 应为空: %+v", list[0])
	}
}

func TestListQueryLikesAndWhere(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})
	records := []models.BannerModel{
		{Show: true, Cover: "alpha-cover", Href: "/a"},
		{Show: false, Cover: "alpha-hidden", Href: "/b"},
		{Show: true, Cover: "beta-cover", Href: "/c"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	list, count, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Page: 1, Limit: 10, Key: "alpha"},
			Likes:    []string{"cover"},
			Where:    db.Where("show = ?", true),
		},
	)
	if err != nil {
		t.Fatalf("ListQuery Likes+Where 查询失败: %v", err)
	}
	if count != 1 || len(list) != 1 {
		t.Fatalf("结果异常 count=%d len=%d", count, len(list))
	}
	if list[0].Href != "/a" {
		t.Fatalf("返回数据错误: %+v", list[0])
	}
}

func TestListQueryCountError(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})
	if err := db.Create(&models.BannerModel{Show: true, Cover: "cover-x", Href: "/x"}).Error; err != nil {
		t.Fatalf("创建数据失败: %v", err)
	}

	_, _, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:    db,
			Where: db.Where("not a valid sql fragment"),
		},
	)
	if err == nil {
		t.Fatal("count 阶段 SQL 错误应返回 error")
	}
}

func TestListQueryUnscoped(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})

	item := models.BannerModel{Show: true, Cover: "deleted-cover", Href: "/deleted"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("创建数据失败: %v", err)
	}
	if err := db.Delete(&item).Error; err != nil {
		t.Fatalf("软删除数据失败: %v", err)
	}

	list, count, err := common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
		},
	)
	if err != nil {
		t.Fatalf("默认 ListQuery 查询失败: %v", err)
	}
	if count != 0 || len(list) != 0 {
		t.Fatalf("默认查询不应返回软删数据 count=%d len=%d", count, len(list))
	}

	list, count, err = common.ListQuery(
		models.BannerModel{},
		common.Options{
			DB:       db,
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			Unscoped: true,
		},
	)
	if err != nil {
		t.Fatalf("Unscoped ListQuery 查询失败: %v", err)
	}
	if count != 1 || len(list) != 1 {
		t.Fatalf("Unscoped 查询应返回软删数据 count=%d len=%d", count, len(list))
	}
	if !list[0].DeletedAt.Valid {
		t.Fatalf("返回数据应保留软删标记: %+v", list[0])
	}
}

func TestPageIDQueryUnscoped(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{})

	item := models.BannerModel{Show: true, Cover: "deleted-cover", Href: "/deleted"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("创建数据失败: %v", err)
	}
	if err := db.Delete(&item).Error; err != nil {
		t.Fatalf("软删除数据失败: %v", err)
	}

	ids, count, err := common.PageIDQuery(
		db.Model(&models.BannerModel{}),
		common.IDPageOptions{
			PageInfo:     common.PageInfo{Page: 1, Limit: 10},
			DefaultOrder: "id desc",
		},
	)
	if err != nil {
		t.Fatalf("默认 PageIDQuery 查询失败: %v", err)
	}
	if count != 0 || len(ids) != 0 {
		t.Fatalf("默认 PageIDQuery 不应返回软删数据 count=%d len=%d", count, len(ids))
	}

	ids, count, err = common.PageIDQuery(
		db.Model(&models.BannerModel{}),
		common.IDPageOptions{
			PageInfo:     common.PageInfo{Page: 1, Limit: 10},
			DefaultOrder: "id desc",
			Unscoped:     true,
		},
	)
	if err != nil {
		t.Fatalf("Unscoped PageIDQuery 查询失败: %v", err)
	}
	if count != 1 || len(ids) != 1 || ids[0] != item.ID {
		t.Fatalf("Unscoped PageIDQuery 应返回软删数据 ids=%v count=%d", ids, count)
	}
}
