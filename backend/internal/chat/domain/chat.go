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

// MessageCursor 是"按时间轴往前翻"的复合游标。
//
// BeforeSendTime 是上一页最老那条消息的 send_time 的 RFC3339Nano 序列化；
// BeforeID 是同一条消息的 ctype.ID（十进制 uint64 的 JSON 字符串）。
// id 只用来给相同 send_time 定序：客户端幂等键派生的主键是 [2^62,2^63)
// 区间的 FNV 哈希，不是时间序，绝不能单独拿它当时序键。
// 两个字段都为零值表示"没有下一页"。
type MessageCursor struct {
	BeforeSendTime string   `json:"before_send_time"`
	BeforeID       ctype.ID `json:"before_id"`
}

// MessageListData 是 GET /chat/messages 的 data 载荷。
//
// Count 始终是该会话当前可见消息的总行数（与是否带游标无关）；
// HasMore=false 时 NextCursor 必须是零值，前端据此停止继续翻页。
type MessageListData struct {
	List       []MessageItem `json:"list"`
	Count      int           `json:"count"`
	HasMore    bool          `json:"has_more"`
	NextCursor MessageCursor `json:"next_cursor"`
}

// MessagePage 是消息列表用例返回的"一页 + 是否还有更早消息 + 下一页游标"。
type MessagePage struct {
	Items      []MessageItem
	Total      int
	HasMore    bool
	NextCursor MessageCursor
}

// MessageQueryResult 是读端口返回的一页原始消息（尚未做 IsSelf/IsRead 富化）。
type MessageQueryResult struct {
	Messages   []Message
	Total      int
	HasMore    bool
	NextCursor MessageCursor
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
//
// Cursor 非空（BeforeSendTime 或 BeforeID 任一有值）时按复合游标往前翻，
// Page.Page 被忽略；否则仍按 (page-1)*limit 偏移（管理员 / type=2 兼容）。
type MessageQuery struct {
	UserID      ctype.ID
	SessionID   string
	ClearBefore ctype.ID
	Unscoped    bool
	Page        Page
	Cursor      MessageCursor
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
	// ListMessages 分页查消息；带复合游标时多取一条以计算 HasMore。
	ListMessages(ctx context.Context, q MessageQuery) (MessageQueryResult, error)
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
	// MarkReadSession 把某会话里发给自己的未读消息一次性标记已读。
	// 幂等：没有可标记的消息时返回空切片、nil 错误。
	MarkReadSession(ctx context.Context, userID ctype.ID, sessionID string) ([]Message, error)
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

// 幂等键 → 消息主键的派生规则在 msgid 包（#279）：它是一条独立的规则，
// 不属于会话 ID / 摘要这类通用 domain 助手。号段与哈希都是它的实现细节，
// 对外只回答"这条幂等键对应哪个主键"。
//
// 由此有一条必须遵守的约束：**排序、水位、分页一律不读 ID 大小**。
// 派生主键的大小与消息先后完全无关（见 ADR-0025「读取路径与号段」）。

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

// ImageURLVerifier 判定一段「图片私信 content」是不是本站上传产物的 URL。
//
// 图片消息的 content 是通用字符串，服务端只把它包成
// {"kind":"image","image_url":content}，从不校验来源。陌生人私信开着时，
// 任意外链都能借私信塞进来（追踪像素 / 钓鱼图 / 别人的图床）。这里定一个窄端口：
// 用例层只问「是不是」，反解 object_key 与查 image_models 的脏活留在装配层。
//
// 返回 false 时用例层报 CodeChatImageNotOwned；端口本身不抛错（存储没配好时
// 一律按"不是"处理，失败关闭，不能让校验变成绕过口子）。
type ImageURLVerifier interface {
	IsOwnedImageURL(ctx context.Context, rawURL string) (bool, error)
}

// BlockLevel 是屏蔽 / 拉黑的级别（ADR-0026）。两者不是同义词：
//   - BlockLevelSoft（屏蔽）：对方发不进来，但我仍能看到历史会话；
//   - BlockLevelHard（拉黑）：屏蔽 + 我也看不到他的会话（会话列表不出现）。
type BlockLevel uint8

const (
	BlockLevelNone BlockLevel = 0
	BlockLevelSoft BlockLevel = 1
	BlockLevelHard BlockLevel = 2
)

// BlockPolicy 读"我"对"对方"的屏蔽 / 拉黑级别，以及"对方"对"我"的级别。
//
// 方向性：A 对 B 的记录不影响 B 对 A。用例层用 Outgoing(sender→receiver)
// 判"能不能发"、用 Incoming(receiver→sender) 判"对方是不是把我拉黑了"，
// 用 HardBlockedIDs 在会话列表里把"我拉黑的人"整条隐藏。
//
// 失败关闭由装配层保证：查库异常时按"没有记录"放行发送是不安全的，
// 因此 Query 出错必须原样返回错误，用例层据此中止发送。
type BlockPolicy interface {
	// Level 返回 owner 对 target 的级别；没有记录时返回 BlockLevelNone。
	Level(ctx context.Context, ownerID, targetID ctype.ID) (BlockLevel, error)
	// HardBlockedIDs 返回 owner 拉黑（level=2）的所有 target，供会话列表过滤。
	HardBlockedIDs(ctx context.Context, ownerID ctype.ID) ([]ctype.ID, error)
}

// Logger 是窄日志接口。
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}
