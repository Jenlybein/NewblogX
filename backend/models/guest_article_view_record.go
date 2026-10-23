// 用户文章查看历史模型

package models

import "time"
import "myblogx/models/ctype"

// 用户文章查看历史表
type GuestArticleViewRecordModel struct {
	ArticleID    ctype.ID     `gorm:"primaryKey" json:"article_id"`
	GuestIP      string       `gorm:"primaryKey" json:"guest_ip"`
	DeviceID     string       `gorm:"primaryKey" json:"device_id"`
	CreatedAt    time.Time    `gorm:"primaryKey" json:"created_at"`
	ArticleModel ArticleModel `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
}
