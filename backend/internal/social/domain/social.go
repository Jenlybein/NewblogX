// Package domain 是社交（关注关系）模块的领域层。
package domain

import (
	"myblogx/internal/kernel/ctype"
	"time"
)

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
	FollowedUserID   ctype.ID
	FollowedNickname string
	FollowedAvatar   string
	FollowedAbstract string
	FansUserID       ctype.ID
	FansNickname     string
	FansAvatar       string
	FansAbstract     string
}

// RelationKind 是关系列表的三个视图。
//
// 为什么要有这个类型：关注与粉丝是同一件事的两个方向，互关是二者的交集。
// 旧实现把它们拆成两个端点、两种条目形状（FollowingItem / FansItem），
// 前端因此要写两套归一、还要在自己那边筛互关。三种视图共用一个查询入口
// 后，"谁是谁"只由 kind 决定，形状只有一种。
type RelationKind string

const (
	// KindFollow 我关注的人。
	KindFollow RelationKind = "follow"
	// KindFans 关注我的人。
	KindFans RelationKind = "fans"
	// KindMutual 互相关注（⊆ KindFollow ∩ KindFans）。
	KindMutual RelationKind = "mutual"
)

// String 返回规范字符串。
func (k RelationKind) String() string { return string(k) }

// ParseRelationKind 解析视图参数；只接受三个已知取值。
func ParseRelationKind(raw string) (RelationKind, bool) {
	switch RelationKind(raw) {
	case KindFollow:
		return KindFollow, true
	case KindFans:
		return KindFans, true
	case KindMutual:
		return KindMutual, true
	}
	return "", false
}

// RelationItem 是关系列表的一行，三种视图共用这一种形状。
//
// 字段名不再带方向前缀（followed_ / fans_）：一行是"某个人 + 我与他的关系"，
// 方向属于视图（kind），不属于条目。
type RelationItem struct {
	UserID     ctype.ID  `json:"user_id"`
	Nickname   string    `json:"nickname"`
	Avatar     string    `json:"avatar"`
	Abstract   string    `json:"abstract"`
	FollowTime time.Time `json:"follow_time"`
	Relation   int8      `json:"relation"`
}

// RelationCounts 是三个视图的计数。
//
// 三个数是并列的：mutual 是 follow 与 fans 的交集，所以 follow ≥ mutual、
// fans ≥ mutual，但 follow 与 fans 之间没有大小关系。
type RelationCounts struct {
	Follow int `json:"follow"`
	Fans   int `json:"fans"`
	Mutual int `json:"mutual"`
}

// Counterpart 返回本视图里"对方"那一端的快照。
//
// 一条关注记录存了双方快照，但列表里每行只该出现一个人：
// kind=fans 时对方是粉丝（谁关注了我），其余视图（follow / mutual）对方是被关注者
// （我关注了谁）。把这段投影放在领域层，是为了让"两种视图各错一位"这种 bug
// 没有第二个写法。
func (f Follow) Counterpart(kind RelationKind) (id ctype.ID, nickname, avatar, abstract string) {
	if kind == KindFans {
		return f.FansUserID, f.FansNickname, f.FansAvatar, f.FansAbstract
	}
	return f.FollowedUserID, f.FollowedNickname, f.FollowedAvatar, f.FollowedAbstract
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
