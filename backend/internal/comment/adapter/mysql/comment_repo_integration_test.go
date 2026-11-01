//go:build integration

package mysql_test

import (
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/comment/adapter/mysql"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// migrate 按业务依赖顺序建表（表结构来源见 internal/testkit/schema）。
//
// 顺序不能随意：MySQL 下 AutoMigrate 会建外键，被引用的表必须先存在。
func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Comment)
	return db
}

// seedCommentTarget 插入评论所需的文章与用户。
//
// comment_models 有两条外键：article_id -> article_models.id、
// user_id -> user_models.id。真库下不先插这两行，创建评论必然失败。
func seedCommentTarget(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.InsertUser(t, db, schema.UserSpec{ID: 100, Username: "comment_author", Nickname: "作者"})
	schema.InsertUser(t, db, schema.UserSpec{ID: 200, Username: "comment_reader", Nickname: "读者"})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             1001,
		Title:          "评论契约文章",
		AuthorID:       100,
		CommentsToggle: true,
		PublishStatus:  schema.ArticleStatusPublished,
		Visibility:     schema.VisibilityVisible,
	})
}

// TestRootCommentDoesNotViolateParentForeignKey 是根评论的真 MySQL 契约。
//
// 为什么必须真 MySQL：根评论的 reply_id 没有父评论可指向，而表里
// fk_comment_models_parent_model (reply_id -> comment_models.id) 存在、
// 且库里**没有 id=0 的行**。若零值被落库写成 0，插入必然违反该外键，
// 线上表现为 violates foreign key constraint —— 任何根评论都会失败。
// SQLite 不会建出这条外键，验证不了这件事。
func TestRootCommentDoesNotViolateParentForeignKey(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	root := &domain.Comment{
		Content:   "顶层评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}

	if err := repo.Create(nil, root); err != nil {
		t.Fatalf("创建根评论失败（这正是外键缺陷）: %v", err)
	}

	// 根评论的父字段必须是 NULL，而不是 0。
	var parentIsNull bool
	if err := db.Raw("SELECT reply_id IS NULL FROM comment_models WHERE id = ?",
		root.ID.Uint64()).Scan(&parentIsNull).Error; err != nil {
		t.Fatalf("回查 reply_id 失败: %v", err)
	}
	if !parentIsNull {
		t.Fatal("根评论的 reply_id 应为 NULL；写成 0 会违反父评论外键")
	}
}

// TestReplyCommentKeepsParentLink 验证回复评论仍记录真实父链，
// 修根评论不能顺带把回复功能改坏。
func TestReplyCommentKeepsParentLink(t *testing.T) {
	db := migrate(t)
	seedCommentTarget(t, db)
	repo := mysql.New(db)

	root := &domain.Comment{
		Content:   "顶层评论",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, root); err != nil {
		t.Fatalf("预置根评论失败: %v", err)
	}

	reply := &domain.Comment{
		Content:   "回复",
		UserID:    ctype.NewID(200),
		ArticleID: ctype.NewID(1001),
		ReplyID:   root.ID,
		RootID:    root.ID,
		Status:    domain.StatusPublished,
	}
	if err := repo.Create(nil, reply); err != nil {
		t.Fatalf("创建回复失败: %v", err)
	}

	var got uint64
	if err := db.Raw("SELECT reply_id FROM comment_models WHERE id = ?",
		reply.ID.Uint64()).Scan(&got).Error; err != nil {
		t.Fatalf("回查 reply_id 失败: %v", err)
	}
	if got != root.ID.Uint64() {
		t.Fatalf("回复的 reply_id 应指向父评论 %d，实际 %d", root.ID.Uint64(), got)
	}
}
