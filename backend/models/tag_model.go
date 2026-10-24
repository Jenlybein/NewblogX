package models

import (
	"myblogx/models/ctype"

	"gorm.io/gorm"
)

// TagModel 公共标签词库
type TagModel struct {
	Model
	Title        string         `gorm:"size:64;uniqueIndex" json:"title"`
	Sort         int            `gorm:"default:0" json:"sort"`
	Description  string         `gorm:"size:255" json:"description"`
	ArticleCount int            `gorm:"default:0" json:"article_count"`
	IsEnabled    bool           `gorm:"default:true;index" json:"is_enabled"`
	CreatedBy    ctype.ID       `gorm:"index" json:"created_by"`
	ArticleList  []ArticleModel `gorm:"many2many:article_tag_models;joinForeignKey:TagID;joinReferences:ArticleID" json:"-"`
}

func (t *TagModel) BeforeDelete(tx *gorm.DB) (err error) {
	// 同 FavoriteModel.BeforeDelete：hook 内用新 session，避免污染主 DELETE 语句。
	hook := tx.Session(&gorm.Session{NewDB: true})
	var relationList []ArticleTagModel
	if err = hook.Find(&relationList, "tag_id = ?", t.ID).Error; err != nil {
		return err
	}
	if len(relationList) == 0 {
		return nil
	}
	if err = hook.Delete(&relationList).Error; err != nil {
		return err
	}
	return nil
}
