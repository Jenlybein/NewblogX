package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/models"
	"myblogx/models/ctype"
)

// FavoriteRepo 是 favorite.Repository 的 MySQL 实现。
type FavoriteRepo struct {
	db *gorm.DB
}

// NewFavoriteRepo 构造仓储。
func NewFavoriteRepo(db *gorm.DB) *FavoriteRepo { return &FavoriteRepo{db: db} }

// FindFolder 读取收藏夹。
func (r *FavoriteRepo) FindFolder(ctx context.Context, id favorite.ID) (*favorite.Folder, error) {
	var m models.FavoriteModel
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
	var m models.FavoriteModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", fromUserID(userID), true).
		Take(&m).Error
	if err == nil {
		return toDomainFolder(m), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	created := &models.FavoriteModel{
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
func (r *FavoriteRepo) ListFolders(ctx context.Context, userID domain.UserID) ([]*favorite.Folder, error) {
	var rows []models.FavoriteModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", fromUserID(userID)).
		Order("is_default desc, id asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*favorite.Folder, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainFolder(m))
	}
	return out, nil
}

// ExistsFolderTitle 判断名称重复。
func (r *FavoriteRepo) ExistsFolderTitle(ctx context.Context, userID domain.UserID, title string, excludeID favorite.ID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&models.FavoriteModel{}).
		Where("user_id = ? AND title = ?", fromUserID(userID), title)
	if excludeID != 0 {
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
	updates := r.db.WithContext(ctx).Model(&models.FavoriteModel{}).
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
	return r.db.WithContext(ctx).Create(&models.FavoriteModel{
		Model:     models.Model{ID: id},
		UserID:    fromUserID(f.UserID()),
		Title:     f.Title(),
		Cover:     f.Cover(),
		Abstract:  f.Abstract(),
		IsDefault: f.IsDefault(),
	}).Error
}

// DeleteFolder 删除收藏夹（FavoriteModel 的 BeforeDelete 会清掉收藏关系）。
func (r *FavoriteRepo) DeleteFolder(ctx context.Context, id favorite.ID) error {
	return r.db.WithContext(ctx).Delete(&models.FavoriteModel{}, "id = ?", ctype.ID(id)).Error
}

// FindItem 读取某夹子里的收藏。
func (r *FavoriteRepo) FindItem(ctx context.Context, articleID domain.ArticleID, userID domain.UserID, folderID favorite.ID) (*favorite.Item, error) {
	var m models.UserArticleFavorModel
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
	var m models.UserArticleFavorModel
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
	return r.db.WithContext(ctx).Create(&models.UserArticleFavorModel{
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
		Delete(&models.UserArticleFavorModel{}).Error
}

// DeleteItemsByArticle 删除某文章全部收藏。
func (r *FavoriteRepo) DeleteItemsByArticle(ctx context.Context, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&models.UserArticleFavorModel{}).Error
}

// ListItems 分页列出某夹子的收藏。
func (r *FavoriteRepo) ListItems(ctx context.Context, folderID favorite.ID, offset, limit int) ([]*favorite.Item, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.UserArticleFavorModel{}).
		Where("favor_id = ?", ctype.ID(folderID))

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []models.UserArticleFavorModel
	if err := q.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*favorite.Item, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainItem(m))
	}
	return out, total, nil
}

// CountByArticle 统计某文章被收藏次数。
func (r *FavoriteRepo) CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&models.UserArticleFavorModel{}).
		Where("article_id = ?", fromArticleID(articleID)).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

func toDomainFolder(m models.FavoriteModel) *favorite.Folder {
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

func toDomainItem(m models.UserArticleFavorModel) *favorite.Item {
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
	return r.db.WithContext(ctx).Create(&models.UserTopArticleModel{
		UserID:    fromUserID(e.UserID()),
		ArticleID: fromArticleID(e.ArticleID()),
	}).Error
}

// Delete 取消置顶（幂等）。
func (r *TopRepo) Delete(ctx context.Context, userID domain.UserID, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND article_id = ?", fromUserID(userID), fromArticleID(articleID)).
		Delete(&models.UserTopArticleModel{}).Error
}

// ListArticleIDs 列出置顶文章。
func (r *TopRepo) ListArticleIDs(ctx context.Context, userID domain.UserID) ([]domain.ArticleID, error) {
	var rows []models.UserTopArticleModel
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
	if err := r.db.WithContext(ctx).Model(&models.UserTopArticleModel{}).
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
		Delete(&models.UserTopArticleModel{}).Error
}

// ViewHistoryRepo 是 viewhistory.Repository 的 MySQL 实现。
type ViewHistoryRepo struct {
	db *gorm.DB
}

// NewViewHistoryRepo 构造仓储。
func NewViewHistoryRepo(db *gorm.DB) *ViewHistoryRepo { return &ViewHistoryRepo{db: db} }

// Upsert 写入或刷新浏览记录（唯一键 article_id + user_id）。
func (r *ViewHistoryRepo) Upsert(ctx context.Context, rec *viewhistory.Record) error {
	var m models.UserArticleViewHistoryModel
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND user_id = ?",
			fromArticleID(rec.ArticleID()), fromUserID(rec.UserID())).
		Take(&m).Error
	if err == nil {
		return r.db.WithContext(ctx).Model(&models.UserArticleViewHistoryModel{}).
			Where("id = ?", m.ID).
			Update("updated_at", rec.ViewedAt()).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.WithContext(ctx).Create(&models.UserArticleViewHistoryModel{
		ArticleID: fromArticleID(rec.ArticleID()),
		UserID:    fromUserID(rec.UserID()),
	}).Error
}

// List 列出浏览历史。
func (r *ViewHistoryRepo) List(ctx context.Context, userID domain.UserID, offset, limit int) ([]*viewhistory.Record, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.UserArticleViewHistoryModel{}).
		Where("user_id = ?", fromUserID(userID))

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []models.UserArticleViewHistoryModel
	if err := q.Order("updated_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*viewhistory.Record, 0, len(rows))
	for _, m := range rows {
		out = append(out, viewhistory.NewRecord(
			domain.UserID(m.UserID), domain.ArticleID(m.ArticleID), m.UpdatedAt))
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
		Delete(&models.UserArticleViewHistoryModel{}).Error
}

// DeleteByArticle 删除某文章全部浏览记录。
func (r *ViewHistoryRepo) DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error {
	return r.db.WithContext(ctx).
		Where("article_id = ?", fromArticleID(articleID)).
		Delete(&models.UserArticleViewHistoryModel{}).Error
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
	if m.ID == 0 {
		return r.db.WithContext(ctx).Create(m).Error
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// Update 保存任务变更。
func (r *ReviewRepo) Update(ctx context.Context, t *review.Task) error {
	m := toReviewRow(t)
	tx := r.db.WithContext(ctx).Model(&models.ArticleReviewTaskModel{}).
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
	var m models.ArticleReviewTaskModel
	if err := r.db.WithContext(ctx).Take(&m, "id = ?", ctype.ID(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, review.ErrTaskNotFound
		}
		return nil, err
	}
	return toDomainTask(m), nil
}

// ListPendingByArticle 取某文章的待处理任务。
func (r *ReviewRepo) ListPendingByArticle(ctx context.Context, articleID domain.ArticleID) ([]*review.Task, error) {
	var rows []models.ArticleReviewTaskModel
	if err := r.db.WithContext(ctx).
		Where("article_id = ? AND status = ?", fromArticleID(articleID), models.ArticleReviewTaskPending).
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
	q := r.db.WithContext(ctx).Model(&models.ArticleReviewTaskModel{})
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
	var rows []models.ArticleReviewTaskModel
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*review.Task, 0, len(rows))
	for _, m := range rows {
		out = append(out, toDomainTask(m))
	}
	return out, total, nil
}

func toReviewRow(t *review.Task) *models.ArticleReviewTaskModel {
	m := &models.ArticleReviewTaskModel{
		ArticleID:            fromArticleID(t.ArticleID()),
		AuthorID:             fromUserID(t.AuthorID()),
		ArticleTitle:         t.ArticleTitle(),
		AuthorName:           t.AuthorName(),
		ArticlePublishStatus: fromStatus(t.PublishStatus()),
		Stage:                models.ArticleReviewTaskStage(t.Stage()),
		Source:               models.ArticleReviewTaskSource(t.Source()),
		Status:               models.ArticleReviewTaskStatus(t.Status()),
		Reason:               t.Reason(),
		ReviewedAt:           t.ReviewedAt(),
		ReviewedBy:           fromReviewerID(t.ReviewedBy()),
	}
	m.ID = ctype.ID(t.ID())
	return m
}

func toDomainTask(m models.ArticleReviewTaskModel) *review.Task {
	return review.RestoreTask(review.RestoreTaskParams{
		ID:            uint64(m.ID),
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
