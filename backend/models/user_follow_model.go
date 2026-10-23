package models

import "myblogx/models/ctype"

type UserFollowModel struct {
	Model
	FollowedUserID    ctype.ID  `gorm:"uniqueIndex:uk_user_follow,priority:1" json:"followed_user_id"`
	FollowedNickname  string    `gorm:"size:64" json:"followed_nickname"`
	FollowedAvatar    string    `gorm:"size:256" json:"followed_avatar"`
	FollowedAbstract  string    `gorm:"size:256" json:"followed_abstract"`
	FansUserID        ctype.ID  `gorm:"uniqueIndex:uk_user_follow,priority:2" json:"fans_user_id"`
	FansNickname      string    `gorm:"size:64" json:"fans_nickname"`
	FansAvatar        string    `gorm:"size:256" json:"fans_avatar"`
	FansAbstract      string    `gorm:"size:256" json:"fans_abstract"`
	FollowedUserModel UserModel `gorm:"foreignKey:FollowedUserID;references:ID"`
	FansUserModel     UserModel `gorm:"foreignKey:FansUserID;references:ID"`
}
