//go:build integration

package mysql_test

// 本文件验证「查询缓存按目标结构体区分」这件事（#132）。
//
// # 结论先行：污染不成立，原根因被推翻
//
// 原假设是：窄结构体查询会把"少了几列"的 JSON 存进缓存，紧接着的整行查询
// 命中它、把窄 JSON 解进整行结构体 → 缺失字段静默归零。
//
// 实测推翻了它：`Take` 不带 Select 时生成的是 `SELECT *`，所以**窄结构体
// 查询缓存的 JSON 也含全部列**（GORM 按 SQL 取回全列，再按 Dest 的字段
// 选择性填充）。于是整行查询解出来的字段是齐的，不会归零。
//
// 这两个用例保留下来当护栏：它们钉住"整行必须读到真实库值"，以及
// "同形状查询仍能命中缓存"。将来若有人改成显式 Select 部分列（那时 SQL
// 才会真的只取部分列、缓存形状才会影响结果），第一条会立刻变红。

import (
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// narrowUserRow 模拟"局部行结构体"：只声明展示用的几列，
// **没有 token_version**（与 article 模块的 userRow 同形状）。
//
// DeletedAt 必须带上：软删条件 `deleted_at IS NULL` 是拼进 SQL 的，
// 没有这个字段的窄结构体会生成**不同**的 SQL，从而命中不到同一个缓存键，
// 污染就复现不出来（测试会假绿）。真实的 article userRow 内嵌 BaseRow，
// 本身就有 DeletedAt —— 所以两个查询的 SQL 才会完全一致。
type narrowUserRow struct {
	ID        ctype.ID `gorm:"primaryKey"`
	Username  string
	Nickname  string
	Role      int8
	DeletedAt gorm.DeletedAt
}

func (narrowUserRow) TableName() string { return "user_models" }

// TestWholeRowIsNotPoisonedByNarrowRow 钉住：先用窄行查、再用整行查，
// 整行必须读到真实库值。
func TestWholeRowIsNotPoisonedByNarrowRow(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore)
	cacher := mysqlcontainer.WithQueryCache(t, db)

	schema.InsertUser(t, db, schema.UserSpec{
		ID:       7201,
		Username: "cache_user",
		Nickname: "缓存用户",
	})
	// token_version 是唯一能分辨"真值 vs 被污染"的字段：真值 7，污染会变 0。
	if err := db.Exec("UPDATE user_models SET token_version = 7 WHERE id = ?", 7201).Error; err != nil {
		t.Fatalf("预置 token_version 失败: %v", err)
	}
	id := ctype.NewID(7201)

	// 第 1 步：窄行查询 → 缓存里存进它的结果。
	var narrow narrowUserRow
	if err := db.Take(&narrow, "id = ?", id).Error; err != nil {
		t.Fatalf("窄行查询失败: %v", err)
	}
	if narrow.Username != "cache_user" {
		t.Fatalf("窄行查询本身应正确，实际 %+v", narrow)
	}
	for _, k := range cacher.Keys() {
		t.Logf("缓存 key = %s", k)
		t.Logf("缓存 JSON = %s", string(cacher.Raw(k)))
	}

	// 第 2 步：整行查询 —— 若命中窄行缓存，token_version 会静默归零。
	var whole schema.UserRow
	if err := db.Take(&whole, "id = ?", id).Error; err != nil {
		t.Fatalf("整行查询失败: %v", err)
	}

	if whole.TokenVersion != 7 {
		t.Fatalf(
			"整行查询被窄行缓存污染：token_version 应为真实库值 7，实际 %d",
			whole.TokenVersion,
		)
	}
}

// TestAnyWriteWipesTheWholeCache 量化该插件的失效成本（#132 AC 第 5 条）。
//
// 插件在 Create/Update/Delete 回调里无条件调用 Invalidate，而本项目的
// redisCacher.Invalidate 是 `SCAN gorm-caches::*` 全量扫描 + DEL。
// 于是**任何一次写都会清空全部缓存**。
//
// 这决定了它的实际收益：本站每次文章浏览都要写浏览计数（/articles/view），
// 写极频繁 → 缓存刚存进去就被下一次浏览清空，命中率趋近于 0，
// 却每次写都付一次 O(N) 的 Redis SCAN。
//
// 本用例把这个事实钉住，作为"是否保留该插件"的判断依据。
func TestAnyWriteWipesTheWholeCache(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore)
	cacher := mysqlcontainer.WithQueryCache(t, db)

	schema.InsertUser(t, db, schema.UserSpec{ID: 7203, Username: "wipe_user", Nickname: "擦除"})
	id := ctype.NewID(7203)

	var first schema.UserRow
	if err := db.Take(&first, "id = ?", id).Error; err != nil {
		t.Fatalf("首次查询失败: %v", err)
	}
	if cacher.Len() == 0 {
		t.Fatal("查询后应写入缓存")
	}

	// 一次写操作（走 gorm 的 Update 回调，才会触发 Invalidate）。
	if err := db.Model(&schema.UserRow{}).
		Where("id = ?", id).
		Update("nickname", "随便改").Error; err != nil {
		t.Fatalf("写操作失败: %v", err)
	}

	if got := cacher.Len(); got != 0 {
		t.Fatalf("任何写都应清空全部缓存（这是全表 SCAN 式失效），实际残留 %d 条", got)
	}
}

// TestSameShapeQueryStillHitsCache 守住所保留的价值：
// 目标结构体相同时，缓存仍应命中。
func TestSameShapeQueryStillHitsCache(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore)
	cacher := mysqlcontainer.WithQueryCache(t, db)

	schema.InsertUser(t, db, schema.UserSpec{ID: 7202, Username: "hit_user", Nickname: "命中用户"})
	id := ctype.NewID(7202)

	var first schema.UserRow
	if err := db.Take(&first, "id = ?", id).Error; err != nil {
		t.Fatalf("首次查询失败: %v", err)
	}
	if cacher.Len() == 0 {
		t.Fatal("首次查询后应写入缓存；未写入说明插件没生效，本文件的护栏没有约束力")
	}

	// 绕过 gorm 直接改库（插件不会感知），再查一次：命中缓存会读到旧值。
	if err := db.Exec("UPDATE user_models SET nickname = '改过了' WHERE id = ?", 7202).Error; err != nil {
		t.Fatalf("改库失败: %v", err)
	}
	var second schema.UserRow
	if err := db.Take(&second, "id = ?", id).Error; err != nil {
		t.Fatalf("二次查询失败: %v", err)
	}
	if second.Nickname != "命中用户" {
		t.Fatalf("同形状查询应命中缓存（读到旧值「命中用户」），实际 %q", second.Nickname)
	}
}
