// 文章点赞模型

package models

import "myblogx/models/ctype"

// 用户点赞表
type ArticleDiggModel struct {
	Model
	ArticleID    ctype.ID     `gorm:"uniqueIndex:uk_article_digg,priority:1" json:"article_id"`
	UserID       ctype.ID     `gorm:"uniqueIndex:uk_article_digg,priority:2" json:"user_id"`
	ArticleModel ArticleModel `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
	UserModel    UserModel    `gorm:"foreignKey:UserID;references:ID" json:"-"`
}
