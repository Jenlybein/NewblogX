package domain

import "context"

// Repository 是关注关系的持久化端口。
type Repository interface {
	// Follow 关注（软删记录会被恢复并刷新快照）；返回本次是否真正写入。
	Follow(ctx context.Context, f *Follow) (bool, error)
	// Unfollow 取关；返回本次是否真的删掉了活记录。
	//
	// 两个参数是"关系的两端"，不是"谁操作"：Unfollow(followedID, fansID) 删的是
	// followedID 被 fansID 关注这一条。所以"移除关注我的人"也走它
	// （Unfollow(我, 粉丝)），语义差别在用例层，不在持久化层。
	Unfollow(ctx context.Context, followedID, fansID uint64) (bool, error)
	// ListRelations 按视图分页查关系。
	//
	// kind=follow 取 ownerID 关注的人；kind=fans 取关注 ownerID 的人；
	// kind=mutual 取与 ownerID 互相关注的人（服务端过滤，不靠调用方筛）。
	ListRelations(ctx context.Context, ownerID uint64, kind RelationKind, page Page) ([]Follow, bool, error)
	// CountRelations 一次取三个视图的计数（供顶栏徽标用）。
	CountRelations(ctx context.Context, ownerID uint64) (RelationCounts, error)
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
