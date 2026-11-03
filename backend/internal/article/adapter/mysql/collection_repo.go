package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// CollectionRepo 是 collection.Repository 的 MySQL 实现。
//
// 与 HotTopRepo（热门置顶）刻意不共用任何方法：合集有标题/描述/封面/展示开关，
// 且收录条数没有上限。共用会让两个功能互相牵制。
type CollectionRepo struct {
	db *gorm.DB
}

// NewCollectionRepo 构造仓储。
func NewCollectionRepo(db *gorm.DB) *CollectionRepo { return &CollectionRepo{db: db} }

// Create 插入合集。
//
// 用 map 插入而不是结构体：is_display 带 default:1，GORM 在 Create 时会**跳过
// 零值字段**让数据库默认值生效——于是"建一个不展示的合集"会被默默写成展示。
// 这正是"先准备、后公布"的运营节奏会踩的坑（Select("*") 也绕不过去）。
func (r *CollectionRepo) Create(ctx context.Context, c *collection.Collection) error {
	return r.db.WithContext(ctx).Model(&collectionRow{}).Create(map[string]any{
		"id":          ctype.ID(c.ID()),
		"created_at":  c.CreatedAt(),
		"updated_at":  c.UpdatedAt(),
		"title":       c.Title(),
		"description": c.Description(),
		"cover":       c.Cover(),
		"is_display":  c.Displayed(),
		"sort_order":  c.SortOrder(),
		"creator_id":  ctype.ID(c.CreatorID()),
	}).Error
}

// Update 保存已有合集的变更。
//
// 用 map 而不是结构体做 Updates：GORM 对结构体会跳过零值字段
// （is_display=false、sort_order=0 就永远改不动），而"关闭展示"正是把 true 改成 false。
//
// 软删也走这里：域里的 SoftDelete 只改聚合状态，落库必须显式写 deleted_at
// （GORM 的默认作用域会把 WHERE 限在 deleted_at IS NULL，所以这条 UPDATE 能落上）。
func (r *CollectionRepo) Update(ctx context.Context, c *collection.Collection) error {
	updates := map[string]any{
		"title":       c.Title(),
		"description": c.Description(),
		"cover":       c.Cover(),
		"is_display":  c.Displayed(),
		"sort_order":  c.SortOrder(),
		"updated_at":  c.UpdatedAt(),
	}
	if c.Deleted() {
		// 没有单独的删除时间字段：软删与最后一次修改是同一刻。
		updates["deleted_at"] = c.UpdatedAt()
	}
	res := r.db.WithContext(ctx).Model(&collectionRow{}).
		Where("id = ?", ctype.ID(c.ID())).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return collection.ErrNotFound
	}
	return nil
}

// Find 按 ID 读取合集（含未展示的）。
//
// 软删行由 GORM 自动排除；读不到一律返回 ErrNotFound（不区分"不存在"与
// "已删除"，两者对调用方是同一件事）。
func (r *CollectionRepo) Find(ctx context.Context, id collection.ID) (*collection.Collection, error) {
	var row collectionRow
	err := r.db.WithContext(ctx).Take(&row, "id = ?", ctype.ID(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toCollection(row), nil
}

// NextSortOrder 返回下一个可用序号（当前最大 + 1，空表为 1）。
func (r *CollectionRepo) NextSortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	err := r.db.WithContext(ctx).Model(&collectionRow{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// ListCards 列出展示中的合集卡片，按序号升序。
//
// 过滤"展示开关"在 SQL 里；篇数与封面回退来自目录树（outlineCardStats），
// 口径与读者详情一致：只数仍公开的文章、封面取 DFS 第一个公开文章节点。
//
// 分页为什么在 Go 里做：合集是**运营编排**的产物，数量以十计；而"开关关闭时
// 只保留仍有公开文章的合集"要先算出每张卡片的 article_count 才能判定，
// 在 SQL 里用别名过滤要包一层子查询，可读性换来的收益为零。
func (r *CollectionRepo) ListCards(
	ctx context.Context, onlyAdminAuthors bool, p domain.Page,
) ([]collection.Card, int64, error) {
	rows, err := r.cardRows(ctx, onlyAdminAuthors)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(rows))
	return pageCards(rows, p), total, nil
}

// cardRows 查出全部展示中的合集卡片（篇数与封面回退来自目录树）。
func (r *CollectionRepo) cardRows(
	ctx context.Context, onlyAdminAuthors bool,
) ([]collection.Card, error) {
	type row struct {
		ID          ctype.ID
		Title       string
		Description string
		Cover       string
		CreatedAt   time.Time
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("collection_models AS c").
		Select("c.id, c.title, c.description, c.cover, c.created_at").
		Where("c.deleted_at IS NULL AND c.is_display = 1").
		Order("c.sort_order ASC, c.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 卡片读面以目录树为准：旧扁平收录中间表已冻结，这里完全不出现。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerCollection, onlyAdminAuthors)
	if err != nil {
		return nil, err
	}

	cards := make([]collection.Card, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		// 开关关闭时，一篇公开文章都凑不出的合集也不出现。
		if onlyAdminAuthors && s.ArticleCount == 0 {
			continue
		}
		cover := m.Cover
		if cover == "" {
			// 没上传封面时回退到 DFS 第一个公开文章节点的封面；它为空就是空。
			cover = s.Cover
		}
		cards = append(cards, collection.Card{
			ID:           collection.ID(m.ID),
			Title:        m.Title,
			Description:  m.Description,
			Cover:        cover,
			ArticleCount: s.ArticleCount,
			CreatedAt:    m.CreatedAt,
		})
	}
	return cards, nil
}

// ListAdminCards 列出全部合集（含未展示）。
//
// 篇数与回退封面与读者卡片同一口径：只数目录树里仍公开的文章节点、封面取
// DFS 第一个公开文章节点；失效节点不进数。
//
// 回退封面单独给出，Cover 仍保持库里的原值：编辑表单据此区分"我传过"与
// "这是回退来的"。
func (r *CollectionRepo) ListAdminCards(ctx context.Context) ([]collection.AdminCard, error) {
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
		Table("collection_models AS c").
		Select("c.id, c.title, c.description, c.cover, c.is_display, c.sort_order").
		Where("c.deleted_at IS NULL").
		Order("c.sort_order ASC, c.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 后台卡片不看「显示非管理员发布文章」这个读者开关，仍按"公开文章"口径。
	stats, err := outlineCardStats(ctx, r.db, outline.OwnerCollection, false)
	if err != nil {
		return nil, err
	}

	out := make([]collection.AdminCard, 0, len(rows))
	for _, m := range rows {
		s := stats[m.ID]
		out = append(out, collection.AdminCard{
			ID:            collection.ID(m.ID),
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

// ListIDs 按序号返回全部未软删合集的 ID（合集之间换序的集合校验用）。
func (r *CollectionRepo) ListIDs(ctx context.Context) ([]collection.ID, error) {
	var raw []ctype.ID
	err := r.db.WithContext(ctx).Model(&collectionRow{}).
		Order("sort_order ASC, id ASC").
		Pluck("id", &raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]collection.ID, 0, len(raw))
	for _, v := range raw {
		out = append(out, collection.ID(v))
	}
	return out, nil
}

// UpdateSortOrders 按给定顺序把合集自身的序号改成 1..n。
//
// 集合一致性由 app 层的 collection.CheckSortOrder 保证，这里只负责落库；
// 逐条 UPDATE 足够（合集以十计），不折腾 CASE WHEN。
func (r *CollectionRepo) UpdateSortOrders(ctx context.Context, ids []collection.ID) error {
	db := r.db.WithContext(ctx)
	for i, id := range ids {
		if err := db.Model(&collectionRow{}).
			Where("id = ?", ctype.ID(id)).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListItemIDs 按序号返回当前收录的文章 ID（不含软删行）。
func (r *CollectionRepo) ListItemIDs(ctx context.Context, id collection.ID) ([]collection.ArticleID, error) {
	var raw []ctype.ID
	err := r.db.WithContext(ctx).Model(&collectionArticleRow{}).
		Where("collection_id = ?", ctype.ID(id)).
		Order("sort_order ASC, id ASC").
		Pluck("article_id", &raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]collection.ArticleID, 0, len(raw))
	for _, v := range raw {
		out = append(out, collection.ArticleID(v))
	}
	return out, nil
}

// AppendItem 收录一篇文章，返回是否真的新增。
//
// 三种情况（与 HotTopRepo.Save 同构）：
//   - 已被活跃收录：幂等返回 false，**不改序号**（重复收录不该把它挪到末尾）；
//   - 曾经移除（行被软删）：恢复那一行并按新序号排到末尾——唯一索引
//     uk_collection_article 不含 deleted_at，重新 INSERT 会撞 1062；
//   - 从未收录：插入新行，序号 = 当前最大 + 1。
func (r *CollectionRepo) AppendItem(
	ctx context.Context, id collection.ID, articleID collection.ArticleID,
) (bool, error) {
	db := r.db.WithContext(ctx)

	var row collectionArticleRow
	err := db.Unscoped().
		Where("collection_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
		Take(&row).Error
	switch {
	case err == nil && !row.DeletedAt.Valid:
		return false, nil
	case err == nil:
		next, err := r.nextItemOrder(db, id)
		if err != nil {
			return false, err
		}
		err = db.Unscoped().Model(&collectionArticleRow{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{"deleted_at": nil, "sort_order": next}).Error
		return err == nil, err
	case errors.Is(err, gorm.ErrRecordNotFound):
		next, err := r.nextItemOrder(db, id)
		if err != nil {
			return false, err
		}
		err = db.Create(&collectionArticleRow{
			CollectionID: ctype.ID(id),
			ArticleID:    ctype.ID(articleID),
			SortOrder:    next,
		}).Error
		// 唯一索引 (collection_id, article_id) 不含 deleted_at：两个管理员同时收录
		// 同一篇时，后到的 INSERT 会撞 1062。把唯一键冲突翻译成"这篇已经在合集里"，
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
func (r *CollectionRepo) RemoveItem(ctx context.Context, id collection.ID, articleID collection.ArticleID) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("collection_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
		Delete(&collectionArticleRow{}).Error; err != nil {
		return err
	}
	return r.compactItems(db, id)
}

// ListIncludedArticles 列出仍公开的收录文章，按顺序升序。
//
// 失效行在这里被 JOIN 过滤掉（与热门置顶 ListCards 同一写法）：
//   - 文章软删 → 过滤；
//   - 不再是"已发布 + visible" → 过滤。
//
// onlyAdminAuthors 为真时（「显示非管理员发布文章」开关关闭）再按作者角色过滤：
// 合集是读者端公开读面，与首页列表、详情、收藏内容、浏览历史同一口径。
func (r *CollectionRepo) ListIncludedArticles(
	ctx context.Context, id collection.ID, onlyAdminAuthors bool,
) ([]collection.IncludedArticle, error) {
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
		Table("collection_article_models AS i").
		Select("a.id AS article_id, i.sort_order, a.title, a.abstract, a.cover, a.view_count, "+
			"COALESCE(u.nickname, '') AS author_nickname, COALESCE(cat.title, '') AS category_title").
		Joins("JOIN article_models AS a ON a.id = i.article_id AND a.deleted_at IS NULL").
		// 软删用户不参与 join：否则 u.role 仍会读到已注销管理员的行。
		Joins("LEFT JOIN user_models AS u ON u.id = a.author_id AND u.deleted_at IS NULL").
		Joins("LEFT JOIN category_models AS cat ON cat.id = a.category_id").
		Where("i.collection_id = ? AND i.deleted_at IS NULL", ctype.ID(id)).
		Where("a.publish_status = ? AND a.visibility_status = ?", statusPublished, visibilityVisible)
	if onlyAdminAuthors {
		q = q.Where("u.role = ?", int8(enum.RoleAdmin))
	}
	if err := q.Order("i.sort_order ASC, i.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]collection.IncludedArticle, 0, len(rows))
	for _, m := range rows {
		out = append(out, collection.IncludedArticle{
			ArticleID:      collection.ArticleID(m.ArticleID),
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
func (r *CollectionRepo) ListAdminItems(ctx context.Context, id collection.ID) ([]collection.AdminItem, error) {
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
		Table("collection_article_models AS i").
		Select("i.article_id, i.sort_order, COALESCE(a.title, '') AS title, COALESCE(a.cover, '') AS cover, "+
			"a.id IS NOT NULL AS found, a.deleted_at IS NOT NULL AS deleted, "+
			"a.publish_status AS publish_state, a.visibility_status AS visibility").
		Joins("LEFT JOIN article_models AS a ON a.id = i.article_id").
		Where("i.collection_id = ? AND i.deleted_at IS NULL", ctype.ID(id)).
		Order("i.sort_order ASC, i.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]collection.AdminItem, 0, len(rows))
	for _, m := range rows {
		item := collection.AdminItem{
			ArticleID: collection.ArticleID(m.ArticleID),
			SortOrder: m.SortOrder,
			Title:     m.Title,
			Cover:     m.Cover,
		}
		if m.Found {
			state := &collection.ArticleState{Deleted: m.Deleted}
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
// 集合一致性由 app 层的 collection.CheckOrder 保证，这里只负责落库；
// 逐条 UPDATE 足够（一个合集的篇数十计），不必折腾 CASE WHEN。
func (r *CollectionRepo) UpdateItemOrder(
	ctx context.Context, id collection.ID, articleIDs []collection.ArticleID,
) error {
	db := r.db.WithContext(ctx)
	for i, articleID := range articleIDs {
		if err := db.Model(&collectionArticleRow{}).
			Where("collection_id = ? AND article_id = ?", ctype.ID(id), ctype.ID(articleID)).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// nextItemOrder 返回合集内下一个可用序号（当前最大 + 1，空合集为 1）。
//
// 并发说明：两个管理员同时"收录一篇"可能算出同一个序号。序号上没有唯一索引
// （零 DDL 加不了），读出用 `sort_order, id` 兜底，同号只退化成"按收录顺序"。
func (r *CollectionRepo) nextItemOrder(db *gorm.DB, id collection.ID) (int, error) {
	var maxOrder int
	err := db.Model(&collectionArticleRow{}).
		Where("collection_id = ?", ctype.ID(id)).
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
func (r *CollectionRepo) compactItems(db *gorm.DB, id collection.ID) error {
	var rows []collectionArticleRow
	err := db.Where("collection_id = ?", ctype.ID(id)).
		Order("sort_order ASC, id ASC").Find(&rows).Error
	if err != nil {
		return err
	}
	for i, row := range rows {
		if row.SortOrder == i+1 {
			continue
		}
		if err := db.Model(&collectionArticleRow{}).
			Where("id = ?", row.ID).
			Update("sort_order", i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

// toCollection 把行结构转成聚合。
func toCollection(m collectionRow) *collection.Collection {
	return collection.Restore(collection.RestoreParams{
		ID:          collection.ID(m.ID),
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

// pageCards 按 Offset/Limit 截取。
func pageCards(cards []collection.Card, p domain.Page) []collection.Card {
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
