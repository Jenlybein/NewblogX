package models_test

import (
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/models"
	"myblogx/test/testutil"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestListScanAndValue(t *testing.T) {
	var l ctype.List
	if err := l.Scan([]byte("a,b,c")); err != nil {
		t.Fatalf("Scan 失败: %v", err)
	}
	if len(l) != 3 || l[0] != "a" {
		t.Fatalf("Scan 结果异常: %+v", l)
	}

	if err := l.Scan(nil); err != nil {
		t.Fatalf("Scan nil 失败: %v", err)
	}
	if len(l) != 0 {
		t.Fatalf("Scan nil 后应为空: %+v", l)
	}

	if err := l.Scan(123); err == nil {
		t.Fatal("非法类型应报错")
	}

	v, err := (ctype.List{"x", "y"}).Value()
	if err != nil {
		t.Fatalf("Value 失败: %v", err)
	}
	if v.(string) != "x,y" {
		t.Fatalf("Value 结果异常: %v", v)
	}
}

func TestEnumStrings(t *testing.T) {
	if enum.LogInfoLevel.String() == "" || enum.LogWarnLevel.String() == "" || enum.LogErrorLevel.String() == "" {
		t.Fatal("日志级别 string 为空")
	}
	if enum.RoleAdmin.String() == "" || enum.RoleUser.String() == "" || enum.RoleGuest.String() == "" {
		t.Fatal("角色 string 为空")
	}
}

// TestModelMethods 已迁到 internal/platform/es/articlesync/rows_test.go。
//
// Mapping / Pipeline / Index 是"ES 文档长什么样"的知识，随投影搬走了；
// 留在这里会变成"测一个已经不存在的职责"。
func TestInitUserDefaults(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{})

	user := models.UserModel{
		Username: "tester_01",
		Password: "pwd",
	}
	testutil.CreateUser(t, db, &user)

	var confModel models.UserConfModel
	if err := db.First(&confModel, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("InitUserDefaults 未创建 user_conf: %v", err)
	}
}

func TestGlobalNotifBeforeDeleteHook(t *testing.T) {
	db := testutil.SetupSQLite(
		t,
		&models.GlobalNotifModel{},
		&models.UserGlobalNotifModel{},
	)

	notif := models.GlobalNotifModel{
		Title:      "global-notif",
		Content:    "content",
		ExpireTime: time.Now().Add(24 * time.Hour),
	}
	if err := db.Create(&notif).Error; err != nil {
		t.Fatalf("创建全局通知失败: %v", err)
	}

	now := time.Now()
	relations := []models.UserGlobalNotifModel{
		{MsgID: notif.ID, UserID: 1, IsRead: true, ReadAt: &now},
		{
			Model: models.Model{
				DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
			},
			MsgID:  notif.ID,
			UserID: 2,
		},
	}
	if err := db.Create(&relations).Error; err != nil {
		t.Fatalf("创建通知关系失败: %v", err)
	}

	if err := db.Unscoped().Delete(&notif).Error; err != nil {
		t.Fatalf("删除全局通知失败: %v", err)
	}

	var cnt int64
	if err := db.Model(&models.UserGlobalNotifModel{}).Where("msg_id = ?", notif.ID).Count(&cnt).Error; err != nil {
		t.Fatalf("查询通知关系失败: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("通知关系未清理, cnt=%d", cnt)
	}
}
