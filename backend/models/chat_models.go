package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum/chat_msg_enum"
	"time"
)

// 存储时，为双方各存一份会话记录
// 用 UserID 存，做到 “单表快速查询”，避免复杂的联表 / 条件判断。
// 设计成只存一份的话，查的字段会多一个，性能差

// id 是数据库内部标识
// session_id 是语义化标识
// 跨表关联时（比如chat_message关联chat_session），如果只用id，一旦会话表数据迁移（如分库分表），id可能重复，导致关联错误；

// 聊天会话
type ChatSessionModel struct {
	Model
	SessionID        string     `gorm:"size:64;not null;index:idx_chat_session_id" json:"session_id"`
	UserID           ctype.ID   `gorm:"not null;uniqueIndex:uk_chat_session_user_receiver,priority:1;index:idx_chat_session_user_time,priority:1" json:"user_id"`
	ReceiverID       ctype.ID   `gorm:"not null;uniqueIndex:uk_chat_session_user_receiver,priority:2" json:"receiver_id"`
	ReceiverNickname string     `gorm:"size:64" json:"receiver_nickname"`
	ReceiverAvatar   string     `gorm:"size:256" json:"receiver_avatar"`
	LastMsgID        ctype.ID   `json:"last_msg_id"`
	LastMsgContent   string     `json:"last_msg_content"`
	LastMsgTime      *time.Time `gorm:"index:idx_chat_session_user_time,priority:2,sort:desc" json:"last_msg_time"`
	ClearBeforeMsgID ctype.ID   `json:"clear_before_msg_id"`
	UnreadCount      int        `json:"unread_count"`
	UserModel        UserModel  `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ReceiverModel    UserModel  `gorm:"foreignKey:ReceiverID;references:ID" json:"-"`
	IsTop            bool       `json:"is_top"`  // 是否置顶
	IsMute           bool       `json:"is_mute"` // 是否静音
}

// 聊天消息
type ChatMsgModel struct {
	Model
	SenderID    ctype.ID                `json:"sender_id"`
	ReceiverID  ctype.ID                `json:"receiver_id"`
	SessionID   string                  `gorm:"size:64;not null;index:idx_chat_msg_session_time,priority:1" json:"session_id"`
	Content     string                  `json:"content"`
	SendTime    time.Time               `gorm:"index:idx_chat_msg_session_time,priority:2,sort:desc" json:"send_time"`
	ReadAt      *time.Time              `json:"read_at"`
	MsgStatus   chat_msg_enum.MsgStatus `json:"msg_status"`
	MsgType     chat_msg_enum.MsgType   `json:"msg_type"`
	SessionList []ChatSessionModel      `gorm:"foreignKey:SessionID;references:SessionID" json:"-"`
}

// 聊天消息用户态
// 该表用于记录“某个用户对某条消息的本地状态”，当前只用于用户侧删除消息。
type ChatMsgUserStateModel struct {
	Model
	MsgID     ctype.ID `gorm:"not null;uniqueIndex:uk_chat_msg_user_state,priority:1;index:idx_chat_msg_user_msg,priority:2;index:idx_chat_msg_user_session_deleted,priority:4" json:"msg_id"`
	UserID    ctype.ID `gorm:"not null;uniqueIndex:uk_chat_msg_user_state,priority:2;index:idx_chat_msg_user_msg,priority:1;index:idx_chat_msg_user_session_deleted,priority:1" json:"user_id"`
	SessionID string   `gorm:"size:64;not null;index:idx_chat_msg_user_session_deleted,priority:2" json:"session_id"`
}
