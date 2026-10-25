package domain

import "context"

// Repository 是关注关系的持久化端口。
type Repository interface {
	// Follow 关注（软删记录会被恢复并刷新快照）；返回本次是否真正写入。
	Follow(ctx context.Context, f *Follow) (bool, error)
	// Unfollow 取关；返回本次是否真的删掉了活记录。
	Unfollow(ctx context.Context, followedID, fansID uint64) (bool, error)
	// ListFollowing 分页查某人关注的人。
	ListFollowing(ctx context.Context, ownerID, followedFilter uint64, page Page) ([]Follow, bool, error)
	// ListFans 分页查某人的粉丝。
	ListFans(ctx context.Context, ownerID, fansFilter uint64, page Page) ([]Follow, bool, error)
	// RelationBatch 批量计算 viewer 与一批用户的关系。
	RelationBatch(ctx context.Context, viewerID uint64, ids []uint64) (map[uint64]Relation, error)
}

// UserStats 更新用户统计（关注数 / 粉丝数）。
type UserStats interface {
	ApplyFollowDelta(ctx context.Context, fansID, followedID uint64, delta int) error
}

// UserDisplay 批量读用户展示快照。
type UserDisplay interface {
	Displays(ctx context.Context, ids []uint64) (map[uint64]Display, error)
}

// Privacy 读用户列表可见性设置。
type Privacy interface {
	FollowVisible(ctx context.Context, userID uint64) (bool, error)
	FansVisible(ctx context.Context, userID uint64) (bool, error)
}

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}
