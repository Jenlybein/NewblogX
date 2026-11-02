package domain

import (
	"context"

	"myblogx/internal/kernel/ctype"
)

// Counts 是文章计数的实时增量（Redis 里尚未落库的部分）。
type Counts struct {
	Favor   map[ctype.ID]int
	Digg    map[ctype.ID]int
	View    map[ctype.ID]int
	Comment map[ctype.ID]int
}

// CounterReader 批量读文章计数增量。
type CounterReader interface {
	Batch(ids []ctype.ID) Counts
}

// UserPreferences 读用户偏好标签（用于"猜你喜欢"加权）。
type UserPreferences interface {
	LikeTagIDs(ctx context.Context, userID ctype.ID) ([]ctype.ID, error)
}

// Crop 是封面取景参数（焦点 0–1 的相对坐标 + 缩放 1–2）。
//
// 与文章详情里的 `crop` 同一口径：列表封面也要按作者选定的取景裁切，
// 否则作者在创作页调好的构图到了列表页又回到"居中"，看起来像取景没生效。
//
// 它随 ES 文档一起投影（`article_sync` 写入、`extract` 读出），
// 所以改取景要重新索引才对存量文档生效（README §11.4.1 的重建 + 回填流程）。
type Crop struct {
	FocusX float64 `json:"focus_x"`
	FocusY float64 `json:"focus_y"`
	Zoom   float64 `json:"zoom"`
}
