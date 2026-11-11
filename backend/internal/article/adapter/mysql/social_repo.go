package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// FavoriteRepo 是 favorite.Repository 的 MySQL 实现。
type FavoriteRepo struct {
	db *gorm.DB
}

// NewFavoriteRepo 构造仓储。
func NewFavoriteRepo(db *gorm.DB) *FavoriteRepo { return &FavoriteRepo{db: db} }

// FindFolder 读取收藏夹。
func (r *FavoriteRepo) FindFolder(ctx context.Context, id favorite.ID) (*favorite.Folder, error) {
	var m favoriteRow
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.ID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, favorite.ErrFolderNotFound
		}
		return nil, err
	}
	return toDomainFolder(m), nil
}

// DefaultFolder 取默认收藏夹，没有则创建。
//
// 与旧实现一致：用户第一次收藏不需要先去建夹子。
func (r *FavoriteRepo) DefaultFolder(ctx context.Context, userID domain.UserID) (*favorite.Folder, error) {
	var m favoriteRow
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", fromUserID(userID), true).
		Take(&m).Error
	if err == nil {
		return toDomainFolder(m), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	created := &favoriteRow{
		UserID:    fromUserID(userID),
		Title:     "默认收藏",
		IsDefault: true,
	}
	if err := r.db.WithContext(ctx).Create(created).Error; err != nil {
		return nil, err
	}
	return toDomainFolder(*created), nil
}

// ListFolders 列出某用户的收藏夹。
//
// 带回列表页需要的读模型字段：所属用户昵称/头像、创建与更新时间。
//
// 计数与包含关系都**现算**而不是读冗余列：`article_count` 过去没有任何写入
// （新建的夹子恒 0，用户会读成"没收藏进去"）。写入侧现在会在同一事务内刷新
// 该列，但读侧仍以关系行为准 —— 列只是给外部消费者看的快照，不能让列表
// 依赖一个"某条写路径忘了刷新"就长期撒谎的值（#344）。
func (r *FavoriteRepo) ListFolders(ctx context.Context, q favorite.ListFoldersQuery) ([]*favorite.Folder, error) {
	var rows []struct {
		ID            ctype.ID
		CreatedAt     time.Time
		UpdatedAt     time.Time
		UserID        ctype.ID
		Title         string
		Cover         string
		Abstract      string
		IsDefault     bool
		ArticleCount  int
		Nickname      string
		Avatar        string
		HasArticle    bool
	}
	// 公开读面（看别人的夹子或匿名）只数已发布文章：草稿/审核中的数量不该
	// 透给访客，与收藏内容列表同一口径。
	visibleFilter := "r.deleted_at IS NULL"
	selectParts := []string{
		"f.*",
		"u.nickname AS nickname",
		"u.avatar AS avatar",
	}
	// SELECT 里的占位符必须与 args 严格同序：可见性过滤在前（仅公开面有），
	// has_article 的 article_id 在后。写反了会把 ID 绑到状态位上，且不报错。
	selectArgs := make([]any, 0, 2)

	if q.PublishedOnly {
		visibleFilter += " AND EXISTS (SELECT 1 FROM article_models pa WHERE pa.id = r.article_id AND pa.deleted_at IS NULL AND pa.publish_status = ?)"
		selectArgs = append(selectArgs, statusPublished)
	}
	selectParts = append(selectParts,
		"(SELECT COUNT(*) FROM user_article_favor_models r WHERE r.favor_id = f.id AND "+visibleFilter+") AS article_count")

	if !q.ArticleID.IsZero() {
		// has_article 只看"这篇文章是否在这个夹子里"，不关心它是否已发布：
		// 收藏弹窗要据此显示勾还是加号（#344）。
		selectParts = append(selectParts,
			"EXISTS (SELECT 1 FROM user_article_favor_models ra WHERE ra.favor_id = f.id AND ra.article_id = ? AND ra.deleted_at IS NULL) AS has_article")
		selectArgs = append(selectArgs, ctype.ID(q.ArticleID))
	}

	// 默认夹固定第一，其余按更新时间倒序：新建或刚变动的夹子排在最前，
	// 与"新建后留在网格里"的交互一致。
	err := r.db.WithContext(ctx).
		Table("favorite_models AS f").
		Select(strings.Join(selectParts, ", "), selectArgs...).
		Joins("LEFT JOIN user_models u ON u.id = f.user_id").
		Where("f.user_id = ? AND f.deleted_at IS NULL", fromUserID(q.UserID)).
		Order("f.is_default desc, f.updated_at desc, f.id desc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*favorite.Folder, 0, len(rows))
	for _, m := range rows {
		out = append(out, favorite.RestoreFolder(favorite.RestoreFolderParams{
			ID:            favorite.ID(m.ID),
			UserID:        domain.UserID(m.UserID),
			Title:         m.Title,
			Cover:         m.Cover,
			Abstract:      m.Abstract,
			IsDefault:     m.IsDefault,
			ArticleCount:  m.ArticleCount,
			CreatedAt:     m.CreatedAt,
			UpdatedAt:     m.UpdatedAt,
			OwnerNickname: m.Nickname,
			OwnerAvatar:   m.Avatar,
			HasArticle:    m.HasArticle,
		}))
	}
	return out, nil
}

// argsForFolders 组装 ListFolders 的条件参数。
//
// SELECT 里的占位符顺序是"可见性过滤（可能带 publish_status）→ has_article 的
// article_id"，这里必须与 SELECT 严格同序，否则会把 ID 绑到状态位上。
func argsForFolders(q favorite.ListFoldersQuery, publishedStatus int8) []any {
	args := make([]any, 0, 2)
	if q.PublishedOnly {
		args = append(args, publishedStatus)
	}
	if !q.ArticleID.IsZero() {
		args = append(args, ctype.ID(q.ArticleID))
	}
	return args
}

// ExistsFolderTitle 判断名称重复。
func (r *FavoriteRepo) ExistsFolderTitle(ctx context.Context, userID domain.UserID, title string, excludeID favorite.ID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&favoriteRow{}).
		Where("user_id = ? AND title = ?", fromUserID(userID), title)
	if !excludeID.IsZero() {
		q = q.Where("id <> ?", ctype.ID(excludeID))
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// SaveFolder 新建或更新收藏夹。
func (r *FavoriteRepo) SaveFolder(ctx context.Context, f *favorite.Folder) error {
	id := ctype.ID(f.ID())
	updates := r.db.WithContext(ctx).Model(&favoriteRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"title":    f.Title(),
			"cover":    f.Cover(),
			"abstract": f.Abstract(),
		})
	if updates.Error != nil {
		return updates.Error
	}
	if updates.RowsAffected > 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&favoriteRow{
		BaseRow:   BaseRow{ID: id},
		UserID:    fromUserID(f.UserID()),
		Title:     f.Title(),
		Cover:     f.Cover(),
		Abstract:  f.Abstract(),
		IsDefault: f.IsDefault(),
	}).Error
}

// DeleteFolder 删除收藏夹与其下所有收藏关系，**不动文章本身**。
//
// 级联是被领域契约要求的（favorite.Repository 的注释、fake 仓储都这么做），
// 旧实现只在注释里写着"BeforeDelete 会清掉关系"，而那个钩子在重构时被删除，
// 于是关系行成了孤儿：文章仍显示"已收藏"，却不在任何夹子里（#344）。
func (r *FavoriteRepo) DeleteFolder(ctx context.Context, id favorite.ID) error {
	if err := r.db.WithContext(ctx).
		Where("favor_id = ?", ctype.ID(id)).
		Delete(&userArticleFavorRow{}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).
		Where("id = ?", ctype.ID(id)).
		Delete(&favoriteRow{}).Error
}

// RefreshFolderStats 在同一事务内重算该夹的文章数并推进更新时间。
//
// 用一条 UPDATE + 子查询而不是"读出来再写回去"：并发收藏两个不同文章时，
// 读-改-写会丢计数，整列重算不会。updated_at 一并推进，收藏夹列表才能按
// "最近变动"排序（#344）。
func (r *FavoriteRepo) RefreshFolderStats(ctx context.Context, folderID favorite.ID, updatedAt time.Time) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE favorite_models AS f
		SET article_count = (
				SELECT COUNT(*) FROM user_article_favor_models r
				WHERE r.favor_id = f.id AND r.deleted_at IS NULL
			),
			updated_at = ?
		WHERE f.id = ? AND f.deleted_at IS NULL`,
		updatedAt, ctype.ID(folderID)).Error
}

// FindItem 读取某夹子里的收藏。
func (r *FavoriteRepo) FindItem(ctx context.Context, articleID domain.ArticleID, userID domain.UserID, folderID favorite.ID) (*favorite.Item, error) {
	var m userArticleFavorRow
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ? AND favor_id = ?",
			fromArticleID(articleID), fromUserID(userID), ctype.ID(folderID)).
		Take(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, favorite.ErrItemNotFound
		}
		return nil, err
	}
	return toDomainItem(m), nil
}

// FindItemAnyFolder 读取任意收藏。
func (r *FavoriteRepo) FindItemAnyFolder(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (*favorite.Item, error) {
	var m userArticleFavorRow
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ?", fromArticleID(articleID), fromUserID(userID)).
		Take(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, favorite.ErrItemNotFound
		}
		return nil, err
	}
	return toDomainItem(m), nil
}

// SaveItem 新建收藏（若同键行曾被软删，就地复活它）。
//
// 查询旧行必须 **Unscoped**：唯一键 uk_user_article_favor (article_id, user_id, favor_id)
// 不含 deleted_at，被「移出收藏夹」软删过的行仍然占着这个键。带软删过滤去查就会走
// Create 撞唯一键，表现是用户「移出后再收藏同一篇」收到 1001 duplicated key not allowed。
// 与浏览历史 Upsert 是同一类问题、同一种解法：找到**软删过的**旧行就复活并刷新快照，
// 只有真没有才插入。
//
// 活行（未软删）重复保存仍然返回 `gorm.ErrDuplicatedKey`：仓库层守住唯一键语义，
// 「已收藏就报 ErrItemExists」由用例层（Favorite）负责。两者分开才能保证并发双击
// 不会既多插一行、又静默把计数 +1。
func (r *FavoriteRepo) SaveItem(ctx context.Context, i *favorite.Item) error {
	var old userArticleFavorRow
	err := r.db.WithContext(ctx).Unscoped().
		Where("article_id = ? AND user_id = ? AND favor_id = ?",
			fromArticleID(i.ArticleID()), fromUserID(i.UserID()), ctype.ID(i.FolderID())).
		Take(&old).Error
	if err == nil {
		if !old.DeletedAt.Valid {
			// 活行已存在：并发重复写，按唯一键冲突处理（不要复活成一次更新）。
			return gorm.ErrDuplicatedKey
		}
		return r.db.WithContext(ctx).Unscoped().Model(&userArticleFavorRow{}).
			Where("id = ?", old.ID).
			Updates(map[string]any{
				"article_title":           i.ArticleTitle(),
				"article_abstract":        i.ArticleAbstract(),
				"article_cover":           i.ArticleCover(),
				"article_status":          fromStatus(i.ArticleStatus()),
				"article_author_id":       fromUserID(i.AuthorID()),
				"article_author_nickname": i.AuthorNickname(),
				"article_author_avatar":   i.AuthorAvatar(),
				"deleted_at":              nil,
			}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.WithContext(ctx).Create(&userArticleFavorRow{
		ArticleID:             fromArticleID(i.ArticleID()),
		UserID:                fromUserID(i.UserID()),
		FavorID:               ctype.ID(i.FolderID()),
		ArticleTitle:          i.ArticleTitle(),
		ArticleAbstract:       i.ArticleAbstract(),
		ArticleCover:          i.ArticleCover(),
		ArticleStatus:         fromStatus(i.ArticleStatus()),
		ArticleAuthorID:       fromUserID(i.AuthorID()),
		ArticleAuthorNickname: i.AuthorNickname(),
		ArticleAuthorAvatar:   i.AuthorAvatar(),
	}).Error
}

// DeleteItem 删除收藏。
func (r *FavoriteRepo) DeleteItem(ctx context.Context, articleID domain.ArticleID, userID domain.UserID, folderID favorite.ID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ? AND favor_id = ?",
			fromArticleID(articleID), fromUserID(userID), ctype.ID(folderID)).
		Delete(&userArticleFavorRow{}).Error
}

// DeleteItemsByArticle 删除某文章全部收藏。
func (r *FavoriteRepo) DeleteItemsByArticle(ctx context.Context, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&userArticleFavorRow{}).Error
}

// ListItems 分页列出某夹子的收藏。
//
// 收藏快照 + 文章实时计数一次查出：列表页要展示 view/digg/comment/favor，
// 快照里没有这些会变的数字，只能 join 文章表取当前值。
//
// onlyAdminAuthors 为真时（「显示非管理员发布文章」开关关闭）再 join 作者行，
// 只保留 role = 管理员的条目。join 必须同时作用于 count 与当页查询：
// 只过滤当页会让 total 变成全量，前端分页直接错乱。
// 用快照里的 article_author_id 而不是文章表的 author_id：作者角色是**现读**的
// （用户改角色后立刻生效），文章归属则取收藏时的快照，与列表展示一致。
func (r *FavoriteRepo) ListItems(ctx context.Context, folderID favorite.ID, offset, limit int, onlyAdminAuthors, publishedOnly bool) ([]*favorite.Item, int64, error) {
	// publishedOnly 为公开读面（他人夹/匿名）准备：收藏时文章可能已发布，
	// 之后被撤回草稿或删除。用 JOIN（而非 EXISTS）同时作用于 count 与当页，
	// 否则 total 会与列表不一致，分页直接错乱。
	joinArticle := "LEFT JOIN article_models a ON a.id = f.article_id AND a.deleted_at IS NULL"
	if publishedOnly {
		joinArticle = "JOIN article_models a ON a.id = f.article_id AND a.deleted_at IS NULL AND a.publish_status = ?"
	}

	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("user_article_favor_models AS f").
			Where("f.favor_id = ? AND f.deleted_at IS NULL", ctype.ID(folderID))
		if onlyAdminAuthors {
			q = q.Joins("JOIN user_models AS author_u ON author_u.id = f.article_author_id AND author_u.role = ?",
				int8(enum.RoleAdmin))
		}
		if publishedOnly {
			q = q.Joins(joinArticle, statusPublished)
		} else {
			q = q.Joins(joinArticle)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 10
	}

	var rows []struct {
		CreatedAt        time.Time
		ArticleUpdatedAt time.Time
		ArticleID        ctype.ID
		UserID           ctype.ID
		ArticleTitle     string
		ArticleAbstract  string
		ArticleCover     string
		ArticleStatus    int8
		AuthorID         ctype.ID
		AuthorNickname   string
		AuthorAvatar     string
		ViewCount        int
		DiggCount        int
		CommentCount     int
		FavorCount       int
	}
	if err := base().
		Select("f.created_at, COALESCE(a.updated_at, f.created_at) AS article_updated_at, " +
			"f.article_id, f.user_id, f.article_title, f.article_abstract, f.article_cover, " +
			"f.article_status, f.article_author_id AS author_id, " +
			"f.article_author_nickname AS author_nickname, f.article_author_avatar AS author_avatar, " +
			"COALESCE(a.view_count,0) AS view_count, COALESCE(a.digg_count,0) AS digg_count, " +
			"COALESCE(a.comment_count,0) AS comment_count, COALESCE(a.favor_count,0) AS favor_count").
		Order("f.created_at desc, f.id desc").
		Offset(offset).Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]*favorite.Item, 0, len(rows))
	for _, m := range rows {
		out = append(out, favorite.RestoreItem(favorite.RestoreItemParams{
			ArticleID:        domain.ArticleID(m.ArticleID),
			UserID:           domain.UserID(m.UserID),
			FolderID:         folderID,
			ArticleTitle:     m.ArticleTitle,
			ArticleAbstract:  m.ArticleAbstract,
			ArticleCover:     m.ArticleCover,
			ArticleStatus:    domain.Status(m.ArticleStatus),
			AuthorID:         domain.UserID(m.AuthorID),
			AuthorNickname:   m.AuthorNickname,
			AuthorAvatar:     m.AuthorAvatar,
			FavoritedAt:      m.CreatedAt,
			ArticleUpdatedAt: m.ArticleUpdatedAt,
			ViewCount:        m.ViewCount,
			DiggCount:        m.DiggCount,
			CommentCount:     m.CommentCount,
			FavorCount:       m.FavorCount,
		}))
	}
	return out, total, nil
}

// CountByArticle 统计某文章被收藏次数。
func (r *FavoriteRepo) CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&userArticleFavorRow{}).
		Where("article_id = ?", fromArticleID(articleID)).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

func toDomainFolder(m favoriteRow) *favorite.Folder {
	return favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID:           favorite.ID(m.ID),
		UserID:       domain.UserID(m.UserID),
		Title:        m.Title,
		Cover:        m.Cover,
		Abstract:     m.Abstract,
		IsDefault:    m.IsDefault,
		ArticleCount: m.ArticleCount,
	})
}

func toDomainItem(m userArticleFavorRow) *favorite.Item {
	return favorite.RestoreItem(favorite.RestoreItemParams{
		ArticleID:       domain.ArticleID(m.ArticleID),
		UserID:          domain.UserID(m.UserID),
		FolderID:        favorite.ID(m.FavorID),
		ArticleTitle:    m.ArticleTitle,
		ArticleAbstract: m.ArticleAbstract,
		ArticleCover:    m.ArticleCover,
		ArticleStatus:   domain.Status(m.ArticleStatus),
		AuthorID:        domain.UserID(m.ArticleAuthorID),
		AuthorNickname:  m.ArticleAuthorNickname,
		AuthorAvatar:    m.ArticleAuthorAvatar,
	})
}

// TopRepo 是 top.Repository 的 MySQL 实现。
type TopRepo struct {
	db *gorm.DB
}

// NewTopRepo 构造仓储。
func NewTopRepo(db *gorm.DB) *TopRepo { return &TopRepo{db: db} }

// Save 新增置顶（幂等：已存在时忽略唯一键冲突）。
func (r *TopRepo) Save(ctx context.Context, e *top.Entry) error {
	exists, err := r.Exists(ctx, e.UserID(), e.ArticleID())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return r.db.WithContext(ctx).Create(&userTopArticleRow{
		UserID:    fromUserID(e.UserID()),
		ArticleID: fromArticleID(e.ArticleID()),
	}).Error
}

// Delete 取消置顶（幂等）。
func (r *TopRepo) Delete(ctx context.Context, userID domain.UserID, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND article_id = ?", fromUserID(userID), fromArticleID(articleID)).
		Delete(&userTopArticleRow{}).Error
}

// ListCards 列出置顶文章的展示卡片。
//
// 一次 join 拿全展示字段：置顶表 → 文章表（排除软删）→ 用户（昵称）→ 分类（标题）。
// 只给 ID 的话首页要逐篇再查详情，N+1。字段名与 ArticleTopCardResponse 一一对应。
func (r *TopRepo) ListCards(ctx context.Context, userID domain.UserID) ([]top.Card, error) {
	type row struct {
		ArticleID      ctype.ID
		Title          string
		Abstract       string
		Cover          string
		ViewCount      int
		AuthorNickname string
		CategoryTitle  string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("user_top_article_models AS t").
		Select("a.id AS article_id, a.title, a.abstract, a.cover, a.view_count, "+
			"COALESCE(u.nickname, '') AS author_nickname, COALESCE(c.title, '') AS category_title").
		Joins("JOIN article_models AS a ON a.id = t.article_id AND a.deleted_at IS NULL").
		Joins("LEFT JOIN user_models AS u ON u.id = a.author_id").
		Joins("LEFT JOIN category_models AS c ON c.id = a.category_id").
		Where("t.user_id = ? AND t.deleted_at IS NULL", fromUserID(userID)).
		Order("t.created_at DESC, t.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]top.Card, 0, len(rows))
	for _, m := range rows {
		out = append(out, top.Card{
			ArticleID:      domain.ArticleID(m.ArticleID),
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

// Exists 判断是否已置顶。
func (r *TopRepo) Exists(ctx context.Context, userID domain.UserID, articleID domain.ArticleID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&userTopArticleRow{}).
		Where("user_id = ? AND article_id = ?", fromUserID(userID), fromArticleID(articleID)).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteByArticle 删除某文章全部置顶。
func (r *TopRepo) DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&userTopArticleRow{}).Error
}

// ViewHistoryRepo 是 viewhistory.Repository 的 MySQL 实现。
type ViewHistoryRepo struct {
	db *gorm.DB
}

// NewViewHistoryRepo 构造仓储。
func NewViewHistoryRepo(db *gorm.DB) *ViewHistoryRepo { return &ViewHistoryRepo{db: db} }

// Upsert 写入或刷新浏览记录（唯一键 article_id + user_id）。
//
// 查询必须 **Unscoped**：唯一键不含 deleted_at，被"移出历史"/90 天清理软删过的
// 同一行仍然占着 (article_id, user_id)。带软删过滤去查就会走 Create 撞唯一键，
// 表现是用户重新打开一篇看过的文章时接口报错。所以这里找到旧行就复活它
// （deleted_at 置空 + 刷新浏览时间），只有真的没有才插入。
func (r *ViewHistoryRepo) Upsert(ctx context.Context, rec *viewhistory.Record) error {
	var m userArticleViewHistoryRow
	err := r.db.WithContext(ctx).Unscoped().
		Where("article_id = ? AND user_id = ?",
			fromArticleID(rec.ArticleID()), fromUserID(rec.UserID())).
		Take(&m).Error
	if err == nil {
		return r.db.WithContext(ctx).Unscoped().Model(&userArticleViewHistoryRow{}).
			Where("id = ?", m.ID).
			Updates(map[string]any{
				"updated_at": rec.ViewedAt(),
				"deleted_at": nil,
			}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.WithContext(ctx).Create(&userArticleViewHistoryRow{
		ArticleID: fromArticleID(rec.ArticleID()),
		UserID:    fromUserID(rec.UserID()),
	}).Error
}

// List 列出一页浏览历史（游标分页），最多 q.Limit 条。
//
// 一次 join 出列表页要的标题/封面与浏览者信息，避免 HTTP 层逐条回查。
//
// 排序是 (updated_at desc, article_id desc)：游标的第二排序键直接用契约里已经
// 暴露的 article_id（(user_id, article_id) 唯一，所以这二元组能唯一定位一条记录），
// 客户端不需要拿一个隐藏的 row id 才能续页。
//
// 过滤（onlyAdminAuthors / keyword）全部下推到这条 SQL：分页必须在**过滤之后**
// 发生，否则"搜到 1 条却给了第 2 页的空列表"。历史表里没有作者与标题快照，
// 两者都经由 article_models 现取。
func (r *ViewHistoryRepo) List(ctx context.Context, q viewhistory.ListQuery) ([]*viewhistory.Record, error) {
	db := r.db.WithContext(ctx).Table("user_article_view_history_models AS h").
		Where("h.user_id = ? AND h.deleted_at IS NULL", fromUserID(q.UserID))
	if q.OnlyAdminAuthors {
		db = db.Joins("JOIN article_models AS author_a ON author_a.id = h.article_id AND author_a.deleted_at IS NULL").
			Joins("JOIN user_models AS author_u ON author_u.id = author_a.author_id AND author_u.role = ?",
				int8(enum.RoleAdmin))
	}
	if q.Keyword != "" {
		// 与文章列表的 keyword 口径一致：按标题模糊匹配（LIKE 通配符不转义，
		// 与 ArticleRepo 的既有实现保持一致）。
		db = db.Where("a.title LIKE ?", "%"+q.Keyword+"%")
	}
	if q.Before != nil {
		// 严格早于游标位置：时间更早，或同一时刻且 article_id 更小。
		db = db.Where("(h.updated_at < ?) OR (h.updated_at = ? AND h.article_id < ?)",
			q.Before.ViewedAt, q.Before.ViewedAt, fromArticleID(q.Before.ArticleID))
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	var rows []struct {
		UpdatedAt time.Time
		ArticleID ctype.ID
		UserID    ctype.ID
		Title     string
		Cover     string
		Nickname  string
		Avatar    string
	}
	if err := db.Select("h.updated_at, h.article_id, h.user_id, " +
		"a.title AS title, a.cover AS cover, u.nickname AS nickname, u.avatar AS avatar").
		Joins("LEFT JOIN article_models a ON a.id = h.article_id AND a.deleted_at IS NULL").
		Joins("LEFT JOIN user_models u ON u.id = h.user_id").
		Order("h.updated_at desc, h.article_id desc").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*viewhistory.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, viewhistory.RestoreRecord(viewhistory.RestoreRecordParams{
			ArticleID:    domain.ArticleID(m.ArticleID),
			UserID:       domain.UserID(m.UserID),
			ViewedAt:     m.UpdatedAt,
			ArticleTitle: m.Title,
			ArticleCover: m.Cover,
			Nickname:     m.Nickname,
			Avatar:       m.Avatar,
		}))
	}
	return out, nil
}

// Delete 删除指定记录。
func (r *ViewHistoryRepo) Delete(ctx context.Context, userID domain.UserID, articleIDs []domain.ArticleID) error {
	if len(articleIDs) == 0 {
		return nil
	}
	raw := make([]ctype.ID, 0, len(articleIDs))
	for _, id := range articleIDs {
		raw = append(raw, fromArticleID(id))
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND article_id IN ?", fromUserID(userID), raw).
		Delete(&userArticleViewHistoryRow{}).Error
}

// DeleteAll 清空某用户的全部浏览历史（「清空历史」）。
func (r *ViewHistoryRepo) DeleteAll(ctx context.Context, userID domain.UserID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", fromUserID(userID)).
		Delete(&userArticleViewHistoryRow{}).Error
}

// DeleteByArticle 删除某文章全部浏览记录。
func (r *ViewHistoryRepo) DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&userArticleViewHistoryRow{}).Error
}

// ReviewRepo 是 review.Repository 的 MySQL 实现。
type ReviewRepo struct {
	db *gorm.DB
}

// NewReviewRepo 构造仓储。
func NewReviewRepo(db *gorm.DB) *ReviewRepo { return &ReviewRepo{db: db} }

// Save 新建任务。
func (r *ReviewRepo) Save(ctx context.Context, t *review.Task) error {
	m := toReviewRow(t)
	if m.ID.IsZero() {
		return r.db.WithContext(ctx).Create(m).Error
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// Update 保存任务变更。
func (r *ReviewRepo) Update(ctx context.Context, t *review.Task) error {
	m := toReviewRow(t)
	tx := r.db.WithContext(ctx).Model(&reviewTaskRow{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"status":      m.Status,
			"reason":      m.Reason,
			"reviewed_at": m.ReviewedAt,
			"reviewed_by": m.ReviewedBy,
			"stage":       m.Stage,
		})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return review.ErrTaskNotFound
	}
	return nil
}

// Find 读取任务。
func (r *ReviewRepo) Find(ctx context.Context, id uint64) (*review.Task, error) {
	var m reviewTaskRow
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.NewID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, review.ErrTaskNotFound
		}
		return nil, err
	}
	return toDomainTask(m), nil
}

// ListPendingByArticle 取某文章的待处理任务。
func (r *ReviewRepo) ListPendingByArticle(ctx context.Context, articleID domain.ArticleID) ([]*review.Task, error) {
	var rows []reviewTaskRow
	if err := r.db.WithContext(ctx).
		Where("article_id = ? AND status = ?", fromArticleID(articleID), reviewTaskPending).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*review.Task, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTask(m))
	}
	return out, nil
}

// ListAll 列出满足状态的全部任务（按风险排序用）。
//
// 与 List 同一顺序（提交时间升序、id 作同毫秒兜底），只是不分页：风险不在库里，
// 排序必须在用例层做，分页只能在排序之后切。
func (r *ReviewRepo) ListAll(ctx context.Context, status *review.Status) ([]*review.Task, error) {
	q := r.db.WithContext(ctx).Model(&reviewTaskRow{})
	if status != nil {
		q = q.Where("status = ?", string(*status))
	}
	var rows []reviewTaskRow
	if err := q.Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*review.Task, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTask(m))
	}
	return out, nil
}

// List 分页列出任务。
func (r *ReviewRepo) List(ctx context.Context, status *review.Status, offset, limit int) ([]*review.Task, int64, error) {
	q := r.db.WithContext(ctx).Model(&reviewTaskRow{})
	if status != nil {
		q = q.Where("status = ?", string(*status))
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []reviewTaskRow
	// 默认按提交时间升序：老投稿在前，新投稿不会一直插队把旧投稿饿死（#307）。
	// id 升序是同毫秒（datetime(3)）的兜底，保证分页稳定。
	if err := q.Order("created_at asc, id asc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*review.Task, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTask(m))
	}
	return out, total, nil
}

func toReviewRow(t *review.Task) *reviewTaskRow {
	m := &reviewTaskRow{
		ArticleID:            fromArticleID(t.ArticleID()),
		AuthorID:             fromUserID(t.AuthorID()),
		ArticleTitle:         t.ArticleTitle(),
		AuthorName:           t.AuthorName(),
		ArticlePublishStatus: fromStatus(t.PublishStatus()),
		Stage:                string(t.Stage()),
		Source:               string(t.Source()),
		Status:               string(t.Status()),
		Reason:               t.Reason(),
		ReviewedAt:           t.ReviewedAt(),
		ReviewedBy:           fromReviewerID(t.ReviewedBy()),
	}
	m.ID = ctype.NewID(t.ID())
	// created_at 就是提交时间：任务行创建的那一刻即作者提交的那一刻。
	// 显式写入（而不是让 gorm 用落库时刻）才能让"提交时间排序"按真实提交顺序
	// 成立，集成测试也据此可复现。
	m.CreatedAt = t.CreatedAt()
	return m
}

func toDomainTask(m reviewTaskRow) *review.Task {
	return review.RestoreTask(review.RestoreTaskParams{
		ID:            m.ID,
		ArticleID:     domain.ArticleID(m.ArticleID),
		AuthorID:      domain.UserID(m.AuthorID),
		ArticleTitle:  m.ArticleTitle,
		AuthorName:    m.AuthorName,
		PublishStatus: domain.Status(m.ArticlePublishStatus),
		Stage:         review.Stage(m.Stage),
		Source:        review.Source(m.Source),
		Status:        review.Status(m.Status),
		Reason:        m.Reason,
		ReviewedAt:    m.ReviewedAt,
		ReviewedBy:    toReviewerID(m.ReviewedBy),
		CreatedAt:     m.CreatedAt,
	})
}
