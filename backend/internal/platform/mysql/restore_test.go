package mysql

import (
	"fmt"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 本文件是包内测试（package mysql），直接验证 RestoreOrCreateUnique 的真实 SQL 行为。
//
// 为什么用 sqlite 而不是 testcontainers：这条逻辑只依赖 GORM 的 schema 反射与
// UPDATE/Create 语义，sqlite 足够；真 MySQL 的差异（唯一索引报错码）由
// mysqlcontainer 的集成测试覆盖。为此本文件刻意不 import test/testutil，
// 避免平台包反向依赖测试工具包。

type softDeleteUniqueRow struct {
	BaseRow
	UserID    ctype.ID `gorm:"uniqueIndex:uk_restore_test,priority:1"`
	Title     string   `gorm:"size:64;uniqueIndex:uk_restore_test,priority:2"`
	Remark    string
	Sort      int
	IsEnabled bool
}

func openSQLite(t *testing.T, entities ...any) *gorm.DB {
	t.Helper()

	// softDeleteUniqueRow 内嵌 BaseRow，其 BeforeCreate 会生成雪花 ID，
	// 所以这里必须先让生成器就绪（原 testutil.SetupSQLite 也做了同样的事）。
	if !idgen.MustReady() {
		if err := idgen.Init(1); err != nil {
			t.Fatalf("初始化雪花 ID 生成器失败: %v", err)
		}
	}

	dsn := fmt.Sprintf("file:restore_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sqlite 连接失败: %v", err)
	}
	// 内存库必须收敛到单连接，多连接会各自看到不同的空库。
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if len(entities) > 0 {
		if err := db.AutoMigrate(entities...); err != nil {
			t.Fatalf("自动迁移失败: %v", err)
		}
	}
	return db
}

func TestRestoreOrCreateUnique(t *testing.T) {
	db := openSQLite(t, &softDeleteUniqueRow{})

	t.Run("创建新记录", func(t *testing.T) {
		ok, err := RestoreOrCreateUnique(db, &softDeleteUniqueRow{
			UserID: 1,
			Title:  "created",
			Remark: "v1",
		}, []string{"user_id", "title"})
		if err != nil {
			t.Fatalf("创建新记录失败: %v", err)
		}
		if !ok {
			t.Fatal("创建结果错误: 预期本次写入成功")
		}
	})

	t.Run("恢复软删记录", func(t *testing.T) {
		row := softDeleteUniqueRow{
			UserID:    2,
			Title:     "restored",
			Remark:    "old",
			Sort:      99,
			IsEnabled: true,
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("准备软删记录失败: %v", err)
		}
		if err := db.Delete(&row).Error; err != nil {
			t.Fatalf("软删记录失败: %v", err)
		}

		ok, err := RestoreOrCreateUnique(db, &softDeleteUniqueRow{
			UserID:    2,
			Title:     "restored",
			Remark:    "",
			Sort:      0,
			IsEnabled: false,
		}, []string{"user_id", "title"})
		if err != nil {
			t.Fatalf("恢复软删记录失败: %v", err)
		}
		if !ok {
			t.Fatal("恢复结果错误: 预期本次写入成功")
		}

		var restored softDeleteUniqueRow
		if err := db.Unscoped().Take(&restored, row.ID).Error; err != nil {
			t.Fatalf("回查恢复记录失败: %v", err)
		}
		if restored.DeletedAt.Valid {
			t.Fatal("恢复后 deleted_at 应为空")
		}
		if restored.Remark != "" {
			t.Fatalf("恢复后 remark 未更新为零值: got=%q", restored.Remark)
		}
		if restored.Sort != 0 {
			t.Fatalf("恢复后 sort 未更新为零值: got=%d", restored.Sort)
		}
		if restored.IsEnabled {
			t.Fatal("恢复后 is_enabled 未更新为 false")
		}
	})

	t.Run("活记录已存在时返回 false", func(t *testing.T) {
		row := softDeleteUniqueRow{
			UserID: 3,
			Title:  "existing",
			Remark: "keep",
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("准备活记录失败: %v", err)
		}

		ok, err := RestoreOrCreateUnique(db, &softDeleteUniqueRow{
			UserID: 3,
			Title:  "existing",
			Remark: "new",
		}, []string{"user_id", "title"})
		if err != nil {
			t.Fatalf("existing 分支失败: %v", err)
		}
		if ok {
			t.Fatal("活记录已存在时不应返回成功")
		}

		// 关键断言：不能把已有活记录的字段改掉。
		var kept softDeleteUniqueRow
		if err := db.Take(&kept, row.ID).Error; err != nil {
			t.Fatalf("回查活记录失败: %v", err)
		}
		if kept.Remark != "keep" {
			t.Fatalf("已存在的活记录被改写: got=%q", kept.Remark)
		}
	})

	t.Run("空匹配字段应报错", func(t *testing.T) {
		ok, err := RestoreOrCreateUnique(db, &softDeleteUniqueRow{
			UserID: 4,
			Title:  "invalid",
		}, nil)
		if err == nil {
			t.Fatal("空匹配字段应返回错误")
		}
		if ok {
			t.Fatal("空匹配字段不应返回成功")
		}
	})

	t.Run("空指针应报错", func(t *testing.T) {
		var row *softDeleteUniqueRow
		ok, err := RestoreOrCreateUnique(db, row, []string{"user_id", "title"})
		if err == nil {
			t.Fatal("空指针应返回错误")
		}
		if ok {
			t.Fatal("空指针不应返回成功")
		}
	})
}
