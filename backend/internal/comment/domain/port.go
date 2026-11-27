package domain

import (
	"context"
	"myblogx/internal/kernel/ctype"
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
	// ListAdmin 分页查管理端评论列表（含已删除行，created_at 倒序）。
	ListAdmin(ctx context.Context, filter AdminFilter, page AdminPage) ([]Comment, bool, error)
	// SoftDeleteMany 批量软删。
	SoftDeleteMany(ctx context.Context, ids []CommentID) error
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

// PublishGate 是发布类动作前的账号闸门（票 399 / ADR-0032 决策 8）。
//
// 实现由 user 模块提供、装配层注入（app/publish_gate_wiring.go）：评论模块
// 只负责在 Create 的最前面调一次，错误原样向上返回（含禁言到期时间的文案）。
// nil 表示未接入（那些不服务用户请求的进程角色），行为与改造前一致。
type PublishGate interface {
	EnsureCanPublish(ctx context.Context, userID ctype.ID) error
}

// Display 是评论列表需要的用户展示信息。
type Display struct {
	Nickname string
	Avatar   string
}

// Article 是评论模块需要的最小文章信息。
type Article struct {
	ID             ctype.ID
	AuthorID       ctype.ID
	Title          string
	Cover          string
	CommentsToggle bool
}

// ArticleInfo 读文章信息（评论创建时的开关与标题、回复通知用）。
type ArticleInfo interface {
	ByID(ctx context.Context, id uint64) (Article, error)
	// TitlesByIDs 批量取文章标题（管理端评论列表要展示"这条评论在哪篇文章下"）。
	//
	// 单独一个批量方法而不是逐条 ByID：一页评论可能来自几十篇文章，逐条查会把
	// 一次列表变成 N 次查询。实现方仍然只在装配层（app/comment_wiring.go），
	// 评论模块不因此认识 article 模块的仓储。
	//
	// 找不到的 ID 不进返回的 map（文章被删 / ID 拼错），这不算错误：一条评论
	// 挂着已经消失的文章，列表仍应该能打开。实现出错才返回 error。
	TitlesByIDs(ctx context.Context, ids []ctype.ID) (map[ctype.ID]string, error)
}

// AccountDisposer 是管理端评论页的账号处置端口（禁言 / 封禁）。
//
// 评论模块刻意不认识 user 模块：禁言与封禁的落库口径（mute_until、令牌版本 +1、
// 吊销会话、不能操作自己）全部归 user 模块所有，这里只声明"两个动作"的形状，
// 由装配层把调用翻成 user 用例（见 backend/app/comment_wiring.go）。这样评论
// 模块不必感知账号状态机，user 侧改口径也不会牵动评论代码。
type AccountDisposer interface {
	// Mute 给 target 设一次禁言，返回到期时间。
	//
	// days = 0 表示永久（管理端下拉框的"永久"档）。实现方负责把"永久"换成
	// 一个不带到期日的远期时间：调用方拿它原样回显给前端，不该再由调用方
	// 自己编造一个"3650 天后"的时间。
	Mute(ctx context.Context, actor, target ctype.ID, days int) (time.Time, error)
	// Ban 封禁 target（账号状态置"封禁"，不可自助恢复）。
	Ban(ctx context.Context, actor, target ctype.ID) error
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
	CommentID    ctype.ID
	Content      string
	ReceiverID   ctype.ID
	ActionUserID ctype.ID
	ArticleID    ctype.ID
	ArticleTitle string
}

// ReplyNotice 是"有人回复了你"。
type ReplyNotice struct {
	CommentID    ctype.ID
	Content      string
	ReceiverID   ctype.ID
	ActionUserID ctype.ID
	ArticleID    ctype.ID
	ArticleTitle string
}

// DiggNotice 是"有人点赞了你的评论"。
type DiggNotice struct {
	CommentID    ctype.ID
	Content      string
	ReceiverID   ctype.ID
	ActionUserID ctype.ID
	ArticleID    ctype.ID
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

// FilterRequest 是一段要在落库前过滤的评论文本。
//
// 端口刻意不带"文本面"参数：评论模块只知道"这是我的评论文本"，把它归到哪个面是
// 装配层的事（ADR-0032 的 surface 枚举属于内容过滤模块）。这样评论模块不必认识
// 另外五个面，将来加面也不会改到这里。
type FilterRequest struct {
	UserID ctype.ID
	// ContentID 是即将落库的评论 ID：评论在 Create 之前先生成 ID，
	// 命中记录才能指回真实内容（后续票的"删除内容"处置要靠它）。
	ContentID ctype.ID
	Text      string
}

// FilterResult 是过滤结果。
type FilterResult struct {
	// Masked 是替换后的文本；用它入库、发通知、回响应。
	Masked string
	// FilteredCount 是实际被替换的处数（只用于"已过滤 N 处"的提示）。
	FilteredCount int
}

// ContentFilterer 在全站唯一的内容过滤层里替换命中词，并记一条关键词风险。
//
// 错误语义：实现方在"命中记录写失败"时应当自己吞掉并记日志（fail-open），
// 返回 error 只留给装配级异常；即便如此，调用方也必须按"记录失败不阻断发布"
// 处理 —— 见 Service.Create 里的注释。
type ContentFilterer interface {
	Filter(ctx context.Context, in FilterRequest) (FilterResult, error)
}
