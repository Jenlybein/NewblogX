// Package domain 是私信模块的领域层：会话/消息实体、类型、权限与摘要规则。
package domain

import (
	"context"
	"fmt"
	"hash/fnv"
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

// 私信模块写进发件箱的事件名。
//
// 命名沿用仓库既有的 "聚合.动作" 惯例（article.created 等）。
const (
	// EventMessageSent 是一条消息已落库：接收方据此做 WS 通知与未读刷新。
	EventMessageSent = "chat.message.sent"
	// EventMessagesRead 是一批消息被标记已读：发送方据此收到已读回执。
	EventMessagesRead = "chat.message.read"
)

// OutboxAggregateType 是私信事件的聚合类型（运维按它筛"私信事件卡在哪"）。
const OutboxAggregateType = "chat"

// SentNotice 是新消息通知（WS 通知通道的载荷）。
//
// 刻意**不带正文**：正文由接收方收到通知后走 HTTP 重新拉取，
// 发件箱里就不必沉淀消息内容，减少泄露面与事件体积。
type SentNotice struct {
	MessageID  ctype.ID  `json:"message_id"`
	SenderID   ctype.ID  `json:"sender_id"`
	ReceiverID ctype.ID  `json:"receiver_id"`
	SessionID  string    `json:"session_id"`
	MsgType    MsgType   `json:"msg_type"`
	SendTime   time.Time `json:"send_time"`
}

// ReadEvent 是一条已读事件（按发送者+会话聚合）。
type ReadEvent struct {
	ReaderID  ctype.ID   `json:"reader_id"`
	SenderID  ctype.ID   `json:"sender_id"`
	SessionID string     `json:"session_id"`
	MsgIDList []ctype.ID `json:"msg_id_list"`
	ReadAt    time.Time  `json:"read_at"`
}

// ToPush 把已读事件转成给发送方的回执载荷（线上 WS 消息形状不变）。
func (e ReadEvent) ToPush() ReadPush {
	return ReadPush{
		MsgType:   TypeRead,
		SessionID: e.SessionID,
		ReaderID:  e.ReaderID,
		MsgIDList: e.MsgIDList,
		ReadAt:    e.ReadAt,
	}
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

// Store 是私信持久化的完整端口（读 + 写）。
//
// 拆成 Query / Command 两个接口而不是一个大接口：读路径可以独立替换与单独构造，
// 写路径（发送、已读、删除）承担事务、幂等与发件箱副作用。用例层分别注入
// Read 与 Write（见 app.New），读写分离在接口层面就不可绕过。
type Store interface {
	Query
	Command
}

// Query 是私信读端口（读走查询）：只做纯读，不产生任何副作用。
type Query interface {
	// ListSessions 分页查会话；unscoped=true 时连已软删的会话一起看（管理员）。
	ListSessions(ctx context.Context, userID ctype.ID, unscoped bool, page Page) ([]Session, int, error)
	// FindSession 查一条会话（校验归属）。
	FindSession(ctx context.Context, sessionID string, userID ctype.ID, unscoped bool) (Session, error)
	// ListMessages 分页查消息。
	ListMessages(ctx context.Context, q MessageQuery) ([]Message, int, error)
	// UserDeletedAt 查用户对这批消息的删除时间。
	UserDeletedAt(ctx context.Context, userID ctype.ID, sessionID string, ids []ctype.ID) (map[ctype.ID]time.Time, error)
}

// Command 是私信写端口（写走命令）。
//
// 所有写方法都在事务内完成聚合写入，并把"需要通知别的进程/连接"的副作用
// 以发件箱事件的形式写进同一个事务（transactional outbox），
// 不在事务内直接改索引或缓存。
type Command interface {
	// Send 在一个事务里：建/恢复双方会话 → 插消息 → 更新双方 last_msg 与未读
	// → 写 chat.message.sent 发件箱事件。
	//
	// duplicated=true 表示发送命中了幂等键（cmd.ClientMsgID），本次没有产生新消息，
	// 返回的是第一次发送落库的那条；调用方应据此回滚发送额度预占。
	Send(ctx context.Context, cmd SendCommand, senderDisplay, receiverDisplay Display, summary string) (Message, bool, error)
	// MarkRead 批量标记已读并递减会话未读；返回本次命中的消息，并在同一事务里
	// 为每个发送人写一条 chat.message.read 发件箱事件。
	MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]Message, error)
	// DeleteMessagesForUser 写用户级删除状态（不动原消息）。
	DeleteMessagesForUser(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]Message, error)
	// DeleteSessions 推进清空水位并软删会话。
	DeleteSessions(ctx context.Context, userID ctype.ID, sessionIDs []string) ([]string, error)
}

// SendCommand 是一次发送命令。
type SendCommand struct {
	SenderID   ctype.ID
	ReceiverID ctype.ID
	MsgType    MsgType
	// Content 是已按消息类型归一化后的正文（图片消息已经是 {"kind":"image",...}）。
	Content string
	// ClientMsgID 是客户端幂等键。为空时按普通发送处理（服务端生成主键、不做幂等）。
	ClientMsgID string
	SendTime    time.Time
}

// ClientKeyIDBase 是"由幂等键派生的消息主键"的保留区间下界（2^62）。
//
// 雪花 ID 在可预见的年份里都远小于 2^62（纪元 2024、41 位毫秒要到 2058 年
// 才逼近上界），因此 [2^62, 2^63) 与正常消息主键不相交 —— 幂等消息不会撞上
// 普通消息，反之亦然。
const ClientKeyIDBase uint64 = 1 << 62

// MessageIDFromClientKey 由（发送人，客户端幂等键）导出确定性的消息主键。
//
// 为什么不用"幂等键查重表"：零 DDL 不能给 chat_msg_models 加列、也不新增表。
// 把幂等键映射到主键上，重复提交就会在 INSERT 时撞主键，由数据库保证
// "重复提交不产生第二条消息"，进程崩溃/多实例并发都不影响。
func MessageIDFromClientKey(sender ctype.ID, key string) ctype.ID {
	h := fnv.New64a()
	_, _ = h.Write([]byte("chat:" + sender.String() + "\x00" + key))
	return ctype.NewID((h.Sum64() & (ClientKeyIDBase - 1)) | ClientKeyIDBase)
}

// RelationReader 读关注关系。
type RelationReader interface {
	Batch(ctx context.Context, viewer ctype.ID, ids []ctype.ID) (map[ctype.ID]int8, error)
}

// UserDirectory 读用户展示快照。
type UserDirectory interface {
	Displays(ctx context.Context, ids []ctype.ID) (map[ctype.ID]Display, error)
}

// StrangerVerdict 是陌生人私信的两级判定结论（#247）。
//
// 之前只返回 bool：拒绝时只知道"被拒了"，于是读者侧只能看到站点级文案，
// 把"接收方个人开关关闭"的发送方引向管理员（管理员查站点总闸只会看到已开启）。
// 结论带上原因，用例层才能为两种拒绝给出不同的错误码与文案。
type StrangerVerdict int

const (
	// StrangerAllowed 两级都允许：可以发。
	StrangerAllowed StrangerVerdict = iota
	// StrangerBlockedBySite 站点总闸（site.chat.stranger_msg_enabled）关闭。
	// 这是站点策略，正确动作是联系管理员。
	StrangerBlockedBySite
	// StrangerBlockedByRecipient 接收方个人开关
	// （user_conf_models.stranger_chat_enabled）关闭。正确动作是关注对方 /
	// 等对方开启，不该去找管理员。
	StrangerBlockedByRecipient
)

// StrangerPolicy 判断"接收方是否接受陌生人发来的私信"。
//
// 结论是**两级取交集**：站点总闸（site.chat.stranger_msg_enabled）AND
// 接收方个人开关（user_conf_models.stranger_chat_enabled，默认开）。
// 所以它必须带接收方 ID —— 个人一级的开关属于接收方，不属于站点
// （#243 把 #202 的单级口径做回用户级；实现见 app/chat_wiring.go）。
//
// 只对"陌生人"调用：自己发给自己、双方已有关注关系或已存在会话时，
// 用例层不参与判定。返回值不只回答"能不能发"，还带上**哪一级拒绝**（#247）。
type StrangerPolicy interface {
	CheckStranger(ctx context.Context, receiverID ctype.ID) (StrangerVerdict, error)
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

// Logger 是窄日志接口。
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}
