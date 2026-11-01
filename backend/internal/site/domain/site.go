// Package domain 是站点模块的领域模型。
//
// 本模块只管两件事：轮播图 CRUD，以及"读/改运行时站点配置"。
// 配置本体（conf.Site / conf.AI）由 conf 与 platform/config 定义，这里只做透传。
package domain

import (
	"time"

	"myblogx/internal/apperr"
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
	// Crop 是这张图的取景参数：露出封面的哪一块。
	//
	// 读出来时一定已填好缺省（见 DefaultCrop），所以消费方不需要再判"有没有"。
	Crop      Crop
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Crop 是轮播图的取景参数：焦点（图上 0–1 的相对坐标）+ 缩放。
//
// 存参数而不是裁好的第二份图：原图只有一份，随时能改取景而不重传
// （ADR-0013 首屏封面例外区）。
type Crop struct {
	// FocusX / FocusY 是焦点在图上的相对位置，取值 0–1；0.5/0.5 是居中。
	FocusX float64
	FocusY float64
	// Zoom 是放大倍数，取值 1–2；1 表示不放大。
	Zoom float64
}

// DefaultCrop 是"没有取景记录"时的取景：居中、缩放 1。
//
// 历史图（取景表里没有行）走这一条，渲染结果与加取景之前完全一致。
var DefaultCrop = Crop{FocusX: 0.5, FocusY: 0.5, Zoom: 1}

// 取景参数的取值范围（闭区间）。
const (
	MinFocus = 0.0
	MaxFocus = 1.0
	MinZoom  = 1.0
	MaxZoom  = 2.0
)

// Validate 校验取景参数是否在合法范围内。
//
// 越界返回错误而不是钳制：钳制会让运营看到的取景与读者看到的取景分家。
func (c Crop) Validate() error {
	ok := c.FocusX >= MinFocus && c.FocusX <= MaxFocus &&
		c.FocusY >= MinFocus && c.FocusY <= MaxFocus &&
		c.Zoom >= MinZoom && c.Zoom <= MaxZoom
	if !ok {
		return apperr.New(apperr.CodeBannerCropInvalid)
	}
	return nil
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
