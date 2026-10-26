// Package domain 是通知模块的领域模型。
//
// 这里有两类"通知"：
//   - Message：点对点的站内信（文章被评论、被点赞、被收藏、系统通知）；
//   - GlobalNotif：管理员发布的全局公告，每个用户各自有已读/删除状态。
package domain

import (
	"myblogx/internal/kernel/ctype"
	"time"
)

// MessageType 是站内信类型。
//
// 数值必须与旧契约完全一致：前端按数字分流，改一个数字就会让历史消息跑到别的分组里。
type MessageType int8

const (
	TypeCommentArticle MessageType = 1 // 文章收到评论
	TypeCommentReply   MessageType = 2 // 评论收到回复
	TypeDiggArticle    MessageType = 3 // 文章被点赞
	TypeUnDiggArticle  MessageType = 4 // 文章被取消点赞
	TypeDiggComment    MessageType = 5 // 评论被点赞
	TypeUnDiggComment  MessageType = 6 // 评论被取消点赞
	TypeFavorArticle   MessageType = 7 // 文章被收藏
	TypeUnFavorArticle MessageType = 8 // 文章被取消收藏
	TypeSystem         MessageType = 9 // 系统通知
)

// Group 是消息列表的分组，对应接口参数 t。
type Group int8

const (
	GroupComment   Group = 1 // 评论和回复
	GroupDiggFavor Group = 2 // 点赞和收藏
	GroupSystem    Group = 3 // 系统通知
)

// Valid 判断分组是否在合法区间。
func (g Group) Valid() bool { return g >= GroupComment && g <= GroupSystem }

// Types 返回该分组覆盖的消息类型。
func (g Group) Types() []MessageType {
	switch g {
	case GroupComment:
		return []MessageType{TypeCommentArticle, TypeCommentReply}
	case GroupDiggFavor:
		return []MessageType{TypeDiggArticle, TypeDiggComment, TypeFavorArticle}
	case GroupSystem:
		return []MessageType{TypeSystem}
	default:
		return nil
	}
}

// Message 是一条站内信。
type Message struct {
	ID       ctype.ID
	Type     MessageType
	Receiver uint64

	// ActionUser 是触发人；系统通知可以没有。
	ActionUser         uint64
	ActionUserNickname string
	ActionUserAvatar   string

	Content string

	// 触发的业务对象，前端据此跳转。
	ArticleID    ctype.ID
	CommentID    ctype.ID
	ArticleTitle string

	// 额外提示链接（系统通知用）。
	LinkTitle string
	LinkHref  string

	IsRead    bool
	ReadAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// VisibleRule 是全局公告对谁可见。
type VisibleRule int8

const (
	VisibleRegistered VisibleRule = 1 // 发布时已注册的老用户（默认）
	VisibleNew        VisibleRule = 2 // 发布之后才注册的新用户
	VisibleAll        VisibleRule = 3 // 所有用户
)

// Effective 把非法值收敛为默认规则。
func (r VisibleRule) Effective() VisibleRule {
	switch r {
	case VisibleNew, VisibleAll:
		return r
	default:
		return VisibleRegistered
	}
}

// GlobalNotif 是管理员发布的全局公告。
type GlobalNotif struct {
	ID          ctype.ID
	VisibleRule VisibleRule
	ExpireAt    time.Time
	ActionUser  uint64
	Title       string
	Icon        string
	Content     string
	Href        string
	CreatedAt   time.Time
}

// VisibleAt 判断某用户在 now 时刻能否看到该公告。
//
// 这是旧 SQL 三段 OR 的等价改写，逐条对应：
//   - 所有用户：只看是否过期；
//   - 老用户：公告发布时间不早于用户注册时间；
//   - 新用户：公告发布时间早于用户注册时间。
//
// 另外旧 SQL 每条分支都带 `expire_time >= user.created_at`，
// 这条在"公告已过期则不可见"的前提下恒成立，这里统一前置判断。
func (n GlobalNotif) VisibleAt(userCreatedAt, now time.Time) bool {
	if !n.ExpireAt.After(now) || n.ExpireAt.Before(userCreatedAt) {
		return false
	}
	switch n.VisibleRule.Effective() {
	case VisibleAll:
		return true
	case VisibleNew:
		return n.CreatedAt.Before(userCreatedAt)
	default:
		return !n.CreatedAt.Before(userCreatedAt)
	}
}

// Conf 是用户的消息通知开关。
type Conf struct {
	DiggNoticeEnabled        bool
	CommentNoticeEnabled     bool
	FavorNoticeEnabled       bool
	PrivateChatNoticeEnabled bool
}

// Page 是列表查询参数：分页 + 关键字。
type Page struct {
	Page  int
	Limit int
	// Key 是标题/内容模糊匹配关键字。只有公告列表用得到，其它列表忽略。
	Key string
}

// MaxLimit 是单页上限，防止前端传个大数把库拖死。
const MaxLimit = 100

// Normalize 收敛分页参数。
func (p Page) Normalize() Page {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	return p
}

// Offset 返回 SQL 偏移量。
func (p Page) Offset() int {
	p = p.Normalize()
	return (p.Page - 1) * p.Limit
}
