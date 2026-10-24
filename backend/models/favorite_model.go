// 收藏模型

package models

import (
	"myblogx/models/ctype"

	"gorm.io/gorm"
)

// 收藏表
type FavoriteModel struct {
	Model
	UserID        ctype.ID                `gorm:"uniqueIndex:uk_favorite_user_title,priority:1;index" json:"user_id"`
	Title         string                  `gorm:"size:32;uniqueIndex:uk_favorite_user_title,priority:2" json:"title"`
	Cover         string                  `gorm:"size:256" json:"cover"`
	Abstract      string                  `gorm:"size:256" json:"abstract"`
	IsDefault     bool                    `gorm:"default:false" json:"is_default"`
	ArticleCount  int                     `gorm:"default:0" json:"article_count"`
	OwnerNickname string                  `gorm:"size:64" json:"owner_nickname"`
	OwnerAvatar   string                  `gorm:"size:256" json:"owner_avatar"`
	UserModel     UserModel               `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleList   []UserArticleFavorModel `gorm:"foreignKey:FavorID" json:"-"`
}

func (f *FavoriteModel) BeforeDelete(tx *gorm.DB) (err error) {
	// hook 内查/删关联必须用新 session：直接复用 tx 会污染正在执行的 DELETE。
	// 且空列表不能直接 Delete——GORM 会因为没有 WHERE 报 "WHERE conditions required"。
	hook := tx.Session(&gorm.Session{NewDB: true})
	var favorList []UserArticleFavorModel
	if err = hook.Find(&favorList, "favor_id = ?", f.ID).Error; err != nil {
		return err
	}

	if len(favorList) == 0 {
		return nil
	}

	if err = hook.Delete(&favorList).Error; err != nil {
		return err
	}

	return nil
}
