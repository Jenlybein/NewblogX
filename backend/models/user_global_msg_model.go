package models

import "time"

import "myblogx/models/ctype"

type UserGlobalNotifModel struct {
	Model
	MsgID  ctype.ID   `gorm:"uniqueIndex:uk_user_global_notif,priority:1" json:"msg_id"`
	UserID ctype.ID   `gorm:"uniqueIndex:uk_user_global_notif,priority:2" json:"user_id"`
	IsRead bool       `json:"is_read"`
	ReadAt *time.Time `json:"read_at"`
}
