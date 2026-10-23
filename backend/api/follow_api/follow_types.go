package follow_api

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"time"
)

type FollowListRequest struct {
	common.PageInfo
	FollowedUserID ctype.ID `form:"followed_user_id"`
	UserID         ctype.ID `form:"user_id"`
}

type FollowListResponse struct {
	FollowedUserID   ctype.ID  `json:"followed_user_id"`
	FollowedNickname string    `json:"followed_nickname"`
	FollowedAvatar   string    `json:"followed_avatar"`
	FollowedAbstract string    `json:"followed_abstract"`
	FollowTime       time.Time `json:"follow_time"`
	Relation         int8      `json:"relation"`
}

type FansListRequest struct {
	common.PageInfo
	FansUserID ctype.ID `form:"fans_user_id"`
	UserID     ctype.ID `form:"user_id"`
}

type FansListResponse struct {
	FansUserID   ctype.ID  `json:"fans_user_id"`
	FansNickname string    `json:"fans_nickname"`
	FansAvatar   string    `json:"fans_avatar"`
	FansAbstract string    `json:"fans_abstract"`
	FollowTime   time.Time `json:"follow_time"`
	Relation     int8      `json:"relation"`
}
