package mysql

import (
	"context"
	"fmt"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
)

// cropRow 对应通用取景表 picture_crop_models。
//
// 键是 (target_type, target_id)：target_type 记主体类型（轮播封面 / 文章封面 /
// 合集封面），target_id 记该主体的 ID。取景参数体系已在 #203 全量退场，
// 表留到 #205 才物理下线；这里只剩"删主体时清 orphans"的删除路径。
type cropRow struct {
	TargetType domain.CropTargetType `gorm:"primaryKey;autoIncrement:false;size:32;not null"`
	TargetID   ctype.ID              `gorm:"primaryKey;autoIncrement:false;not null"`
	FocusX     float64
	FocusY     float64
	Zoom       float64
}

// TableName 固定表名。
func (cropRow) TableName() string { return "picture_crop_models" }

// CropRepo 是通用取景表的清理实现。
//
// 删除按 target_type 参数化，不写死轮播：轮播封面用
// domain.CropTargetBanner，文章封面用 domain.CropTargetArticle。
// 同一数值的 target_id 在两种类型下是两条互不覆盖的记录。
type CropRepo struct {
	db *gorm.DB
}

var _ domain.PictureCropRepository = (*CropRepo)(nil)

// NewCrop 构造通用取景仓储。
func NewCrop(db *gorm.DB) *CropRepo { return &CropRepo{db: db} }

// Delete 实现 domain.PictureCropRepository。
func (r *CropRepo) Delete(ctx context.Context, targetType domain.CropTargetType, targetIDs []uint64) error {
	return deleteCrops(r.db.WithContext(ctx), targetType, targetIDs)
}

// DeleteTx 在调用方的事务里真删一组取景行。
//
// 轮播图是软删（deleted_at），取景表没有软删列，删轮播时顺手真删掉它的行。
func (r *CropRepo) DeleteTx(tx *gorm.DB, targetType domain.CropTargetType, targetIDs []uint64) error {
	return deleteCrops(tx, targetType, targetIDs)
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
// 写错类型不会报错、只会静默删错行，所以入口先拦一道。
func validateCropTargetType(targetType domain.CropTargetType) error {
	if !targetType.Valid() {
		return fmt.Errorf("picture crop: unsupported target type %q", targetType)
	}
	return nil
}
