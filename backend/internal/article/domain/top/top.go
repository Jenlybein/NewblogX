// Package top 是"用户置顶自己的文章"子域。
//
// 与 favorite 的区别：置顶没有夹子层级，只是 (userID, articleID) 的唯一关系，
// 用于在个人主页把代表作排在前面。
package top

import (
	"context"

	"myblogx/internal/article/domain"
)

// Entry 是一条置顶关系。
//
// 唯一约束 (userID, articleID)，与 uk_user_top_article 一致。
// 没有数量上限：旧实现的置顶列表直接全量返回，未做限制。
type Entry struct {
	userID    domain.UserID
	articleID domain.ArticleID
}

// NewEntry 建立置顶关系。
func NewEntry(userID domain.UserID, articleID domain.ArticleID) *Entry {
	return &Entry{userID: userID, articleID: articleID}
}

// UserID 返回用户。
func (e *Entry) UserID() domain.UserID { return e.userID }

// ArticleID 返回文章。
func (e *Entry) ArticleID() domain.ArticleID { return e.articleID }

// Repository 是置顶关系的持久化契约。
type Repository interface {
	// Save 新增置顶；已存在视为成功（幂等）。
	Save(ctx context.Context, e *Entry) error
	// Delete 取消置顶；不存在视为成功（幂等）。
	Delete(ctx context.Context, userID domain.UserID, articleID domain.ArticleID) error
	// ListArticleIDs 列出某用户置顶的文章，按置顶时间倒序。
	ListArticleIDs(ctx context.Context, userID domain.UserID) ([]domain.ArticleID, error)
	// Exists 判断是否已置顶。
	Exists(ctx context.Context, userID domain.UserID, articleID domain.ArticleID) (bool, error)
	// DeleteByArticle 删除某文章的全部置顶（删文章级联用）。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error
}
