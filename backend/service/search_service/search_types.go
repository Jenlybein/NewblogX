package search_service

import (
	"myblogx/common"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/markdown"
	"time"
)

type PageMode string

const (
	PageModeHasMore PageMode = "has_more"
	PageModeCount   PageMode = "count"
)

type ArticleSearchRequest struct {
	// Type
	// 1 公共文章列表/搜索 2 猜你喜欢 3 作者文章 4 自己文章 5 管理员搜
	// Sort
	// 1 默认搜索 2 最新发布 3 最多回复
	// 4 最多点赞 5 最多收藏 6 最多浏览
	common.PageInfo
	Type     int8     `form:"type"`
	Sort     int8     `form:"sort"`
	PageMode PageMode `form:"page_mode"`
	// tag_ids 统一由 handler 做兼容解析（支持 tag_ids=1,2 与 tag_ids=1&tag_ids=2）
	// 避免 query binder 在遇到 "1,2" 时按单个 ID 解析导致失败。
	TagIDs        []ctype.ID         `form:"-"`
	CategoryID    ctype.ID           `form:"category_id"`
	AuthorID      ctype.ID           `form:"author_id"`
	Status        enum.ArticleStatus `form:"status"`
	Key           string             `form:"key"`
	LegacyUserID  ctype.ID           `form:"user_id" json:"-"`
	LegacyTagList []string           `form:"tag_list" json:"-"`
}

type SearchTag struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

type SearchCategory struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

type SearchAuthor struct {
	ID       ctype.ID `json:"id"`
	Nickname string   `json:"nickname"`
	Avatar   string   `json:"avatar"`
}

type SearchTop struct {
	User  bool `json:"user"`
	Admin bool `json:"admin"`
}

type SearchHighlight struct {
	Title    string `json:"title,omitempty"`
	Abstract string `json:"abstract,omitempty"`
}

type SearchListResponse struct {
	ID               ctype.ID                     `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	Title            string                       `json:"title"`
	Abstract         string                       `json:"abstract,omitempty"`
	Cover            string                       `json:"cover"`
	ViewCount        int                          `json:"view_count"`
	DiggCount        int                          `json:"digg_count"`
	CommentCount     int                          `json:"comment_count"`
	FavorCount       int                          `json:"favor_count"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	PublishStatus    enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status"`
	Tags             []SearchTag                  `json:"tags"`
	Category         *SearchCategory              `json:"category,omitempty"`
	Author           SearchAuthor                 `json:"author"`
	Top              *SearchTop                   `json:"top,omitempty"`
	Highlight        *SearchHighlight             `json:"highlight,omitempty"`
	Score            float64                      `json:"score,omitempty"`
	Content          string                       `json:"-"`
	Part             []markdown.ContentPart       `json:"-"`
	UserTop          bool                         `json:"-"`
	AdminTop         bool                         `json:"-"`
	CategoryTitle    string                       `json:"-"`
	UserNickname     string                       `json:"-"`
	UserAvatar       string                       `json:"-"`
}

type SearchPagination struct {
	Mode       PageMode `json:"mode"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	HasMore    bool     `json:"has_more"`
	Total      *int     `json:"total,omitempty"`
	TotalPages *int     `json:"total_pages,omitempty"`
}

type ArticleSearchResponse struct {
	List       []SearchListResponse `json:"list"`
	Pagination SearchPagination     `json:"pagination"`
}

func (r ArticleSearchRequest) NormalizeType() int8 {
	if r.Type == 0 {
		return 1
	}
	return r.Type
}

func (r ArticleSearchRequest) NormalizeSort() int8 {
	if r.Sort == 0 {
		return 1
	}
	return r.Sort
}

func (r ArticleSearchRequest) NormalizePageMode() PageMode {
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

func (r ArticleSearchRequest) NormalizeAuthorID() ctype.ID {
	if r.AuthorID != 0 {
		return r.AuthorID
	}
	return r.LegacyUserID
}

func (r ArticleSearchRequest) TagTitleList() []string {
	return append([]string(nil), r.LegacyTagList...)
}
