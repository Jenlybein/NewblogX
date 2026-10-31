package mysql

import (
	"context"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// cropRow 对应 banner_crop_models 表：轮播图的取景参数（与 banner_models 一对一）。
//
// 为什么单独一张表：本项目零 DDL 口径下既有表是冻结的（ES 投影靠 CDC 按列名
// 解析，加列等于改列名）。新增表建得出来，改既有表不行。
type cropRow struct {
	// BannerID 既是主键也是外键语义：一张轮播图最多一行取景。
	BannerID ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	// FocusX / FocusY 是焦点在图上的相对坐标（0–1），Zoom 是放大倍数（1–2）。
	FocusX float64
	FocusY float64
	Zoom   float64
}

// TableName 固定表名。
func (cropRow) TableName() string { return "banner_crop_models" }

// loadCrops 批量取某几条轮播图的取景参数。
//
// 没有记录的直接不出现在返回的 map 里，由调用方补缺省值：
// "有没有记录"与"记录里的值是不是缺省"是两件事，前者是数据形态，后者才是取值。
func (r *BannerRepo) loadCrops(ctx context.Context, ids []uint64) (map[uint64]domain.Crop, error) {
	out := make(map[uint64]domain.Crop, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []cropRow
	if err := r.db.WithContext(ctx).Where("banner_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.BannerID.Uint64()] = domain.Crop{FocusX: row.FocusX, FocusY: row.FocusY, Zoom: row.Zoom}
	}
	return out, nil
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
	byID, err := r.loadCrops(ctx, ids)
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

// upsertCrop 写入或覆盖一行取景参数。
func upsertCrop(tx *gorm.DB, bannerID uint64, crop domain.Crop) error {
	row := cropRow{
		BannerID: ctype.NewID(bannerID),
		FocusX:   crop.FocusX,
		FocusY:   crop.FocusY,
		Zoom:     crop.Zoom,
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}
