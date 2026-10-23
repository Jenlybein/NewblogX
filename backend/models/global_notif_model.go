// 全局通知模型

package models

import (
	"myblogx/models/ctype"
	"myblogx/models/enum/global_notif_enum"
	"time"

	"gorm.io/gorm"
)

// 全局通知表
type GlobalNotifModel struct {
	Model
	UserVisibleRule global_notif_enum.Type `json:"user_visible_rule"`
	ExpireTime      time.Time              `json:"expire_time"`             // 通知过期
	ActionUser      ctype.ID               `json:"action_user"`             // 操作人
	Title           string                 `gorm:"size:64" json:"title"`    // 通知标题
	Icon            string                 `gorm:"size:64" json:"icon"`     // 通知图标
	Content         string                 `gorm:"size:128" json:"content"` // 通知内容
	Href            string                 `gorm:"size:256" json:"herf"`    // 通知链接
}

func (g *GlobalNotifModel) BeforeDelete(tx *gorm.DB) (err error) {
	return tx.Where("msg_id = ?", g.ID).Delete(&UserGlobalNotifModel{}).Error
}
