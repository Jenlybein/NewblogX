package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
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
func (r *FavoriteRepo) ListFolders(ctx context.Context, userID domain.UserID) ([]*favorite.Folder, error) {
	// 不内嵌 favoriteRow：内嵌会把 TableName() 一起提升到匿名结构体上，
	// GORM 解析 schema 时以"表模型"方式处理，导致自增主键/时间列扫不出来。
	var rows []struct {
		ID           ctype.ID
		CreatedAt    time.Time
		UpdatedAt    time.Time
		UserID       ctype.ID
		Title        string
		Cover        string
		Abstract     string
		IsDefault    bool
		ArticleCount int
		Nickname     string
		Avatar       string
	}
	if err := r.db.WithContext(ctx).
		Table("favorite_models AS f").
		Select("f.*, u.nickname AS nickname, u.avatar AS avatar").
		Joins("LEFT JOIN user_models u ON u.id = f.user_id").
		Where("f.user_id = ? AND f.deleted_at IS NULL", fromUserID(userID)).
		Order("f.is_default desc, f.id asc").
		Scan(&rows).Error; err != nil {
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
		}))
	}
	return out, nil
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

// DeleteFolder 删除收藏夹（FavoriteModel 的 BeforeDelete 会清掉收藏关系）。
func (r *FavoriteRepo) DeleteFolder(ctx context.Context, id favorite.ID) error {
	return r.db.WithContext(ctx).
		Where("id = ?", ctype.ID(id)).
		Delete(&favoriteRow{}).Error
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

// SaveItem 新建收藏。
func (r *FavoriteRepo) SaveItem(ctx context.Context, i *favorite.Item) error {
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
func (r *FavoriteRepo) ListItems(ctx context.Context, folderID favorite.ID, offset, limit int) ([]*favorite.Item, int64, error) {
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Table("user_article_favor_models AS f").
			Where("f.favor_id = ? AND f.deleted_at IS NULL", ctype.ID(folderID))
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 10
	}

	var rows []struct {
		CreatedAt       time.Time
		ArticleID       ctype.ID
		UserID          ctype.ID
		ArticleTitle    string
		ArticleAbstract string
		ArticleCover    string
		ArticleStatus   int8
		AuthorID        ctype.ID
		AuthorNickname  string
		AuthorAvatar    string
		ViewCount       int
		DiggCount       int
		CommentCount    int
		FavorCount      int
	}
	if err := base().
		Select("f.created_at, f.article_id, f.user_id, f.article_title, f.article_abstract, f.article_cover, " +
			"f.article_status, f.article_author_id AS author_id, " +
			"f.article_author_nickname AS author_nickname, f.article_author_avatar AS author_avatar, " +
			"COALESCE(a.view_count,0) AS view_count, COALESCE(a.digg_count,0) AS digg_count, " +
			"COALESCE(a.comment_count,0) AS comment_count, COALESCE(a.favor_count,0) AS favor_count").
		Joins("LEFT JOIN article_models a ON a.id = f.article_id AND a.deleted_at IS NULL").
		Order("f.created_at desc, f.id desc").
		Offset(offset).Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]*favorite.Item, 0, len(rows))
	for _, m := range rows {
		out = append(out, favorite.RestoreItem(favorite.RestoreItemParams{
			ArticleID:       domain.ArticleID(m.ArticleID),
			UserID:          domain.UserID(m.UserID),
			FolderID:        folderID,
			ArticleTitle:    m.ArticleTitle,
			ArticleAbstract: m.ArticleAbstract,
			ArticleCover:    m.ArticleCover,
			ArticleStatus:   domain.Status(m.ArticleStatus),
			AuthorID:        domain.UserID(m.AuthorID),
			AuthorNickname:  m.AuthorNickname,
			AuthorAvatar:    m.AuthorAvatar,
			FavoritedAt:     m.CreatedAt,
			ViewCount:       m.ViewCount,
			DiggCount:       m.DiggCount,
			CommentCount:    m.CommentCount,
			FavorCount:      m.FavorCount,
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

// ListArticleIDs 列出置顶文章。
func (r *TopRepo) ListArticleIDs(ctx context.Context, userID domain.UserID) ([]domain.ArticleID, error) {
	var rows []userTopArticleRow
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", fromUserID(userID)).
		Order("created_at desc, id desc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ArticleID, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.ArticleID(m.ArticleID))
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
func (r *ViewHistoryRepo) Upsert(ctx context.Context, rec *viewhistory.Record) error {
	var m userArticleViewHistoryRow
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ?",
			fromArticleID(rec.ArticleID()), fromUserID(rec.UserID())).
		Take(&m).Error
	if err == nil {
		return r.db.WithContext(ctx).Model(&userArticleViewHistoryRow{}).
			Where("id = ?", m.ID).
			Update("updated_at", rec.ViewedAt()).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.WithContext(ctx).Create(&userArticleViewHistoryRow{
		ArticleID: fromArticleID(rec.ArticleID()),
		UserID:    fromUserID(rec.UserID()),
	}).Error
}

// List 列出浏览历史。
//
// 一次 join 出列表页要的标题/封面与浏览者信息，避免 HTTP 层逐条回查。
func (r *ViewHistoryRepo) List(ctx context.Context, userID domain.UserID, offset, limit int) ([]*viewhistory.Record, int64, error) {
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Table("user_article_view_history_models AS h").
			Where("h.user_id = ? AND h.deleted_at IS NULL", fromUserID(userID))
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
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
	if err := base().
		Select("h.updated_at, h.article_id, h.user_id, " +
			"a.title AS title, a.cover AS cover, u.nickname AS nickname, u.avatar AS avatar").
		Joins("LEFT JOIN article_models a ON a.id = h.article_id AND a.deleted_at IS NULL").
		Joins("LEFT JOIN user_models u ON u.id = h.user_id").
		Order("h.updated_at desc, h.id desc").
		Offset(offset).Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
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
	return out, total, nil
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
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
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
