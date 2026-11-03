package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// ProjectRepo 是 project.Repository 的 MySQL 实现（本票只含读者侧公开读）。
//
// 与 CollectionRepo 刻意不共用任何方法：项目与合集是两套表、两套数据，
// "结构同构"是为了心智复用，不是为了把两边的读模型拼成一个函数。
// 管理端写路径（#197）会在本文件里补上。
type ProjectRepo struct {
	db *gorm.DB
}

// NewProjectRepo 构造仓储。
func NewProjectRepo(db *gorm.DB) *ProjectRepo { return &ProjectRepo{db: db} }

// Find 按 ID 读取项目（含未展示的）。
//
// 软删行由 GORM 自动排除；读不到一律返回 ErrNotFound（不区分"不存在"与
// "已删除"，两者对调用方是同一件事）。
func (r *ProjectRepo) Find(ctx context.Context, id project.ID) (*project.Project, error) {
	var row projectRow
	err := r.db.WithContext(ctx).Take(&row, "id = ?", ctype.ID(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, project.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toProject(row), nil
}

// ListCards 列出展示中的项目卡片，按序号升序。
//
// 过滤"展示开关 + 仍公开的收录条数"都放在 SQL 里，与 domain 的公开口径
// 对齐（已发布 + visible + 未删除）。封面回退（没有封面就用首篇公开文章的
// 封面）也在 SQL 里做：列表页一次查完，不留给前端补。
//
// 分页为什么在 Go 里做：项目是**运营编排**的产物，数量以十计；而"开关关闭时
// 只保留仍有公开文章的项目"要先算出每张卡片的 article_count 才能判定。
func (r *ProjectRepo) ListCards(
	ctx context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]project.Card, int64, error) {
	rows, err := r.cardRows(ctx, onlyAdminAuthors)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(rows))
	return pageProjectCards(rows, p), total, nil
}

// cardRows 查出全部展示中的项目卡片（含 article_count 与封面回退）。
func (r *ProjectRepo) cardRows(
	ctx context.Context, onlyAdminAuthors bool,
) ([]project.Card, error) {
	type row struct {
		ID           ctype.ID
		Title        string
		Description  string
		Cover        string
		ArticleCount int
		CreatedAt    time.Time
	}
	// 公开文章的谓词：已发布 + visible + 未删除。
	publicArticle := "i.deleted_at IS NULL AND a.deleted_at IS NULL " +
		"AND a.publish_status = ? AND a.visibility_status = ?"

	adminFilter := ""
	if onlyAdminAuthors {
		// 作者不是管理员的文章不算"公开"：与首页列表、详情同一口径
		// （ADR-0017 决策 4）。管理员作者判定用用户表现读的 role。
		adminFilter = " AND u.role = ?"
	}

	coverFallback := "(SELECT a2.cover FROM project_article_models AS i2 " +
		"JOIN article_models AS a2 ON a2.id = i2.article_id " +
		"WHERE i2.project_id = p.id AND i2.deleted_at IS NULL AND a2.deleted_at IS NULL " +
		"AND a2.publish_status = ? AND a2.visibility_status = ? " +
		"ORDER BY i2.sort_order ASC, i2.id ASC LIMIT 1)"

	countSub := "(SELECT COUNT(*) FROM project_article_models AS i " +
		"JOIN article_models AS a ON a.id = i.article_id " +
		"JOIN user_models AS u ON u.id = a.author_id AND u.deleted_at IS NULL " +
		"WHERE i.project_id = p.id AND " + publicArticle + adminFilter + ")"

	// 参数顺序：coverFallback 的 ?, ? → countSub 的 ?, ? [→ u.role]
	//
	// 这些占位符都在 SELECT 子句里，所以必须作为 Select 的 args 传进去；
	// 传给 Where 的会错位到后面的 WHERE 参数上。
	selectArgs := []any{statusPublished, visibilityVisible, statusPublished, visibilityVisible}
	if onlyAdminAuthors {
		selectArgs = append(selectArgs, int8(enum.RoleAdmin))
	}

	countSelect := "p.id, p.title, p.description, p.created_at, " +
		"(CASE WHEN p.cover = '' THEN COALESCE(" + coverFallback + ", '') ELSE p.cover END) AS cover, " +
		countSub + " AS article_count"

	var out []row
	if err := r.db.WithContext(ctx).
		Table("project_models AS p").
		Select(countSelect, selectArgs...).
		Where("p.deleted_at IS NULL AND p.is_display = 1").
		Order("p.sort_order ASC, p.id ASC").
		Scan(&out).Error; err != nil {
		return nil, err
	}

	cards := make([]project.Card, 0, len(out))
	for _, m := range out {
		// 开关关闭时，一篇公开文章都凑不出的项目也不出现。
		if onlyAdminAuthors && m.ArticleCount == 0 {
			continue
		}
		cards = append(cards, project.Card{
			ID:           project.ID(m.ID),
			Title:        m.Title,
			Description:  m.Description,
			Cover:        m.Cover,
			ArticleCount: m.ArticleCount,
			CreatedAt:    m.CreatedAt,
		})
	}
	return cards, nil
}

// ListIncludedArticles 列出仍公开的收录文章，按顺序升序。
//
// 失效行在这里被 JOIN 过滤掉（与合集同一写法）：
//   - 文章软删 → 过滤；
//   - 不再是"已发布 + visible" → 过滤。
//
// onlyAdminAuthors 为真时（「显示非管理员发布文章」开关关闭）再按作者角色过滤：
// 项目是读者端公开读面，与首页列表、详情、收藏内容、浏览历史同一口径。
func (r *ProjectRepo) ListIncludedArticles(
	ctx context.Context, id project.ID, onlyAdminAuthors bool,
) ([]project.IncludedArticle, error) {
	type row struct {
		ArticleID      ctype.ID
		SortOrder      int
		Title          string
		Abstract       string
		Cover          string
		ViewCount      int
		AuthorNickname string
		CategoryTitle  string
	}
	var rows []row
	q := r.db.WithContext(ctx).
		Table("project_article_models AS i").
		Select("a.id AS article_id, i.sort_order, a.title, a.abstract, a.cover, a.view_count, "+
			"COALESCE(u.nickname, '') AS author_nickname, COALESCE(cat.title, '') AS category_title").
		Joins("JOIN article_models AS a ON a.id = i.article_id AND a.deleted_at IS NULL").
		// 软删用户不参与 join：否则 u.role 仍会读到已注销管理员的行。
		Joins("LEFT JOIN user_models AS u ON u.id = a.author_id AND u.deleted_at IS NULL").
		Joins("LEFT JOIN category_models AS cat ON cat.id = a.category_id").
		Where("i.project_id = ? AND i.deleted_at IS NULL", ctype.ID(id)).
		Where("a.publish_status = ? AND a.visibility_status = ?", statusPublished, visibilityVisible)
	if onlyAdminAuthors {
		q = q.Where("u.role = ?", int8(enum.RoleAdmin))
	}
	if err := q.Order("i.sort_order ASC, i.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]project.IncludedArticle, 0, len(rows))
	for _, m := range rows {
		out = append(out, project.IncludedArticle{
			ArticleID:      project.ArticleID(m.ArticleID),
			SortOrder:      m.SortOrder,
			Title:          m.Title,
			Abstract:       m.Abstract,
			Cover:          m.Cover,
			ViewCount:      m.ViewCount,
			AuthorNickname: m.AuthorNickname,
			CategoryTitle:  m.CategoryTitle,
		})
	}
	return out, nil
}

// toProject 把行结构转成聚合。
func toProject(m projectRow) *project.Project {
	return project.Restore(project.RestoreParams{
		ID:          project.ID(m.ID),
		Title:       m.Title,
		Description: m.Description,
		Cover:       m.Cover,
		CreatorID:   domain.UserID(m.CreatorID),
		Displayed:   m.IsDisplay,
		SortOrder:   m.SortOrder,
		Deleted:     m.DeletedAt.Valid,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	})
}

// pageProjectCards 按 Offset/Limit 截取。
func pageProjectCards(cards []project.Card, p domain.Page) []project.Card {
	start := p.Offset
	if start > len(cards) {
		start = len(cards)
	}
	end := start + p.Limit
	if end > len(cards) {
		end = len(cards)
	}
	return cards[start:end]
}
