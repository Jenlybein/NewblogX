package models

import "myblogx/models/ctype"

// ArticleTagModel 文章和公共标签的关系表
type ArticleTagModel struct {
	Model
	ArticleID ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:1;index:idx_article_tag_article,priority:1" json:"article_id"`
	TagID     ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:2;index:idx_article_tag_tag,priority:1" json:"tag_id"`
	Article   ArticleModel
	Tag       TagModel
}
