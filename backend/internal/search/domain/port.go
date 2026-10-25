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
