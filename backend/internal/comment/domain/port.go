package domain

import (
	"context"
	"time"
)

// Repository 是评论的持久化端口。
type Repository interface {
	// Create 落库并回填 ID / CreatedAt。
	Create(ctx context.Context, c *Comment) error
	// FindByID 查评论（不存在返回 CodeCommentNotFound）。
	FindByID(ctx context.Context, id CommentID) (Comment, error)
	// FindPublished 查指定文章下已发布的评论（回复目标校验用）。
	FindPublished(ctx context.Context, id CommentID, articleID uint64) (Comment, error)
	// FindPublishedByID 查已发布的评论（点赞用，不需要文章 ID）。
	FindPublishedByID(ctx context.Context, id CommentID) (Comment, error)
	// FindByIDs 批量查评论。
	FindByIDs(ctx context.Context, ids []CommentID) ([]Comment, error)
	// ListRoot 分页查一级评论（created_at desc）。
	ListRoot(ctx context.Context, articleID uint64, page Page) ([]Comment, bool, error)
	// ListReplies 分页查某根评论下的回复（created_at asc）。
	ListReplies(ctx context.Context, articleID, rootID uint64, page Page) ([]Comment, bool, error)
	// ListFamily 查一条一级评论及其全部回复（删除级联用）。
	ListFamily(ctx context.Context, rootID CommentID) ([]Comment, error)
	// ListManaged 管理端分页查询。
	ListManaged(ctx context.Context, q ManagedQuery) ([]Comment, int64, error)
	// UpdateStatus 改状态。
	UpdateStatus(ctx context.Context, id CommentID, status Status) error
	// SoftDeleteMany 批量软删。
	SoftDeleteMany(ctx context.Context, ids []CommentID) error
}

// ManagedQuery 是管理端列表的查询条件。
type ManagedQuery struct {
	// Type：1 我文章下的评论、2 我发的评论、3 管理员查全部。
	Type      int8
	Status    Status
	ViewerID  uint64
	ArticleID uint64
	UserID    uint64
	// ArticleIDs 仅 Type=1 使用（该用户的文章 ID 列表）。
	ArticleIDs []uint64
	Page       Page
}

// DiggRepository 是评论点赞的持久化端口。
type DiggRepository interface {
	// Exists 判断是否已点赞（含已软删的记录）。
	Exists(ctx context.Context, commentID, userID uint64) (bool, error)
	// Delete 取消点赞；返回本次是否真的删掉了活记录。
	Delete(ctx context.Context, commentID, userID uint64) (bool, error)
	// Create 点赞（软删记录会被恢复）；返回本次是否真的新增/恢复。
	Create(ctx context.Context, commentID, userID uint64) (bool, error)
	// DeleteByComments 删除一批评论下的所有点赞记录。
	DeleteByComments(ctx context.Context, commentIDs []uint64) error
	// DiggedSet 返回 viewer 在这批评论里点过赞的集合。
	DiggedSet(ctx context.Context, viewerID uint64, commentIDs []uint64) (map[uint64]bool, error)
}

// Counters 是评论计数的增量读（Redis 里"当前值 - 表内基数"的部分）。
type Counters interface {
	// Batch 返回评论 ID → 点赞增量 / 回复增量。
	Batch(ctx context.Context, ids []uint64) (digg, reply map[uint64]int)
}

// RelationReader 读 viewer 与一批用户的关系。
type RelationReader interface {
	Batch(ctx context.Context, viewerID uint64, userIDs []uint64) (map[uint64]int8, error)
}

// UserDirectory 读用户展示快照。
type UserDirectory interface {
	Displays(ctx context.Context, ids []uint64) (map[uint64]Display, error)
}

// Display 是评论列表需要的用户展示信息。
type Display struct {
	Nickname string
	Avatar   string
}

// Article 是评论模块需要的最小文章信息。
type Article struct {
	ID             uint64
	AuthorID       uint64
	Title          string
	Cover          string
	CommentsToggle bool
}

// ArticleInfo 读文章信息与"某作者的文章 ID 列表"。
type ArticleInfo interface {
	ByID(ctx context.Context, id uint64) (Article, error)
	BaseMap(ctx context.Context, ids []uint64) (map[uint64]Article, error)
	IDsByAuthor(ctx context.Context, authorID, articleID uint64) ([]uint64, error)
}

// CacheWriter 回写评论相关的计数缓存。
type CacheWriter interface {
	AddArticleComments(ctx context.Context, articleID uint64, delta int) error
	AddReply(ctx context.Context, rootID uint64, delta int) error
	DelReply(ctx context.Context, rootID uint64) error
	AddDigg(ctx context.Context, commentID uint64, delta int) error
	DelDigg(ctx context.Context, commentID uint64) error
}

// CommentNotice 是"有人评论了你的文章"。
type CommentNotice struct {
	CommentID    uint64
	Content      string
	ReceiverID   uint64
	ActionUserID uint64
	ArticleID    uint64
	ArticleTitle string
}

// ReplyNotice 是"有人回复了你"。
type ReplyNotice struct {
	CommentID    uint64
	Content      string
	ReceiverID   uint64
	ActionUserID uint64
	ArticleID    uint64
	ArticleTitle string
}

// DiggNotice 是"有人点赞了你的评论"。
type DiggNotice struct {
	CommentID    uint64
	Content      string
	ReceiverID   uint64
	ActionUserID uint64
	ArticleID    uint64
	ArticleTitle string
}

// Notifier 发送评论相关通知。
//
// 现在是同步调用（旧实现是裸 goroutine）；通知失败不影响主流程，
// 后续 S4.6 会把这里换成 outbox 事件。
type Notifier interface {
	CommentCreated(ctx context.Context, in CommentNotice) error
	ReplyCreated(ctx context.Context, in ReplyNotice) error
	DiggCreated(ctx context.Context, in DiggNotice) error
}

// IDGenerator 生成评论 ID。
type IDGenerator interface {
	NextCommentID() CommentID
}

// Clock 提供当前时间。
type Clock interface {
	Now() time.Time
}

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}
