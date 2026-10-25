package domain

import (
	"context"
	"time"
)

// MessageRepository 是站内信的持久化端口。
type MessageRepository interface {
	// List 按接收者与类型分组分页查询；hasMore 由多取一条判断。
	List(ctx context.Context, receiver uint64, types []MessageType, page Page) ([]Message, bool, error)
	// Find 查一条属于该接收者的消息。
	Find(ctx context.Context, receiver, id uint64) (Message, error)
	// MarkRead 把一条消息标记为已读。
	MarkRead(ctx context.Context, receiver, id uint64) error
	// MarkGroupRead 把某分组下所有未读标记为已读，返回实际条数。
	MarkGroupRead(ctx context.Context, receiver uint64, types []MessageType) (int64, error)
	// Delete 删除一条属于该接收者的消息。
	Delete(ctx context.Context, receiver, id uint64) error
	// DeleteGroup 删除某分组下所有消息，返回实际条数。
	DeleteGroup(ctx context.Context, receiver uint64, types []MessageType) (int64, error)
	// Create 落库一条消息。
	Create(ctx context.Context, m *Message) error
	// ExistsDigg 判断"同一个人对同一个对象点过赞/收过藏"的消息是否已存在。
	//
	// 旧实现用这个做去重：点赞 -> 取消 -> 再点赞不应该产生第二条通知。
	ExistsDigg(ctx context.Context, actionUser uint64, typ MessageType, articleID, commentID uint64) (bool, error)
	// CountUnread 按类型统计未读数。
	CountUnread(ctx context.Context, receiver uint64) (map[MessageType]int64, error)
}

// GlobalRepository 是全局公告的持久化端口。
type GlobalRepository interface {
	// ListVisible 分页查某用户可见的公告（排除该用户已删除的）。
	ListVisible(ctx context.Context, userID uint64, userCreatedAt time.Time, page Page) ([]GlobalNotif, bool, error)
	// ListAll 分页查全部公告（管理员视角）。
	ListAll(ctx context.Context, page Page) ([]GlobalNotif, bool, error)
	// FindVisible 在给定 ID 中筛出该用户可见的公告。
	FindVisible(ctx context.Context, userID uint64, ids []uint64, userCreatedAt time.Time) ([]GlobalNotif, error)
	// MarkRead 批量标记已读，返回真正发生变化的条数。
	MarkRead(ctx context.Context, userID uint64, ids []uint64) (int, error)
	// DeleteForUser 批量对用户隐藏公告，返回实际条数。
	DeleteForUser(ctx context.Context, userID uint64, ids []uint64) (int, error)
	// ReadSet 返回这批公告里用户已读的集合。
	ReadSet(ctx context.Context, userID uint64, ids []uint64) (map[uint64]bool, error)
	// CountUnread 统计该用户可见且未读的公告数。
	CountUnread(ctx context.Context, userID uint64, userCreatedAt time.Time) (int64, error)
	// Create 新建公告。
	Create(ctx context.Context, n *GlobalNotif) error
	// ExistsTitle 判断标题是否已被占用。
	ExistsTitle(ctx context.Context, title string) (bool, error)
	// DeleteMany 管理员批量删除公告，连同所有用户的状态一起清掉。
	DeleteMany(ctx context.Context, ids []uint64) (int, error)
}

// ConfRepository 是用户通知开关的持久化端口。
type ConfRepository interface {
	// Find 读开关；用户配置不存在时返回 apperr.CodeFollowConfigMissing 等价错误码。
	Find(ctx context.Context, userID uint64) (Conf, error)
	// Replace 用给定值整体覆盖四个开关。
	//
	// 旧接口是"全量替换"语义：请求体里没带的字段会被写成 false，
	// 这里保持一致，避免前端少传字段反而保留了旧值。
	Replace(ctx context.Context, userID uint64, conf Conf) error
}

// UserDirectory 提供做通知所需的用户侧信息。
type UserDirectory interface {
	// CreatedAt 返回注册时间（公告可见性依赖它）。
	CreatedAt(ctx context.Context, userID uint64) (time.Time, error)
	// Profile 返回昵称与头像，用于消息里的操作人快照。
	Profile(ctx context.Context, userID uint64) (nickname, avatar string, err error)
}

// ChatUnreadCounter 读私信未读总数。
//
// 私信数据属于 chat 模块，这里只读一个聚合值，不碰对方的表。
type ChatUnreadCounter interface {
	UnreadTotal(ctx context.Context, userID uint64) (int64, error)
}
