package message_service

import "myblogx/models/ctype"

type ArticleCommentMessage struct {
	CommentID ctype.ID
	Content   string

	ReceiverID   ctype.ID
	ActionUserID ctype.ID

	ArticleID    ctype.ID
	ArticleTitle string
}

type ArticleReplyMessage struct {
	CommentID ctype.ID
	Content   string

	ReceiverID   ctype.ID
	ActionUserID ctype.ID

	ArticleID    ctype.ID
	ArticleTitle string
}

type ArticleDiggMessage struct {
	ReceiverID   ctype.ID
	ActionUserID ctype.ID

	ArticleID    ctype.ID
	ArticleTitle string
}

type CommentDiggMessage struct {
	CommentID ctype.ID
	Content   string

	ReceiverID   ctype.ID
	ActionUserID ctype.ID

	ArticleID    ctype.ID
	ArticleTitle string
}

type ArticleFavorMessage struct {
	ReceiverID   ctype.ID
	ActionUserID ctype.ID

	ArticleID    ctype.ID
	ArticleTitle string
}

type SystemMessage struct {
	ReceiverID   ctype.ID
	ActionUserID *ctype.ID

	Content string

	// 额外提示的链接
	LinkTitle string `gorm:"size:128" json:"link_title"`
	LinkHerf  string `gorm:"size:256" json:"link_herf"`
}
