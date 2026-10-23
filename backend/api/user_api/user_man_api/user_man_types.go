package user_man_api

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"time"
)

type UserListRequest struct {
	common.PageInfo
}

type UserListResponse struct {
	ID        ctype.ID  `json:"id"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"` // 注册时间
	// ArticleCount int       `json:"article_count"`
	// FansCount    int       `json:"fans_count"`
	// FollowCount  int       `json:"follow_count"`
	IP          string    `json:"ip"`
	Addr        string    `json:"addr"`
	LastLoginAt time.Time `json:"last_login_at"` // 最后登录时间
}
