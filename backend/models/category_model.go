// 分类模型

package models

import "myblogx/models/ctype"

// 分类表
type CategoryModel struct {
	Model
	Title       string         `gorm:"size:256;uniqueIndex:uk_category_user_title,priority:2" json:"title"`
	UserID      ctype.ID       `gorm:"uniqueIndex:uk_category_user_title,priority:1;index" json:"user_id"`
	UserModel   UserModel      `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleList []ArticleModel `gorm:"foreignKey:CategoryID" json:"-"`
}
