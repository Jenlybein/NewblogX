// 评论模型

package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// 评论表
type CommentModel struct {
	Model
	Content           string             `json:"content" gorm:"type:text;not null"`
	UserID            ctype.ID           `json:"user_id"`
	UserNickname      string             `json:"user_nickname" gorm:"size:64"`
	UserAvatar        string             `json:"user_avatar" gorm:"size:256"`
	UserModel         UserModel          `json:"user_model" gorm:"foreignKey:UserID;references:ID"`
	ArticleID         ctype.ID           `json:"article_id" gorm:"not null;index:idx_article_root"`
	ReplyId           ctype.ID           `json:"reply_id" gorm:"default:0"` // 回复的评论id，0表示一级评论
	ReplyUserID       ctype.ID           `json:"reply_user_id" gorm:"default:0"`
	ReplyUserNickname string             `json:"reply_user_nickname" gorm:"size:64"`
	RootID            ctype.ID           `json:"root_id" gorm:"default:0;index:idx_article_root"` // 根评论ID，0表示本身就是一级评论
	DiggCount         int                `json:"digg_count" gorm:"default:0"`                     // 点赞数
	ReplyCount        int                `json:"reply_count" gorm:"default:0"`
	Status            enum.CommentStatus `gorm:"default:0" json:"status"`
	ArticleModel      ArticleModel       `json:"article_model" gorm:"foreignKey:ArticleID;references:ID"`
	ParentModel       *CommentModel      `json:"parent_model" gorm:"foreignKey:ReplyId;references:ID"`
}

// 用户点赞表
type CommentDiggModel struct {
	Model
	CommentID    ctype.ID     `gorm:"uniqueIndex:uk_comment_digg,priority:1" json:"comment_id"`
	UserID       ctype.ID     `gorm:"uniqueIndex:uk_comment_digg,priority:2" json:"user_id"`
	UserModel    UserModel    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	CommentModel CommentModel `gorm:"foreignKey:CommentID;references:ID" json:"-"`
}
