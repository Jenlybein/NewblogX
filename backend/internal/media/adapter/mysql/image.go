// Package mysql 是媒体模块的 MySQL 适配器。
package mysql

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// imageRow 对应 image_models 表。
type imageRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	UserID    ctype.ID
	Provider  string
	Bucket    string
	ObjectKey string
	FileName  string
	URL       string
	MimeType  string
	Size      int64
	Width     int
	Height    int
	Hash      string
	Status    uint
}

// TableName 固定表名。
func (imageRow) TableName() string { return "image_models" }

// Repo 实现图片持久化端口。
type Repo struct {
	db *gorm.DB
}

var _ domain.Repository = (*Repo)(nil)

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// FindByHash 实现 domain.Repository。
func (r *Repo) FindByHash(ctx context.Context, hash string) (domain.Image, bool, error) {
	var row imageRow
	err := r.db.WithContext(ctx).Where("hash = ?", hash).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.Image{}, false, nil
	}
	if err != nil {
		return domain.Image{}, false, err
	}
	return toImage(row), true, nil
}

// Create 实现 domain.Repository。
func (r *Repo) Create(ctx context.Context, img *domain.Image) error {
	if img == nil {
		return apperr.New(apperr.CodeImageInvalid)
	}
	if img.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		img.ID = ctype.NewID(next.Uint64())
	}
	now := time.Now()
	row := imageRow{
		ID:        img.ID,
		CreatedAt: now,
		UserID:    img.UserID,
		Provider:  string(img.Provider),
		Bucket:    img.Bucket,
		ObjectKey: img.ObjectKey,
		FileName:  img.FileName,
		URL:       img.URL,
		MimeType:  img.MimeType,
		Size:      img.Size,
		Width:     img.Width,
		Height:    img.Height,
		Hash:      img.Hash,
		Status:    uint(img.Status),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	img.CreatedAt = now
	return nil
}

// FindByID 实现 domain.Repository。
func (r *Repo) FindByID(ctx context.Context, id domain.ImageID) (domain.Image, error) {
	var row imageRow
	if err := r.db.WithContext(ctx).Take(&row, "id = ?", id.Uint64()).Error; err != nil {
		return domain.Image{}, translate(err)
	}
	return toImage(row), nil
}

// FindByObjectKey 实现 domain.Repository。
func (r *Repo) FindByObjectKey(ctx context.Context, objectKey string) (domain.Image, bool, error) {
	var row imageRow
	err := r.db.WithContext(ctx).Where("object_key = ?", objectKey).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.Image{}, false, nil
	}
	if err != nil {
		return domain.Image{}, false, err
	}
	return toImage(row), true, nil
}

// FindByIDs 实现 domain.Repository。
func (r *Repo) FindByIDs(ctx context.Context, ids []domain.ImageID) ([]domain.Image, error) {
	raw := toRawIDs(ids)
	if len(raw) == 0 {
		return nil, nil
	}
	var rows []imageRow
	if err := r.db.WithContext(ctx).Where("id IN ?", raw).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Image, 0, len(rows))
	for _, row := range rows {
		out = append(out, toImage(row))
	}
	return out, nil
}

// UpdateStatus 实现 domain.Repository。
func (r *Repo) UpdateStatus(ctx context.Context, id domain.ImageID, status domain.Status) error {
	return r.db.WithContext(ctx).Model(&imageRow{}).
		Where("id = ?", id.Uint64()).
		Update("status", uint(status)).Error
}

// List 实现 domain.Repository。
func (r *Repo) List(ctx context.Context, q domain.Query) ([]domain.Image, int64, error) {
	q = q.Normalize()
	base := func() *gorm.DB {
		tx := r.db.WithContext(ctx).Model(&imageRow{})
		if key := strings.TrimSpace(q.Key); key != "" {
			like := "%" + key + "%"
			tx = tx.Where("file_name LIKE ? OR object_key LIKE ?", like, like)
		}
		return tx
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []imageRow
	if err := base().Order("id DESC").Limit(q.Limit).Offset(q.Offset()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Image, 0, len(rows))
	for _, row := range rows {
		out = append(out, toImage(row))
	}
	return out, total, nil
}

// SoftDeleteMany 实现 domain.Repository。
func (r *Repo) SoftDeleteMany(ctx context.Context, ids []domain.ImageID) error {
	return r.db.WithContext(ctx).Where("id IN ?", toRawIDs(ids)).Delete(&imageRow{}).Error
}

// DeleteWithRefs 实现 domain.Repository。
func (r *Repo) DeleteWithRefs(ctx context.Context, ids []domain.ImageID) error {
	raw := toRawIDs(ids)
	ctypeIDs := make([]ctype.ID, 0, len(raw))
	for _, id := range raw {
		ctypeIDs = append(ctypeIDs, ctype.NewID(id))
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := imageref.DeleteImageRefsByImageIDs(tx, ctypeIDs); err != nil {
			return err
		}
		return tx.Unscoped().Where("id IN ?", raw).Delete(&imageRow{}).Error
	})
}

// ResolveURLByID 按图片 ID 取可用地址（供其他模块读）。
func ResolveURLByID(db *gorm.DB, id uint64) (string, error) {
	if id == 0 {
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
	var row imageRow
	if err := db.Select("id", "url", "status").Take(&row, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", apperr.New(apperr.CodeImageUnavailable)
		}
		return "", err
	}
	switch domain.Status(row.Status) {
	case domain.StatusPass, domain.StatusReviewing:
		return strings.TrimSpace(row.URL), nil
	default:
		return "", apperr.New(apperr.CodeImageUnavailable)
	}
}

// FindIDByURL 按地址反查图片 ID；查不到返回 false。
func FindIDByURL(db *gorm.DB, rawURL string) (uint64, bool, error) {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return 0, false, nil
	}
	var row imageRow
	if err := db.Select("id").Where("url = ?", url).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, false, nil
		}
		return 0, false, err
	}
	return row.ID.Uint64(), true, nil
}

func translate(err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperr.New(apperr.CodeImageUnavailable)
	}
	return err
}

func toRawIDs(ids []domain.ImageID) []uint64 {
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			out = append(out, id.Uint64())
		}
	}
	return out
}

func toImage(row imageRow) domain.Image {
	return domain.Image{
		ID:        ctype.NewID(row.ID.Uint64()),
		CreatedAt: row.CreatedAt,
		UserID:    row.UserID,
		Provider:  domain.Provider(row.Provider),
		Bucket:    row.Bucket,
		ObjectKey: row.ObjectKey,
		FileName:  row.FileName,
		URL:       row.URL,
		MimeType:  row.MimeType,
		Size:      row.Size,
		Width:     row.Width,
		Height:    row.Height,
		Hash:      row.Hash,
		Status:    domain.Status(row.Status),
	}
}
