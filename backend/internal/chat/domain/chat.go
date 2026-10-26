// Package domain 是私信模块的领域层：会话/消息实体、类型、权限与摘要规则。
package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/utils/markdown"
)

// MsgType 是消息类型（与库里的数值一致）。
type MsgType int8

const (
	// TypeText 文本。
	TypeText MsgType = 1
	// TypeImage 图片（Content 存 JSON）。
	TypeImage MsgType = 2
	// TypeAudio 语音。
	TypeAudio MsgType = 3
	// TypeVideo 视频。
	TypeVideo MsgType = 4
	// TypeFile 文件。
	TypeFile MsgType = 5
	// TypeEmoji 表情。
	TypeEmoji MsgType = 6
	// TypeMarkdown Markdown。
	TypeMarkdown MsgType = 7
	// TypeRead 已读通知。
	TypeRead MsgType = 8
)

// MsgStatus 是消息状态。
type MsgStatus int8

const (
	// StatusSent 已发送。
	StatusSent MsgStatus = 1
	// StatusDelivered 已送达。
	StatusDelivered MsgStatus = 2
	// StatusRead 已读。
	StatusRead MsgStatus = 3
	// StatusRecall 已撤回。
	StatusRecall MsgStatus = 4
	// StatusDeleted 已删除。
	StatusDeleted MsgStatus = 5
)

// IsRead 判断消息是否已读。
func (s MsgStatus) IsRead() bool { return s >= StatusRead }

// Session 是一条会话（双方各存一份）。
type Session struct {
	ID               ctype.ID
	SessionID        string
	UserID           ctype.ID
	ReceiverID       ctype.ID
	ReceiverNickname string
	ReceiverAvatar   string
	LastMsgContent   string
	LastMsgTime      *time.Time
	ClearBeforeMsgID ctype.ID
	UnreadCount      int
	IsTop            bool
	IsMute           bool
	DeletedAt        *time.Time
}

// Message 是一条消息。
type Message struct {
	ID         ctype.ID
	SenderID   ctype.ID
	ReceiverID ctype.ID
	SessionID  string
	Content    string
	SendTime   time.Time
	ReadAt     *time.Time
	MsgStatus  MsgStatus
	MsgType    MsgType
}

// SessionItem 是会话列表的一行（字段名与旧契约一致）。
type SessionItem struct {
	SessionID        string     `json:"session_id"`
	ReceiverID       ctype.ID   `json:"receiver_id"`
	ReceiverNickname string     `json:"receiver_nickname"`
	ReceiverAvatar   string     `json:"receiver_avatar"`
	Relation         int8       `json:"relation"`
	LastMsgContent   string     `json:"last_msg_content"`
	LastMsgTime      *time.Time `json:"last_msg_time"`
	UnreadCount      int        `json:"unread_count"`
	IsTop            bool       `json:"is_top"`
	IsMute           bool       `json:"is_mute"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// MessageItem 是消息列表的一行。
type MessageItem struct {
	ID         ctype.ID   `json:"id"`
	SenderID   ctype.ID   `json:"sender_id"`
	ReceiverID ctype.ID   `json:"receiver_id"`
	SessionID  string     `json:"session_id"`
	Content    string     `json:"content"`
	SendTime   time.Time  `json:"send_time"`
	MsgStatus  MsgStatus  `json:"msg_status"`
	MsgType    MsgType    `json:"msg_type"`
	IsSelf     bool       `json:"is_self"`
	IsRead     bool       `json:"is_read"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

// ReadPush 是给发送方的已读回执（按发送者+会话聚合）。
type ReadPush struct {
	MsgType   MsgType    `json:"msg_type"`
	SessionID string     `json:"session_id"`
	ReaderID  ctype.ID   `json:"reader_id"`
	MsgIDList []ctype.ID `json:"msg_id_list"`
	ReadAt    time.Time  `json:"read_at"`
}

// Display 是用户展示快照。
type Display struct {
	Nickname string
	Avatar   string
}

// Page 是分页参数。
type Page struct {
	Page  int
	Limit int
}

// Normalize 收敛分页（默认 10、上限 100）。
func (p Page) Normalize() Page {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return p
}

// Offset 返回偏移量。
func (p Page) Offset() int { n := p.Normalize(); return (n.Page - 1) * n.Limit }

// MessageQuery 是消息列表查询。
type MessageQuery struct {
	UserID      ctype.ID
	SessionID   string
	ClearBefore ctype.ID
	Unscoped    bool
	Page        Page
}

// BuildSessionID 生成会话 ID（小 ID 在前，保证双方一致）。
func BuildSessionID(a, b ctype.ID) string {
	if a.Uint64() < b.Uint64() {
		return fmt.Sprintf("chat:%s:%s", a, b)
	}
	return fmt.Sprintf("chat:%s:%s", b, a)
}

// Summary 生成会话列表的最后一条消息摘要。
func Summary(t MsgType, content string) string {
	var abstract string
	switch t {
	case TypeText:
		abstract = content
	case TypeImage:
		abstract = "[图片]"
	case TypeMarkdown:
		abstract = markdown.MdToText(content)
	case TypeAudio:
		abstract = "[语音]"
	case TypeVideo:
		abstract = "[视频]"
	case TypeFile:
		abstract = "[文件]"
	case TypeEmoji:
		abstract = "[表情]"
	default:
		abstract = "[消息]"
	}
	abstract = strings.TrimSpace(abstract)
	if len(abstract) > 20 {
		return string([]rune(abstract)[:20]) + "..."
	}
	return abstract
}

// Repository 是私信持久化端口。
type Repository interface {
	// SendMessage 在一个事务里：建/恢复双方会话 → 插消息 → 更新双方 last_msg 与未读。
	SendMessage(ctx context.Context, msg *Message, senderDisplay, receiverDisplay Display, summary string) error
	// ListSessions 分页查会话；unscoped=true 时连已软删的会话一起看（管理员）。
	ListSessions(ctx context.Context, userID ctype.ID, unscoped bool, page Page) ([]Session, int, error)
	// FindSession 查一条会话（校验归属）。
	FindSession(ctx context.Context, sessionID string, userID ctype.ID, unscoped bool) (Session, error)
	// ListMessages 分页查消息。
	ListMessages(ctx context.Context, q MessageQuery) ([]Message, int, error)
	// UserDeletedAt 查用户对这批消息的删除时间。
	UserDeletedAt(ctx context.Context, userID ctype.ID, sessionID string, ids []ctype.ID) (map[ctype.ID]time.Time, error)
	// MarkRead 批量标记已读并递减会话未读；返回本次命中的消息。
	MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]Message, error)
	// DeleteMessagesForUser 写用户级删除状态（不动原消息）。
	DeleteMessagesForUser(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]Message, error)
	// DeleteSessions 推进清空水位并软删会话。
	DeleteSessions(ctx context.Context, userID ctype.ID, sessionIDs []string) ([]string, error)
}

// RelationReader 读关注关系。
type RelationReader interface {
	Batch(ctx context.Context, viewer ctype.ID, ids []ctype.ID) (map[ctype.ID]int8, error)
}

// UserDirectory 读用户展示快照。
type UserDirectory interface {
	Displays(ctx context.Context, ids []ctype.ID) (map[ctype.ID]Display, error)
}

// StrangerPolicy 读"是否接收陌生人私信"。
type StrangerPolicy interface {
	StrangerEnabled(ctx context.Context, userID ctype.ID) (bool, error)
}

// Reservation 是一次发送额度预占。
type Reservation interface {
	Commit() error
	Rollback() error
}

// Limiter 做发送前限流与额度预占。
type Limiter interface {
	Reserve(ctx context.Context, senderID, receiverID ctype.ID, relation int8) (Reservation, error)
}

// Hub 是在线连接池。
type Hub interface {
	IsOnline(userID ctype.ID) bool
	PushJSON(userID ctype.ID, v any) int
}

// Logger 是窄日志接口。
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}
