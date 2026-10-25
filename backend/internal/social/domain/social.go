// Package domain 是社交（关注关系）模块的领域层。
package domain

import "time"

// Relation 是两人之间的关系。
type Relation int8

const (
	// RelationStranger 陌生人。
	RelationStranger Relation = 1
	// RelationFollowed 我关注了对方。
	RelationFollowed Relation = 2
	// RelationFans 对方关注了我。
	RelationFans Relation = 3
	// RelationFriend 互相关注。
	RelationFriend Relation = 4
)

// Int8 返回底层值。
func (r Relation) Int8() int8 { return int8(r) }

// Follow 是一条关注记录（含双方展示快照）。
type Follow struct {
	CreatedAt        time.Time
	FollowedUserID   uint64
	FollowedNickname string
	FollowedAvatar   string
	FollowedAbstract string
	FansUserID       uint64
	FansNickname     string
	FansAvatar       string
	FansAbstract     string
}

// FollowingItem 是关注列表的一行（字段名与旧契约一致）。
type FollowingItem struct {
	FollowedUserID   uint64    `json:"followed_user_id"`
	FollowedNickname string    `json:"followed_nickname"`
	FollowedAvatar   string    `json:"followed_avatar"`
	FollowedAbstract string    `json:"followed_abstract"`
	FollowTime       time.Time `json:"follow_time"`
	Relation         int8      `json:"relation"`
}

// FansItem 是粉丝列表的一行。
type FansItem struct {
	FansUserID   uint64    `json:"fans_user_id"`
	FansNickname string    `json:"fans_nickname"`
	FansAvatar   string    `json:"fans_avatar"`
	FansAbstract string    `json:"fans_abstract"`
	FollowTime   time.Time `json:"follow_time"`
	Relation     int8      `json:"relation"`
}

// Display 是用户展示快照。
type Display struct {
	Nickname string
	Avatar   string
	Abstract string
}

// Page 是分页参数。
type Page struct {
	Page  int
	Limit int
}

// Normalize 收敛分页（默认 10、上限 100）。
func (p Page) Normalize() Page {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return p
}

// Offset 返回偏移量。
func (p Page) Offset() int { n := p.Normalize(); return (n.Page - 1) * n.Limit }
