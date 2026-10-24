package models_test

import (
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"testing"
	"time"
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

func TestModelMethods(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
	})

	article := models.ArticleModel{}
	if article.Index() != "article_index" {
		t.Fatalf("Index 错误: %s", article.Index())
	}
	if article.Mapping() == "" {
		t.Fatal("Mapping 不应为空")
	}
	if article.Pipeline() == "" {
		t.Fatal("Pipeline 不应为空")
	}
	if article.PipelineName() == "" {
		t.Fatal("PipelineName 不应为空")
	}

	img := models.ImageModel{URL: "https://cdn.example.com/a.png"}
	if img.WebPath() != "https://cdn.example.com/a.png" {
		t.Fatalf("WebPath 错误: %s", img.WebPath())
	}

	u := models.UserModel{Model: models.Model{CreatedAt: time.Now().AddDate(-2, 0, 0)}}
	if u.CodeAge() < 1 {
		t.Fatalf("CodeAge 计算异常: %d", u.CodeAge())
	}
}

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
