package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	sitedomain "myblogx/internal/site/domain"
)

// cropRow 对应通用取景表 picture_crop_models（表与列由站点模块统一维护，本模块不加列）。
//
// 键是 (target_type, target_id)：文章封面取景只是 target_type=article 的那一类。
// 取景表没有软删列，删文章时对应的行真删。
//
// 取景参数体系已在 #203 全量退场：本模块不再读写取景参数，表留到 #205
// 才物理下线，因此这里只剩删除路径。
//
// 为什么本模块自己也有一份行结构而不是复用 site/adapter/mysql：跨模块 import
// 别人的 adapter 会把它变成事实上的公共库（archcheck R11）。表结构是共享事实，
// 行结构按模块复制，与其它跨模块只读表的分法一致。
type cropRow struct {
	TargetType sitedomain.CropTargetType `gorm:"primaryKey;autoIncrement:false;size:32;not null"`
	TargetID   ctype.ID                  `gorm:"primaryKey;autoIncrement:false;not null"`
	FocusX     float64
	FocusY     float64
	Zoom       float64
}

// TableName 固定表名。
func (cropRow) TableName() string { return "picture_crop_models" }

// CropRepo 是通用取景表的文章侧清理实现（供 app.Repos.Crops 使用）。
type CropRepo struct {
	db *gorm.DB
}

var _ sitedomain.PictureCropRepository = (*CropRepo)(nil)

// NewCropRepo 构造通用取景表仓储。
func NewCropRepo(db *gorm.DB) *CropRepo { return &CropRepo{db: db} }

// Delete 实现 sitedomain.PictureCropRepository：真删一组主体的取景行。
func (r *CropRepo) Delete(ctx context.Context, targetType sitedomain.CropTargetType, targetIDs []uint64) error {
	if err := validateCropTargetType(targetType); err != nil {
		return err
	}
	if len(targetIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("target_type = ? AND target_id IN ?", targetType, targetIDs).
		Delete(&cropRow{}).Error
}

// validateCropTargetType 拒绝未登记的主体类型：表里没有枚举约束，
// 写错类型不会报错、只会静默删错行。
func validateCropTargetType(targetType sitedomain.CropTargetType) error {
	if !targetType.Valid() {
		return fmt.Errorf("picture crop: unsupported target type %q", targetType)
	}
	return nil
}
