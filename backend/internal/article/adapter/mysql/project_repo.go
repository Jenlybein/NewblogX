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
// 与读者列表的差别：这里收录条数算**全部未软删的**收录行（不看文章是否公开），
// 后台要能看出"这个项目里有几篇已经失效了"。
//
// 回退封面单独查一份（与读者卡片同一个谓词）：后台列表要按读者看到的样子预览，
// 但 Cover 仍保持库里的原值，编辑表单据此区分"我传过"与"这是回退"。
func (r *ProjectRepo) ListAdminCards(ctx context.Context) ([]project.AdminCard, error) {
	type row struct {
		ID            ctype.ID
		Title         string
		Description   string
		Cover         string
		FallbackCover string
		IsDisplay     bool
		SortOrder     int
		ArticleCount  int
	}
	coverFallback := "(SELECT a2.cover FROM project_article_models AS i2 " +
		"JOIN article_models AS a2 ON a2.id = i2.article_id " +
		"WHERE i2.project_id = p.id AND i2.deleted_at IS NULL AND a2.deleted_at IS NULL " +
		"AND a2.publish_status = ? AND a2.visibility_status = ? " +
		"ORDER BY i2.sort_order ASC, i2.id ASC LIMIT 1)"

	var rows []row
	err := r.db.WithContext(ctx).
		Table("project_models AS p").
		Select("p.id, p.title, p.description, p.cover, p.is_display, p.sort_order, "+
			"COALESCE("+coverFallback+", '') AS fallback_cover, "+
			"(SELECT COUNT(*) FROM project_article_models AS i "+
			"WHERE i.project_id = p.id AND i.deleted_at IS NULL) AS article_count",
			statusPublished, visibilityVisible).
		Where("p.deleted_at IS NULL").
		Order("p.sort_order ASC, p.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]project.AdminCard, 0, len(rows))
	for _, m := range rows {
		out = append(out, project.AdminCard{
			ID:            project.ID(m.ID),
			Title:         m.Title,
			Description:   m.Description,
			Cover:         m.Cover,
			Displayed:     m.IsDisplay,
			ArticleCount:  m.ArticleCount,
			SortOrder:     m.SortOrder,
			FallbackCover: m.FallbackCover,
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

// ListItemIDs 按序号返回当前收录的文章 ID（不含软删行）。
func (r *ProjectRepo) ListItemIDs(ctx context.Context, id project.ID) ([]project.ArticleID, error) {
	var raw []ctype.ID
	err := r.db.WithContext(ctx).Model(&projectArticleRow{}).
		Where("project_id = ?", ctype.ID(id)).
		Order("sort_order ASC, id ASC").
		Pluck("article_id", &raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]project.ArticleID, 0, len(raw))
	for _, v := range raw {
		out = append(out, project.ArticleID(v))
	}
	return out, nil
}

// AppendItem 收录一篇文章，返回是否真的新增。
//
// 三种情况（与 CollectionRepo.AppendItem 同构）：
//   - 已被活跃收录：幂等返回 false，**不改序号**（重复收录不该把它挪到末尾）；
//   - 曾经移除（行被软删）：恢复那一行并按新序号排到末尾——唯一索引
//     uk_project_article 不含 deleted_at，重新 INSERT 会撞 1062；
//   - 从未收录：插入新行，序号 = 当前最大 + 1。
func (r *ProjectRepo) AppendItem(
	ctx context.Context, id project.ID, articleID project.ArticleID,
) (bool, error) {
	db := r.db.WithContext(ctx)

	var row projectArticleRow
	err := db.Unscoped().
		Where("project_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
		Take(&row).Error
	switch {
	case err == nil && !row.DeletedAt.Valid:
		return false, nil
	case err == nil:
		next, err := r.nextItemOrder(db, id)
		if err != nil {
			return false, err
		}
		err = db.Unscoped().Model(&projectArticleRow{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{"deleted_at": nil, "sort_order": next}).Error
		return err == nil, err
	case errors.Is(err, gorm.ErrRecordNotFound):
		next, err := r.nextItemOrder(db, id)
		if err != nil {
			return false, err
		}
		err = db.Create(&projectArticleRow{
			ProjectID: ctype.ID(id),
			ArticleID: ctype.ID(articleID),
			SortOrder: next,
		}).Error
		// 唯一索引 (project_id, article_id) 不含 deleted_at：两个管理员同时收录
		// 同一篇时，后到的 INSERT 会撞 1062。把唯一键冲突翻译成"这篇已经在项目里"，
		// 而不是把裸数据库错误透传成 500。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		return err == nil, err
	default:
		return false, err
	}
}

// RemoveItem 移除收录（软删）并压实剩余序号；不存在视为成功。
func (r *ProjectRepo) RemoveItem(ctx context.Context, id project.ID, articleID project.ArticleID) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("project_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
		Delete(&projectArticleRow{}).Error; err != nil {
		return err
	}
	return r.compactItems(db, id)
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

// ListAdminItems 列出全部收录行（含已失效），按顺序升序。
//
// LEFT JOIN：文章被软删时行还在（能看见标题与状态），
// 文章被物理删掉时 a.id 为 NULL —— 此时 Article 置 nil，由 domain 判为失效。
func (r *ProjectRepo) ListAdminItems(ctx context.Context, id project.ID) ([]project.AdminItem, error) {
	type row struct {
		ArticleID    ctype.ID
		SortOrder    int
		Title        string
		Cover        string
		Found        bool
		Deleted      bool
		PublishState *int8
		Visibility   *string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("project_article_models AS i").
		Select("i.article_id, i.sort_order, COALESCE(a.title, '') AS title, COALESCE(a.cover, '') AS cover, "+
			"a.id IS NOT NULL AS found, a.deleted_at IS NOT NULL AS deleted, "+
			"a.publish_status AS publish_state, a.visibility_status AS visibility").
		Joins("LEFT JOIN article_models AS a ON a.id = i.article_id").
		Where("i.project_id = ? AND i.deleted_at IS NULL", ctype.ID(id)).
		Order("i.sort_order ASC, i.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]project.AdminItem, 0, len(rows))
	for _, m := range rows {
		item := project.AdminItem{
			ArticleID: project.ArticleID(m.ArticleID),
			SortOrder: m.SortOrder,
			Title:     m.Title,
			Cover:     m.Cover,
		}
		if m.Found {
			state := &project.ArticleState{Deleted: m.Deleted}
			if m.PublishState != nil {
				state.Status = toStatus(*m.PublishState)
			}
			if m.Visibility != nil {
				state.Visibility = toVisibility(*m.Visibility)
			}
			item.Article = state
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateItemOrder 按给定顺序把收录序号改成 1..n。
//
// 集合一致性由 app 层的 project.CheckOrder 保证，这里只负责落库；
// 逐条 UPDATE 足够（一个项目的篇数十计），不必折腾 CASE WHEN。
func (r *ProjectRepo) UpdateItemOrder(
	ctx context.Context, id project.ID, articleIDs []project.ArticleID,
) error {
	db := r.db.WithContext(ctx)
	for i, articleID := range articleIDs {
		if err := db.Model(&projectArticleRow{}).
			Where("project_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// nextItemOrder 返回项目内下一个可用序号（当前最大 + 1，空项目为 1）。
//
// 并发说明：两个管理员同时"收录一篇"可能算出同一个序号。序号上没有唯一索引
// （零 DDL 加不了），读出用 `sort_order, id` 兜底，同号只退化成"按收录顺序"。
func (r *ProjectRepo) nextItemOrder(db *gorm.DB, id project.ID) (int, error) {
	var maxOrder int
	err := db.Model(&projectArticleRow{}).
		Where("project_id = ?", ctype.ID(id)).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// compactItems 把剩余收录行的序号压实成 1..n。
//
// 为什么需要：序号会在后台页展示、也会被"上移/下移"当作位置基准；
// 移除一篇后留个洞，人看到的就是"1,2,4"。
func (r *ProjectRepo) compactItems(db *gorm.DB, id project.ID) error {
	var rows []projectArticleRow
	err := db.Where("project_id = ?", ctype.ID(id)).
		Order("sort_order ASC, id ASC").Find(&rows).Error
	if err != nil {
		return err
	}
	for i, row := range rows {
		if row.SortOrder == i+1 {
			continue
		}
		if err := db.Model(&projectArticleRow{}).
			Where("id = ?", row.ID).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
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
