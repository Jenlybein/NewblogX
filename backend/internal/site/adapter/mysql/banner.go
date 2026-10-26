// Package mysql 是站点模块的 MySQL 适配器。
package mysql

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
)

// bannerRow 对应 banner_models 表。
type bannerRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	Show      bool
	Cover     string
	Href      string
}

// TableName 固定表名。
func (bannerRow) TableName() string { return "banner_models" }

// BannerRepo 实现轮播图持久化端口。
type BannerRepo struct {
	db *gorm.DB
}

var _ domain.BannerRepository = (*BannerRepo)(nil)

// NewBanner 构造轮播图仓储。
func NewBanner(db *gorm.DB) *BannerRepo { return &BannerRepo{db: db} }

// List 实现 domain.BannerRepository。
func (r *BannerRepo) List(ctx context.Context, q domain.BannerQuery) ([]domain.Banner, bool, error) {
	q = q.Normalize()
	var rows []bannerRow
	err := r.db.WithContext(ctx).
		Where("show = ?", q.Show).
		Order("created_at DESC, id DESC").
		Limit(q.Limit + 1).
		Offset(q.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > q.Limit
	if hasMore {
		rows = rows[:q.Limit]
	}
	out := make([]domain.Banner, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBanner(row))
	}
	return out, hasMore, nil
}

// Find 实现 domain.BannerRepository。
func (r *BannerRepo) Find(ctx context.Context, id uint64) (domain.Banner, error) {
	var row bannerRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.Banner{}, apperr.New(apperr.CodeBannerNotFound)
		}
		return domain.Banner{}, err
	}
	return toBanner(row), nil
}

// Create 实现 domain.BannerRepository。
func (r *BannerRepo) Create(ctx context.Context, b *domain.Banner) error {
	if b == nil {
		return apperr.New(apperr.CodeBannerNotFound)
	}
	if b.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		b.ID = next
	}
	now := time.Now()
	row := bannerRow{
		ID:        b.ID,
		CreatedAt: now,
		UpdatedAt: now,
		Show:      b.Show,
		Cover:     b.Cover,
		Href:      b.Href,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	b.CreatedAt = now
	b.UpdatedAt = now
	return nil
}

// Update 实现 domain.BannerRepository。
func (r *BannerRepo) Update(ctx context.Context, id uint64, cover, href string, show bool) error {
	return r.db.WithContext(ctx).Model(&bannerRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"cover":      cover,
			"href":       href,
			"show":       show,
			"updated_at": time.Now(),
		}).Error
}

// Delete 实现 domain.BannerRepository。
func (r *BannerRepo) Delete(ctx context.Context, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&bannerRow{})
	return int(res.RowsAffected), res.Error
}

// toBanner 把表行转成领域实体。
func toBanner(row bannerRow) domain.Banner {
	return domain.Banner{
		ID:        row.ID,
		Show:      row.Show,
		Cover:     row.Cover,
		Href:      row.Href,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
