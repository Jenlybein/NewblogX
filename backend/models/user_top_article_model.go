// 用户置顶文章模型

package models

import "myblogx/models/ctype"

// 用户置顶文章表
type UserTopArticleModel struct {
	Model
	UserID       ctype.ID     `gorm:"uniqueIndex:uk_user_top_article,priority:1" json:"user_id"`
	ArticleID    ctype.ID     `gorm:"uniqueIndex:uk_user_top_article,priority:2" json:"article_id"`
	UserModel    UserModel    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleModel ArticleModel `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
}
