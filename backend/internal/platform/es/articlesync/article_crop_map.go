package articlesync

import (
	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

// cropSnapshot 是一篇文章的封面取景（焦点 0–1 + 缩放 1–2）。
//
// 取景数据不在 article_models 里：它是 ADR-0020 授权的通用取景表
// `picture_crop_models` 的一行（`target_type = 'article'`）。所以重建 ES 文档时
// 要单独查一次，把它作为**文档字段**投影出去 —— 列表封面才能按作者选的构图裁切。
type cropSnapshot struct {
	FocusX float64
	FocusY float64
	Zoom   float64
}

// cropRow 对应 picture_crop_models 里文章取景需要的列。
type cropRow struct {
	TargetType string   `gorm:"column:target_type"`
	TargetID   ctype.ID `gorm:"column:target_id"`
	FocusX     float64  `gorm:"column:focus_x"`
	FocusY     float64  `gorm:"column:focus_y"`
	Zoom       float64  `gorm:"column:zoom"`
}

// TableName 固定表名（通用取景表，banner 与 article 共用）。
func (cropRow) TableName() string { return "picture_crop_models" }

// articleCropTargetType 是文章封面在通用取景表里的主体类型。
const articleCropTargetType = "article"

// cropDocumentField 把取景快照折成 ES 文档里的 `crop` 字段。
//
// zoom 为 0 表示"没有这一段数据"（map 里查不到的零值），此时写 nil：
// 文档里多一个 `"crop": null` 会让读侧分不清"没取景"和"取景是全零"，
// 而全零的 zoom 在渲染时是非法的（缩放必须 ≥1）。
func cropDocumentField(crop cropSnapshot) any {
	if crop.Zoom <= 0 {
		return nil
	}
	return map[string]any{
		"focus_x": crop.FocusX,
		"focus_y": crop.FocusY,
		"zoom":    crop.Zoom,
	}
}

// loadArticleCropMap 批量读文章的封面取景。
//
// 查不到就表示"这些文章没有取景记录"（缺省居中、缩放 1），不是错误：
// 存量文章本来就没有这一行，不该让整次同步失败。
func loadArticleCropMap(db *gorm.DB, articleIDs []ctype.ID) (map[ctype.ID]cropSnapshot, error) {
	out := map[ctype.ID]cropSnapshot{}
	if db == nil || len(articleIDs) == 0 {
		return out, nil
	}

	raw := make([]ctype.ID, 0, len(articleIDs))
	for _, id := range articleIDs {
		if !id.IsZero() {
			raw = append(raw, id)
		}
	}
	if len(raw) == 0 {
		return out, nil
	}

	var rows []cropRow
	err := db.Table((cropRow{}).TableName()).
		Select("target_id", "focus_x", "focus_y", "zoom").
		Where("target_type = ? AND target_id IN ?", articleCropTargetType, raw).
		Find(&rows).Error
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out[row.TargetID] = cropSnapshot{
			FocusX: row.FocusX,
			FocusY: row.FocusY,
			Zoom:   row.Zoom,
		}
	}
	return out, nil
}
