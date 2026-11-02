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

// sidebarCollectionRow 是推荐合集查询的行形状（投影读，只读展示需要的几列）。
type sidebarCollectionRow struct {
	ID           ctype.ID
	Title        string
	Description  string
	Cover        string
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
// 三个过滤都在 SQL 里，与读者侧合集列表同一口径：
//   - is_display 为真（关掉展示的合集连数据都不该出现在右栏）；
//   - deleted_at 为空（软删的合集不出现）；
//   - 文章数只数"已发布 + 可见 + 未删除"的收录文章，与合集列表页的读数一致 ——
//     一张写着「20 篇」点进去只有 18 篇是坏体验。
//
// 封面回退（合集没封面就用首篇公开文章的封面）同样在 SQL 里，口径与列表页一致。
// 但这里刻意**不复用**文章模块的仓储：跨模块读只做站点模块自己的只读投影，
// 表结构由 schema.sql 冻结，列漂移会被列契约测试拦住。
func (s *SidebarData) Collections(ctx context.Context, limit int) ([]domain.SidebarCollection, error) {
	var rows []sidebarCollectionRow
	err := s.db.WithContext(ctx).Raw(
		`SELECT c.id, c.title, c.description,
		        (CASE WHEN c.cover = ''
		              THEN COALESCE((SELECT a2.cover
		                               FROM collection_article_models AS i2
		                               JOIN article_models AS a2 ON a2.id = i2.article_id
		                              WHERE i2.collection_id = c.id
		                                AND i2.deleted_at IS NULL
		                                AND a2.deleted_at IS NULL
		                                AND a2.publish_status = ?
		                                AND a2.visibility_status = ?
		                              ORDER BY i2.sort_order ASC, i2.id ASC
		                              LIMIT 1), '')
		              ELSE c.cover END) AS cover,
		        (SELECT COUNT(*)
		           FROM collection_article_models AS i
		           JOIN article_models AS a ON a.id = i.article_id
		          WHERE i.collection_id = c.id
		            AND i.deleted_at IS NULL
		            AND a.deleted_at IS NULL
		            AND a.publish_status = ?
		            AND a.visibility_status = ?) AS article_count
		   FROM collection_models AS c
		  WHERE c.deleted_at IS NULL
		    AND c.is_display = ?
		  ORDER BY c.sort_order ASC, c.id ASC
		  LIMIT ?`,
		int(enum.ArticleStatusPublished), string(enum.ArticleVisibilityVisible),
		int(enum.ArticleStatusPublished), string(enum.ArticleVisibilityVisible),
		true, clampLimit(limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SidebarCollection, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SidebarCollection{
			ID:           row.ID,
			Title:        row.Title,
			Description:  row.Description,
			Cover:        row.Cover,
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
