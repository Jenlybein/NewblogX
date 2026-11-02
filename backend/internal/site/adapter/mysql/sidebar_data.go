package mysql

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
)

// SidebarData 实现右栏各卡数据的只读端口。
//
// 三种卡的数据都在别的模块的表里（文章 / 评论 / 标签）。这里只做**投影读**，
// 不写、不 import 任何模块的 adapter：跨模块读的耦合收在站点模块自己的
// 只读查询里，表结构由 schema.sql 冻结，查询列的漂移会被列契约测试拦住。
type SidebarData struct {
	db *gorm.DB
}

var _ domain.SidebarDataSource = (*SidebarData)(nil)

// NewSidebarData 构造右栏数据源。
func NewSidebarData(db *gorm.DB) *SidebarData { return &SidebarData{db: db} }

// hotArticleRow / latestCommentRow / tagCloudRow 是查询结果的行形状。
//
// 刻意不复用各业务模块的行结构：这里只需要右栏展示的几列，
// 复用大结构会把无关列也读出来，且跨模块 import row 会被 R11 拦下。
type hotArticleRow struct {
	ID        ctype.ID
	Title     string
	DiggCount int
	ViewCount int
}

type latestCommentRow struct {
	ID           ctype.ID
	Content      string
	CreatedAt    time.Time
	ArticleID    ctype.ID
	ArticleTitle string
	UserNickname string
}

type tagCloudRow struct {
	ID           ctype.ID
	Title        string
	ArticleCount int
}

// HotArticles 按点赞数降序取公开文章。
func (s *SidebarData) HotArticles(ctx context.Context, limit int) ([]domain.SidebarArticle, error) {
	var rows []hotArticleRow
	err := s.db.WithContext(ctx).Raw(
		`SELECT id, title, digg_count, view_count
		   FROM article_models
		  WHERE deleted_at IS NULL
		    AND publish_status = ?
		    AND visibility_status = ?
		  ORDER BY digg_count DESC, id DESC
		  LIMIT ?`,
		int(enum.ArticleStatusPublished), string(enum.ArticleVisibilityVisible), clampLimit(limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SidebarArticle, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarArticle{
			ID:        row.ID,
			Title:     row.Title,
			DiggCount: row.DiggCount,
			ViewCount: row.ViewCount,
		})
	}
	return out, nil
}

// LatestComments 按时间降序取公开文章下的已发布评论。
//
// 评论的作者昵称是评论行里的冗余快照（user_nickname），不需要 join user 表。
func (s *SidebarData) LatestComments(ctx context.Context, limit int) ([]domain.SidebarComment, error) {
	var rows []latestCommentRow
	err := s.db.WithContext(ctx).Raw(
		`SELECT c.id, c.content, c.created_at, c.article_id, a.title AS article_title, c.user_nickname
		   FROM comment_models c
		   JOIN article_models a ON a.id = c.article_id
		  WHERE c.deleted_at IS NULL
		    AND a.deleted_at IS NULL
		    AND c.status = ?
		    AND a.publish_status = ?
		    AND a.visibility_status = ?
		  ORDER BY c.created_at DESC, c.id DESC
		  LIMIT ?`,
		int(enum.CommentStatusPublished),
		int(enum.ArticleStatusPublished), string(enum.ArticleVisibilityVisible), clampLimit(limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SidebarComment, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarComment{
			ID:           row.ID,
			Content:      row.Content,
			ArticleID:    row.ArticleID,
			ArticleTitle: row.ArticleTitle,
			UserNickname: row.UserNickname,
			CreatedAt:    row.CreatedAt,
		})
	}
	return out, nil
}

// TagCloud 按文章数降序取启用中的标签。
func (s *SidebarData) TagCloud(ctx context.Context, limit int) ([]domain.SidebarTag, error) {
	var rows []tagCloudRow
	err := s.db.WithContext(ctx).Raw(
		`SELECT id, title, article_count
		   FROM tag_models
		  WHERE deleted_at IS NULL
		    AND is_enabled = ?
		  ORDER BY article_count DESC, id ASC
		  LIMIT ?`,
		true, clampLimit(limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SidebarTag, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarTag{
			ID:           row.ID,
			Title:        row.Title,
			ArticleCount: row.ArticleCount,
		})
	}
	return out, nil
}

// clampLimit 收敛数据条数：0 或负数走默认值，超过上限截到上限。
func clampLimit(limit int) int {
	if limit <= 0 {
		return domain.SidebarCardDataLimit
	}
	if limit > domain.SidebarCardDataLimit {
		return domain.SidebarCardDataLimit
	}
	return limit
}
