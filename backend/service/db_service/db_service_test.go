package db_service_test

import (
	"testing"

	"myblogx/models"
	"myblogx/models/ctype"
	dbservice "myblogx/service/db_service"
	"myblogx/test/testutil"
)

type softDeleteUniqueTestModel struct {
	models.Model
	UserID    ctype.ID `gorm:"uniqueIndex:uk_soft_delete_unique_test,priority:1"`
	Title     string   `gorm:"size:64;uniqueIndex:uk_soft_delete_unique_test,priority:2"`
	Remark    string
	Sort      int
	IsEnabled bool
}

func TestRestoreOrCreateUnique(t *testing.T) {
	db := testutil.SetupSQLite(t, &softDeleteUniqueTestModel{})

	t.Run("创建新记录", func(t *testing.T) {
		ok, err := dbservice.RestoreOrCreateUnique(db, &softDeleteUniqueTestModel{
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
		row := softDeleteUniqueTestModel{
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

		ok, err := dbservice.RestoreOrCreateUnique(db, &softDeleteUniqueTestModel{
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

		var restored softDeleteUniqueTestModel
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

	t.Run("活记录已存在时返回 existing", func(t *testing.T) {
		row := softDeleteUniqueTestModel{
			UserID: 3,
			Title:  "existing",
			Remark: "keep",
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("准备活记录失败: %v", err)
		}

		ok, err := dbservice.RestoreOrCreateUnique(db, &softDeleteUniqueTestModel{
			UserID: 3,
			Title:  "existing",
			Remark: "new",
		}, []string{"user_id", "title"})
		if err != nil {
			t.Fatalf("existing 分支失败: %v", err)
		}
		if ok {
			t.Fatal("existing 结果错误: 活记录已存在时不应返回成功")
		}
	})

	t.Run("空匹配字段应报错", func(t *testing.T) {
		ok, err := dbservice.RestoreOrCreateUnique(db, &softDeleteUniqueTestModel{
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
		var row *softDeleteUniqueTestModel
		ok, err := dbservice.RestoreOrCreateUnique(db, row, []string{"user_id", "title"})
		if err == nil {
			t.Fatal("空指针应返回错误")
		}
		if ok {
			t.Fatal("空指针不应返回成功")
		}
	})
}
