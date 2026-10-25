// Package domain 是站点模块的领域模型。
//
// 本模块只管两件事：轮播图 CRUD，以及"读/改运行时站点配置"。
// 配置本体（conf.Site / conf.AI）由 conf 与 platform/config 定义，这里只做透传。
package domain

import "time"

// SensitivePlaceholder 是管理员接口回显敏感字段时用的占位符。
//
// 前端保存时会把这个值原样传回来，服务端据此判断"用户没有改密钥"。
const SensitivePlaceholder = "******"

// Banner 是首页轮播图。
type Banner struct {
	ID           uint64
	Show         bool
	Cover        string
	CoverImageID uint64 // 0 表示没反查到图片 ID（封面可能已被删除）
	Href         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// BannerQuery 是轮播图列表的查询条件。
type BannerQuery struct {
	Page  int
	Limit int
	// Show 对应旧接口的 show 过滤：true 只返回展示中的，false 只返回隐藏的。
	//
	// 注意这是"精确匹配"而不是"不过滤"——与旧实现一致，前端要全部数据时
	// 会分别请求两次，或者由管理端直接看列表。
	Show bool
}

// MaxLimit 是单页上限。
const MaxLimit = 100

// Normalize 收敛分页参数。
func (q BannerQuery) Normalize() BannerQuery {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > MaxLimit {
		q.Limit = MaxLimit
	}
	return q
}

// Offset 返回 SQL 偏移量。
func (q BannerQuery) Offset() int {
	q = q.Normalize()
	return (q.Page - 1) * q.Limit
}
