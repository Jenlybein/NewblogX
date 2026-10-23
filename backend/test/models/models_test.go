package models_test

import (
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/article_service"
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

func TestDeleteArticles(t *testing.T) {
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.ArticleModel{},
		&models.CommentModel{},
		&models.ArticleDiggModel{},
		&models.FavoriteModel{},
		&models.UserArticleFavorModel{},
		&models.UserTopArticleModel{},
		&models.UserArticleViewHistoryModel{},
	)

	author := models.UserModel{Username: "u1", Password: "p"}
	testutil.CreateUser(t, db, &author)
	article := models.ArticleModel{Title: "t", AuthorID: author.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	comment := models.CommentModel{ArticleID: article.ID, UserID: author.ID, Content: "c"}
	digg := models.ArticleDiggModel{ArticleID: article.ID, UserID: author.ID}
	favorite := models.FavoriteModel{UserID: author.ID, Title: "f"}
	if err := db.Create(&favorite).Error; err != nil {
		t.Fatalf("创建收藏夹失败: %v", err)
	}
	favorRelation := models.UserArticleFavorModel{
		ArticleID: article.ID,
		UserID:    author.ID,
		FavorID:   favorite.ID,
	}
	top := models.UserTopArticleModel{UserID: author.ID, ArticleID: article.ID}
	view := models.UserArticleViewHistoryModel{UserID: author.ID, ArticleID: article.ID}

	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	if err := db.Create(&digg).Error; err != nil {
		t.Fatalf("创建点赞失败: %v", err)
	}
	if err := db.Create(&favorRelation).Error; err != nil {
		t.Fatalf("创建收藏关系失败: %v", err)
	}
	if err := db.Create(&top).Error; err != nil {
		t.Fatalf("创建置顶失败: %v", err)
	}
	if err := db.Create(&view).Error; err != nil {
		t.Fatalf("创建浏览记录失败: %v", err)
	}

	if err := article_service.DeleteArticles(db, []models.ArticleModel{article}, false); err != nil {
		t.Fatalf("删除文章失败: %v", err)
	}

	var cnt int64
	_ = db.Model(&models.CommentModel{}).Where("article_id = ?", article.ID).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("评论未清理, cnt=%d", cnt)
	}
	_ = db.Model(&models.ArticleDiggModel{}).Where("article_id = ?", article.ID).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("点赞未清理, cnt=%d", cnt)
	}
	_ = db.Model(&models.UserArticleFavorModel{}).Where("article_id = ?", article.ID).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("收藏关系未清理, cnt=%d", cnt)
	}
	_ = db.Model(&models.UserTopArticleModel{}).Where("article_id = ?", article.ID).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("置顶未清理, cnt=%d", cnt)
	}
	_ = db.Model(&models.UserArticleViewHistoryModel{}).Where("article_id = ?", article.ID).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("浏览记录未清理, cnt=%d", cnt)
	}
}
