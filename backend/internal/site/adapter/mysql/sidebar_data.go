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
// 热门文章 / 最新评论 / 标签云三种卡的数据都在别的模块的表里，这里做**只读
// 投影查询**（不写、不 import 任何模块的 adapter）。推荐合集卡不同：它的篇数
// 与封面回退依赖目录树的深度优先遍历，那套口径只应有 article 模块的一份
// 实现，所以通过端口注入读模型，而不是在站点里再写一套 SQL。
type SidebarData struct {
	db          *gorm.DB
	collections domain.SidebarCollectionReader
}

var _ domain.SidebarDataSource = (*SidebarData)(nil)

// NewSidebarData 构造右栏数据源；collections 为推荐合集卡的读模型端口。
func NewSidebarData(db *gorm.DB, collections domain.SidebarCollectionReader) *SidebarData {
	return &SidebarData{db: db, collections: collections}
}

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

// HotArticles 按点赞数降序取公开文章，并剔除主栏「热门置顶」已经露出的文章。
//
// 去重是这张卡的产品口径：同一篇文章在首页出现两次不是"推荐更多"，是版面事故。
// 排除放在 SQL 而不是取回后再过滤，否则被置顶的文章会占掉一条配额，
// 热门文章卡可能只剩 2–3 条 —— 看起来像数据不够，实际是去重吃掉了名额。
//
// 只排除**仍在架**的置顶行（deleted_at IS NULL）：运营取消置顶后，
// 那篇文章应该重新回到热门文章的候选里。失效的置顶行（文章转草稿等）
// 本来就会被下面的公开条件挡住，无需额外判断。
func (s *SidebarData) HotArticles(ctx context.Context, limit int) ([]domain.SidebarArticle, error) {
	var rows []hotArticleRow
	err := s.db.WithContext(ctx).Raw(
		`SELECT a.id, a.title, a.digg_count, a.view_count
		   FROM article_models a
		  WHERE a.deleted_at IS NULL
		    AND a.publish_status = ?
		    AND a.visibility_status = ?
		    AND NOT EXISTS (
		        SELECT 1
		          FROM article_hot_top_models t
		         WHERE t.article_id = a.id
		           AND t.deleted_at IS NULL
		    )
		  ORDER BY a.digg_count DESC, a.id DESC
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

// Collections 按合集自己的运营顺序取展示中的合集。
//
// 读模型由 article 模块的端口提供：篇数只数仍公开的文章、封面为空时取目录树
// 里第一个公开文章节点的封面，且与读者侧合集列表同一口径（含「显示非管理员
// 发布文章」开关）。站点模块不直连 article 的表，也不在本地再算一遍。
//
// 未装配读模型端口时返回空列表：右栏少一张卡，不影响其它卡片与页面。
func (s *SidebarData) Collections(ctx context.Context, limit int) ([]domain.SidebarCollection, error) {
	if s.collections == nil {
		return nil, nil
	}
	items, err := s.collections.RecommendedCollections(ctx, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	return items, nil
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
