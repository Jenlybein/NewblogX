package sitemsg_api

import (
	"myblogx/common"
	"myblogx/models/ctype"
)

type UserMsgConfResponseAndRequest struct {
	DiggNoticeEnabled        bool `json:"digg_notice_enabled"`
	CommentNoticeEnabled     bool `json:"comment_notice_enabled"`
	FavorNoticeEnabled       bool `json:"favor_notice_enabled"`
	PrivateChatNoticeEnabled bool `json:"private_chat_notice_enabled"`
}

type SitemsgListRequest struct {
	common.PageInfo
	T int8 `form:"t" binding:"required,oneof=1 2 3"` // 1.评论和回复 2.点赞和收藏 3.系统通知
}

type SitemsgReadRequest struct {
	ID ctype.ID `json:"id"`
	T  int8     `json:"t" binding:"omitempty,oneof=1 2 3"` // 批量已读的类型
}

type SitemsgRemoveRequest struct {
	ID ctype.ID `json:"id"`
	T  int8     `json:"t" binding:"omitempty,oneof=1 2 3"` // 批量已读的类型
}

type SitemsgUserResponse struct {
	CommentMsgCount   int `json:"comment_msg_count"`
	DiggFavorMsgCount int `json:"digg_favor_msg_count"`
	PrivateMsgCount   int `json:"private_msg_count"`
	SystemMsgCount    int `json:"system_msg_count"`
	GlobalMsgCount    int `json:"global_msg_count"`
}
