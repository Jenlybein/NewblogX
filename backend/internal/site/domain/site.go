// Package domain 是站点模块的领域模型。
//
// 本模块只管两件事：轮播图 CRUD，以及"读/改运行时站点配置"。
// 配置本体（conf.Site / conf.AI）由 conf 与 platform/config 定义，这里只做透传。
package domain

import (
	"strings"
	"time"

	"myblogx/internal/kernel/ctype"
)

// SensitivePlaceholder 是管理员接口回显敏感字段时用的占位符。
//
// 前端保存时会把这个值原样传回来，服务端据此判断"用户没有改密钥"。
const SensitivePlaceholder = "******"

// Banner 是首页轮播图。
type Banner struct {
	ID           ctype.ID
	Show         bool
	Cover        string
	CoverImageID ctype.ID // 零值表示没反查到图片 ID（封面可能已被删除）
	Href         string
	// Title 是轮播标题，纯文本、可空：空标题在读者站不渲染文字区。
	//
	// 存的是运营输入的原文（只收敛首尾空白），不做 HTML / Markdown 解析或渲染：
	// 读者站按纯文本插值输出，不引入清洗面。
	Title string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MaxTitleLength 是轮播标题的列宽（banner_models.title 是 varchar(256)）。
const MaxTitleLength = 256

// NormalizeTitle 把标题收敛成落库的形状：去首尾空白，超长按字符截到列宽。
//
// 只做这两件纯文本的事，不做任何 HTML / Markdown 解析：
// 标题里出现标签就原样存、原样回传，由读者站按纯文本渲染。
// 纯空白一律收敛成空串 —— 否则读者站会为一行空格渲染出空标题块。
func NormalizeTitle(raw string) string {
	title := strings.TrimSpace(raw)
	runes := []rune(title)
	if len(runes) > MaxTitleLength {
		return string(runes[:MaxTitleLength])
	}
	return title
}

// BannerQuery 是轮播图列表的查询条件。
type BannerQuery struct {
	Page  int
	Limit int
	// Show 是可选的展示状态过滤：nil 不过滤，true 只返回展示中的，
	// false 只返回隐藏的。
	Show *bool
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
