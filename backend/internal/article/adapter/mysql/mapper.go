// Package mysql 是 article 模块的持久化适配层。
//
// 两件事：
//  1. 实现 domain 的各仓储端口（SQL 都在这里，不再散落在 handler）；
//  2. 实现 app.TxManager：开一个事务，用同一个句柄构造 app.Repos。
//
// 关于 row：本项目**零 DDL**（ES 投影靠 CDC 按列名解析 binlog），
// 因此 row 直接复用 models 里已有的 GORM 模型，而不是另建一套带 tag 的结构体。
// "领域实体与 row 分离"在这里的体现是：domain.Article 是纯结构体，
// 所有 GORM tag 只存在于 models，两者之间只通过本文件的 mapper 转换。
package mysql

import (
	"myblogx/internal/article/domain"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// toArticleID 把库里的 ID 转成领域 ID。
func toArticleID(id ctype.ID) domain.ArticleID { return domain.ArticleID(id) }

// fromArticleID 把领域 ID 转成库里的 ID。
func fromArticleID(id domain.ArticleID) ctype.ID { return ctype.ID(id) }

// toUserID 把库里的用户 ID 转成领域 ID。
func toUserID(id ctype.ID) domain.UserID { return domain.UserID(id) }

// fromUserID 把领域用户 ID 转成库里的 ID。
func fromUserID(id domain.UserID) ctype.ID { return ctype.ID(id) }

// toStatus 把库里的状态转成领域状态。
func toStatus(s enum.ArticleStatus) domain.Status { return domain.Status(s) }

// fromStatus 把领域状态转成库里的状态。
func fromStatus(s domain.Status) enum.ArticleStatus { return enum.ArticleStatus(s) }

// toVisibility 把库里的可见性转成领域值。
func toVisibility(v enum.ArticleVisibilityStatus) domain.Visibility {
	return domain.Visibility(v)
}

// fromVisibility 把领域可见性转成库里的值。
func fromVisibility(v domain.Visibility) enum.ArticleVisibilityStatus {
	return enum.ArticleVisibilityStatus(domain.NormalizeVisibility(v))
}

// toCategoryID 转换可空分类 ID。
func toCategoryID(id *ctype.ID) *domain.CategoryID {
	if id == nil {
		return nil
	}
	v := domain.CategoryID(*id)
	return &v
}

// fromCategoryID 转换可空分类 ID。
func fromCategoryID(id *domain.CategoryID) *ctype.ID {
	if id == nil {
		return nil
	}
	v := ctype.ID(*id)
	return &v
}

// toReviewerID 转换可空审核人 ID。
func toReviewerID(id *ctype.ID) *domain.UserID {
	if id == nil {
		return nil
	}
	v := domain.UserID(*id)
	return &v
}

// fromReviewerID 转换可空审核人 ID。
func fromReviewerID(id *domain.UserID) *ctype.ID {
	if id == nil {
		return nil
	}
	v := ctype.ID(*id)
	return &v
}

// toDomainArticle 把 row 转成聚合根。
//
// tags 由调用方批量查询后传入：文章与标签是多对多关系，
// 放在这里 join 会让每次读取都多一条 SQL，列表页尤其明显。
func toDomainArticle(m models.ArticleModel, tags []domain.TagID) *domain.Article {
	return domain.Restore(domain.RestoreParams{
		ID:             toArticleID(m.ID),
		AuthorID:       toUserID(m.AuthorID),
		Title:          m.Title,
		Abstract:       m.Abstract,
		Content:        m.Content,
		CategoryID:     toCategoryID(m.CategoryID),
		Cover:          m.Cover,
		CommentsToggle: m.CommentsToggle,
		Status:         toStatus(m.PublishStatus),
		Visibility:     toVisibility(m.VisibilityStatus),
		SubmittedAt:    m.SubmittedAt,
		ReviewedAt:     m.ReviewedAt,
		ReviewedBy:     toReviewerID(m.ReviewedBy),
		Deleted:        m.DeletedAt.Valid,
		Counters: domain.Counters{
			View:    m.ViewCount,
			Digg:    m.DiggCount,
			Comment: m.CommentCount,
			Favor:   m.FavorCount,
		},
		Tags:      tags,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	})
}

// applyArticleTo 把聚合写回 row。
//
// 只写业务字段：ID / CreatedAt 由库维护，DeletedAt 由软删逻辑单独处理，
// 计数字段不在这里更新（走 Redis 增量 + 定时落库，与既有设计一致）。
func applyArticleTo(a *domain.Article, m *models.ArticleModel) {
	m.ID = fromArticleID(a.ID())
	m.AuthorID = fromUserID(a.AuthorID())
	m.Title = a.Title()
	m.Abstract = a.Abstract()
	m.Content = a.Content()
	m.CategoryID = fromCategoryID(a.CategoryID())
	m.Cover = a.Cover()
	m.CommentsToggle = a.CommentsToggle()
	m.PublishStatus = fromStatus(a.Status())
	m.VisibilityStatus = fromVisibility(a.Visibility())
	m.SubmittedAt = a.SubmittedAt()
	m.ReviewedAt = a.ReviewedAt()
	m.ReviewedBy = fromReviewerID(a.ReviewedBy())
	m.UpdatedAt = a.UpdatedAt()
}
