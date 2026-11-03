package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel/ctype"
)

// ProjectRepo 是 project.Repository 的 MySQL 实现（读者侧公开读 + 管理端读写）。
//
// 与 CollectionRepo 刻意不共用任何方法：项目与合集是两套表、两套数据，
// "结构同构"是为了心智复用，不是为了把两边的读模型拼成一个函数。
type ProjectRepo struct {
	db *gorm.DB
}

// NewProjectRepo 构造仓储。
func NewProjectRepo(db *gorm.DB) *ProjectRepo { return &ProjectRepo{db: db} }

// Create 插入项目。
//
// 用 map 插入而不是结构体：is_display 带 default:1，GORM 在 Create 时会**跳过
// 零值字段**让数据库默认值生效——于是"建一个不展示的项目"会被默默写成展示。
// 这正是"先准备、后公布"的运营节奏会踩的坑（Select("*") 也绕不过去）。
func (r *ProjectRepo) Create(ctx context.Context, p *project.Project) error {
	return r.db.WithContext(ctx).Model(&projectRow{}).Create(map[string]any{
		"id":          ctype.ID(p.ID()),
		"created_at":  p.CreatedAt(),
		"updated_at":  p.UpdatedAt(),
		"title":       p.Title(),
		"description": p.Description(),
		"cover":       p.Cover(),
		"is_display":  p.Displayed(),
		"sort_order":  p.SortOrder(),
		"creator_id":  ctype.ID(p.CreatorID()),
	}).Error
}

// Update 保存已有项目的变更。
//
// 用 map 而不是结构体做 Updates：GORM 对结构体会跳过零值字段
// （is_display=false、sort_order=0 就永远改不动），而"关闭展示"正是把 true
// 改成 false。软删也走这里：域里的 SoftDelete 只改聚合状态，落库必须显式写
// deleted_at（GORM 的默认作用域会把 WHERE 限在 deleted_at IS NULL，所以这条
// UPDATE 能落上）。
func (r *ProjectRepo) Update(ctx context.Context, p *project.Project) error {
	updates := map[string]any{
		"title":       p.Title(),
		"description": p.Description(),
		"cover":       p.Cover(),
		"is_display":  p.Displayed(),
		"sort_order":  p.SortOrder(),
		"updated_at":  p.UpdatedAt(),
	}
	if p.Deleted() {
		// 没有单独的删除时间字段：软删与最后一次修改是同一刻。
		updates["deleted_at"] = p.UpdatedAt()
	}
	res := r.db.WithContext(ctx).Model(&projectRow{}).
		Where("id = ?", ctype.ID(p.ID())).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return project.ErrNotFound
	}
	return nil
}

// NextSortOrder 返回下一个可用序号（当前最大 + 1，空表为 1）。
func (r *ProjectRepo) NextSortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	err := r.db.WithContext(ctx).Model(&projectRow{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

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

// ListAdminCards 列出全部项目（含未展示）。
//
// 篇数与回退封面与读者卡片同一口径：只数目录树里仍公开的文章节点、封面取
// DFS 第一个公开文章节点；失效节点不进数。
//
// 回退封面单独给出，Cover 仍保持库里的原值：编辑表单据此区分"我传过"与
// "这是回退来的"。
func (r *ProjectRepo) ListAdminCards(ctx context.Context) ([]project.AdminCard, error) {
	type row struct {
		ID          ctype.ID
		Title       string
		Description string
		Cover       string
		IsDisplay   bool
		SortOrder   int
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("project_models AS p").
		Select("p.id, p.title, p.description, p.cover, p.is_display, p.sort_order").
		Where("p.deleted_at IS NULL").
		Order("p.sort_order ASC, p.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 后台卡片不看「显示非管理员发布文章」这个读者开关，仍按"公开文章"口径。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerProject, false)
	if err != nil {
		return nil, err
	}

	out := make([]project.AdminCard, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		out = append(out, project.AdminCard{
			ID:            project.ID(m.ID),
			Title:         m.Title,
			Description:   m.Description,
			Cover:         m.Cover,
			Displayed:     m.IsDisplay,
			ArticleCount:  s.ArticleCount,
			SortOrder:     m.SortOrder,
			FallbackCover: s.Cover,
		})
	}
	return out, nil
}

// ListIDs 按序号返回全部未软删项目的 ID（项目之间换序的集合校验用）。
func (r *ProjectRepo) ListIDs(ctx context.Context) ([]project.ID, error) {
	var raw []ctype.ID
	err := r.db.WithContext(ctx).Model(&projectRow{}).
		Order("sort_order ASC, id ASC").
		Pluck("id", &raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]project.ID, 0, len(raw))
	for _, v := range raw {
		out = append(out, project.ID(v))
	}
	return out, nil
}

// UpdateSortOrders 按给定顺序把项目自身的序号改成 1..n。
//
// 集合一致性由 app 层的 project.CheckSortOrder 保证，这里只负责落库；
// 逐条 UPDATE 足够（项目以十计），不折腾 CASE WHEN。
func (r *ProjectRepo) UpdateSortOrders(ctx context.Context, ids []project.ID) error {
	db := r.db.WithContext(ctx)
	for i, id := range ids {
		if err := db.Model(&projectRow{}).
			Where("id = ?", ctype.ID(id)).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListCards 列出展示中的项目卡片，按序号升序。
//
// 过滤"展示开关"在 SQL 里；篇数与封面回退来自目录树（outlineCardStats），
// 与合集 / 读者详情同一口径：只数仍公开的文章、封面取 DFS 第一个公开文章节点。
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

// cardRows 查出全部展示中的项目卡片（篇数与封面回退来自目录树）。
func (r *ProjectRepo) cardRows(
	ctx context.Context, onlyAdminAuthors bool,
) ([]project.Card, error) {
	type row struct {
		ID          ctype.ID
		Title       string
		Description string
		Cover       string
		CreatedAt   time.Time
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("project_models AS p").
		Select("p.id, p.title, p.description, p.cover, p.created_at").
		Where("p.deleted_at IS NULL AND p.is_display = 1").
		Order("p.sort_order ASC, p.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 卡片读面以目录树为准：旧扁平收录中间表已冻结，这里完全不出现。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerProject, onlyAdminAuthors)
	if err != nil {
		return nil, err
	}

	cards := make([]project.Card, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		// 开关关闭时，一篇公开文章都凑不出的项目也不出现。
		if onlyAdminAuthors && s.ArticleCount == 0 {
			continue
		}
		cover := m.Cover
		if cover == "" {
			// 没上传封面时回退到 DFS 第一个公开文章节点的封面；它为空就是空。
			cover = s.Cover
		}
		cards = append(cards, project.Card{
			ID:           project.ID(m.ID),
			Title:        m.Title,
			Description:  m.Description,
			Cover:        cover,
			ArticleCount: s.ArticleCount,
			CreatedAt:    m.CreatedAt,
		})
	}
	return cards, nil
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
