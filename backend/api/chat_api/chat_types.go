package chat_api

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"myblogx/models/enum/chat_msg_enum"
	"time"
)

type ChatMsgListRequest struct {
	common.PageInfo
	SessionID string   `form:"session_id" binding:"required"`
	UserID    ctype.ID `form:"user_id"`
	Type      int8     `form:"type" binding:"required,oneof=1 2"`
}

type ChatSessionListRequest struct {
	common.PageInfo
	UserID ctype.ID `form:"user_id"`
	Type   int8     `form:"type" binding:"required,oneof=1 2"`
}

type ChatSessionDeleteUserRequest struct {
	SessionIDList []string `json:"session_id_list" binding:"required"`
}

type ChatMsgDeleteUserRequest struct {
	MsgIDList []ctype.ID `json:"msg_id_list" binding:"required"`
}

type ChatMsgReadUserRequest struct {
	MsgIDList []ctype.ID `json:"msg_id_list" binding:"required"`
}

type ChatSessionListResponse struct {
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

type ChatRequest struct {
	ReceiverID ctype.ID              `json:"receiver_id" binding:"required"`
	MsgType    chat_msg_enum.MsgType `json:"msg_type" binding:"required,oneof=1 2 7"` // 1 文本 2 图片 7 Markdown
	Content    string                `json:"content" binding:"required"`
}

type ChatMsgResponse struct {
	ID         ctype.ID                `json:"id"`
	SenderID   ctype.ID                `json:"sender_id"`
	ReceiverID ctype.ID                `json:"receiver_id"`
	SessionID  string                  `json:"session_id"`
	Content    string                  `json:"content"`
	SendTime   time.Time               `json:"send_time"`
	MsgStatus  chat_msg_enum.MsgStatus `json:"msg_status"`
	MsgType    chat_msg_enum.MsgType   `json:"msg_type"`
	IsSelf     bool                    `json:"is_self"`
	IsRead     bool                    `json:"is_read"`
	DeletedAt  *time.Time              `json:"deleted_at,omitempty"`
}

type ChatMsgReadPush struct {
	MsgType   chat_msg_enum.MsgType `json:"msg_type"`
	SessionID string                `json:"session_id"`
	ReaderID  ctype.ID              `json:"reader_id"`
	MsgIDList []ctype.ID            `json:"msg_id_list"`
	ReadAt    time.Time             `json:"read_at"`
}
