// Package domain 是搜索模块的领域层：请求归一化、响应结构与分页语义。
package domain

import (
	"strings"
	"time"

	"myblogx/models/ctype"
	"myblogx/utils/markdown"
)

// PageMode 是分页模式。
type PageMode string

const (
	// PageModeHasMore 只关心"还有没有下一页"，不查总数。
	PageModeHasMore PageMode = "has_more"
	// PageModeCount 需要总数与总页数。
	PageModeCount PageMode = "count"
)

// ArticleStatus 是文章发布状态（与库里的数值一致）。
type ArticleStatus int8

const (
	// StatusDraft 草稿。
	StatusDraft ArticleStatus = 1
	// StatusExamining 审核中。
	StatusExamining ArticleStatus = 2
	// StatusPublished 已发布。
	StatusPublished ArticleStatus = 3
	// StatusRejected 已拒绝。
	StatusRejected ArticleStatus = 4
)

// VisibilityStatus 是文章可见性（ES 里存字符串）。
type VisibilityStatus string

// VisibilityVisible 公开可见。
const VisibilityVisible VisibilityStatus = "visible"

// PageInfo 是分页参数。
type PageInfo struct {
	Limit int
	Page  int
	Key   string
	Order string
}

// NormalizePage 返回合法页码。
func (p PageInfo) NormalizePage() int {
	if p.Page <= 0 {
		return 1
	}
	return p.Page
}

// GetLimit 返回合法每页条数（默认 10、上限 100）。
func (p PageInfo) GetLimit() int {
	if p.Limit <= 0 || p.Limit > 100 {
		return 10
	}
	return p.Limit
}

// Request 是文章搜索请求。
type Request struct {
	PageInfo
	// Type：1 公共/搜索 2 猜你喜欢 3 作者文章 4 自己文章 5 管理员。
	Type int8
	// Sort：1 默认 2 最新 3 回复 4 点赞 5 收藏 6 浏览。
	Sort          int8
	PageMode      PageMode
	TagIDs        []ctype.ID
	CategoryID    ctype.ID
	AuthorID      ctype.ID
	Status        ArticleStatus
	Key           string
	LegacyUserID  ctype.ID
	LegacyTagList []string
}

// NormalizeType 默认公共搜索。
func (r Request) NormalizeType() int8 {
	if r.Type == 0 {
		return 1
	}
	return r.Type
}

// NormalizeSort 默认综合排序。
func (r Request) NormalizeSort() int8 {
	if r.Sort == 0 {
		return 1
	}
	return r.Sort
}

// NormalizePageMode 按类型给默认分页语义。
func (r Request) NormalizePageMode() PageMode {
	if r.PageMode == PageModeCount {
		return PageModeCount
	}
	if r.PageMode == PageModeHasMore {
		return PageModeHasMore
	}
	switch r.NormalizeType() {
	case 4, 5:
		return PageModeCount
	default:
		return PageModeHasMore
	}
}

// NormalizeAuthorID 兼容旧 user_id 参数。
func (r Request) NormalizeAuthorID() ctype.ID {
	if r.AuthorID != 0 {
		return r.AuthorID
	}
	return r.LegacyUserID
}

// Normalize 把请求收敛到合法范围（除身份校验外）。
func (r Request) Normalize() Request {
	r.Type = r.NormalizeType()
	r.Sort = r.NormalizeSort()
	r.PageMode = r.NormalizePageMode()
	r.AuthorID = r.NormalizeAuthorID()
	r.Key = strings.TrimSpace(r.Key)
	return r
}

// Tag 是标签展示项。
type Tag struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

// Category 是分类展示项。
type Category struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

// Author 是作者展示项。
type Author struct {
	ID       ctype.ID `json:"id"`
	Nickname string   `json:"nickname"`
	Avatar   string   `json:"avatar"`
}

// Top 是置顶标记。
type Top struct {
	User  bool `json:"user"`
	Admin bool `json:"admin"`
}

// Highlight 是高亮片段。
type Highlight struct {
	Title    string `json:"title,omitempty"`
	Abstract string `json:"abstract,omitempty"`
}

// Item 是搜索结果的一行。
type Item struct {
	ID               ctype.ID         `json:"id"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Title            string           `json:"title"`
	Abstract         string           `json:"abstract,omitempty"`
	Cover            string           `json:"cover"`
	ViewCount        int              `json:"view_count"`
	DiggCount        int              `json:"digg_count"`
	CommentCount     int              `json:"comment_count"`
	FavorCount       int              `json:"favor_count"`
	CommentsToggle   bool             `json:"comments_toggle"`
	PublishStatus    ArticleStatus    `json:"publish_status"`
	VisibilityStatus VisibilityStatus `json:"visibility_status"`
	Tags             []Tag            `json:"tags"`
	Category         *Category        `json:"category,omitempty"`
	Author           Author           `json:"author"`
	Top              *Top             `json:"top,omitempty"`
	Highlight        *Highlight       `json:"highlight,omitempty"`
	Score            float64          `json:"score,omitempty"`

	Content       string                 `json:"-"`
	Part          []markdown.ContentPart `json:"-"`
	UserTop       bool                   `json:"-"`
	AdminTop      bool                   `json:"-"`
	CategoryTitle string                 `json:"-"`
	UserNickname  string                 `json:"-"`
	UserAvatar    string                 `json:"-"`
}

// Pagination 是分页信息。
type Pagination struct {
	Mode       PageMode `json:"mode"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	HasMore    bool     `json:"has_more"`
	Total      *int     `json:"total,omitempty"`
	TotalPages *int     `json:"total_pages,omitempty"`
}

// Response 是搜索结果。
type Response struct {
	List       []Item     `json:"list"`
	Pagination Pagination `json:"pagination"`
}

// Viewer 是搜索操作者（匿名时为零值）。
type Viewer struct {
	ID    ctype.ID
	Admin bool
}

// IsZero 判断是否匿名。
func (v Viewer) IsZero() bool { return v.ID == 0 }
