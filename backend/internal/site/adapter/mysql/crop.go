package mysql

import (
	"context"
	"fmt"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// cropRow 对应通用取景表 picture_crop_models。
//
// 键是 (target_type, target_id)：target_type 记主体类型（轮播封面 / 文章封面 /
// 合集封面），target_id 记该主体的 ID。取景三列沿用既有口径
// （焦点 0–1、缩放 1–2）；表里没有软删列，删主体时取景行真删。
type cropRow struct {
	TargetType domain.CropTargetType `gorm:"primaryKey;autoIncrement:false;size:32;not null"`
	TargetID   ctype.ID              `gorm:"primaryKey;autoIncrement:false;not null"`
	FocusX     float64
	FocusY     float64
	Zoom       float64
}

// TableName 固定表名。
func (cropRow) TableName() string { return "picture_crop_models" }

// CropRepo 是通用取景表的仓储实现。
//
// 读写都按 target_type 参数化，不写死轮播：轮播封面用
// domain.CropTargetBanner，文章封面用 domain.CropTargetArticle。
// 同一数值的 target_id 在两种类型下是两条互不覆盖的记录。
type CropRepo struct {
	db *gorm.DB
}

var _ domain.PictureCropRepository = (*CropRepo)(nil)

// NewCrop 构造通用取景仓储。
func NewCrop(db *gorm.DB) *CropRepo { return &CropRepo{db: db} }

// Load 实现 domain.PictureCropRepository。
func (r *CropRepo) Load(ctx context.Context, targetType domain.CropTargetType, targetIDs []uint64) (map[uint64]domain.Crop, error) {
	return loadCrops(ctx, r.db, targetType, targetIDs)
}

// Upsert 实现 domain.PictureCropRepository。
func (r *CropRepo) Upsert(ctx context.Context, targetType domain.CropTargetType, targetID uint64, crop domain.Crop) error {
	return upsertCrop(r.db.WithContext(ctx), targetType, targetID, crop)
}

// UpsertTx 在调用方的事务里写入或覆盖一条取景参数。
//
// 轮播图与其取景必须一起成功：只建出轮播图的那一半，前台会按
// "居中、缩放 1"渲染，运营以为取景生效了。
func (r *CropRepo) UpsertTx(tx *gorm.DB, targetType domain.CropTargetType, targetID uint64, crop domain.Crop) error {
	return upsertCrop(tx, targetType, targetID, crop)
}

// Delete 实现 domain.PictureCropRepository。
func (r *CropRepo) Delete(ctx context.Context, targetType domain.CropTargetType, targetIDs []uint64) error {
	return deleteCrops(r.db.WithContext(ctx), targetType, targetIDs)
}

// DeleteTx 在调用方的事务里真删一组取景行。
func (r *CropRepo) DeleteTx(tx *gorm.DB, targetType domain.CropTargetType, targetIDs []uint64) error {
	return deleteCrops(tx, targetType, targetIDs)
}

// loadCrops 批量取某几条主体的取景参数。
//
// 没有记录的直接不出现在返回的 map 里，由调用方补缺省值：
// "有没有记录"与"记录里的值是不是缺省"是两件事，前者是数据形态，后者才是取值。
func loadCrops(ctx context.Context, db *gorm.DB, targetType domain.CropTargetType, ids []uint64) (map[uint64]domain.Crop, error) {
	if err := validateCropTargetType(targetType); err != nil {
		return nil, err
	}
	out := make(map[uint64]domain.Crop, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []cropRow
	if err := db.WithContext(ctx).
		Where("target_type = ? AND target_id IN ?", targetType, ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TargetID.Uint64()] = domain.Crop{FocusX: row.FocusX, FocusY: row.FocusY, Zoom: row.Zoom}
	}
	return out, nil
}

// upsertCrop 写入或覆盖一行取景参数。
func upsertCrop(tx *gorm.DB, targetType domain.CropTargetType, targetID uint64, crop domain.Crop) error {
	if err := validateCropTargetType(targetType); err != nil {
		return err
	}
	row := cropRow{
		TargetType: targetType,
		TargetID:   ctype.NewID(targetID),
		FocusX:     crop.FocusX,
		FocusY:     crop.FocusY,
		Zoom:       crop.Zoom,
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// deleteCrops 真删一组主体的取景行；id 列表为空时什么都不做。
func deleteCrops(tx *gorm.DB, targetType domain.CropTargetType, ids []uint64) error {
	if err := validateCropTargetType(targetType); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("target_type = ? AND target_id IN ?", targetType, ids).
		Delete(&cropRow{}).Error
}

// validateCropTargetType 拒绝未登记的主体类型：表里没有枚举约束，
// 写错类型不会报错、只会静默变成"没有取景"，所以入口先拦一道。
func validateCropTargetType(targetType domain.CropTargetType) error {
	if !targetType.Valid() {
		return fmt.Errorf("picture crop: unsupported target type %q", targetType)
	}
	return nil
}

// attachCrops 给一批轮播图补上取景参数：有记录用记录，没有就是缺省。
func (r *BannerRepo) attachCrops(ctx context.Context, rows []domain.Banner) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.Uint64())
	}
	byID, err := r.crops.Load(ctx, domain.CropTargetBanner, ids)
	if err != nil {
		return err
	}
	for i := range rows {
		if crop, ok := byID[rows[i].ID.Uint64()]; ok {
			rows[i].Crop = crop
			continue
		}
		rows[i].Crop = domain.DefaultCrop
	}
	return nil
}
