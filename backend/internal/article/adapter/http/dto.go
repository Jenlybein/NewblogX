// Package http 是 article 模块的 HTTP 适配层。
//
// 职责：解析请求 → 转成 app 的命令 → 调用例 → 组装响应。
// 所有业务判断都在 app/domain，这里只做协议转换与鉴权上下文提取。
//
// DTO 字段与改造前的 api/article_api 严格一致：前端零改动，
// 由 golden 逐字节比对保证（见 S3.10）。
package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
)

// ArticleCreateRequest 创建文章。
type ArticleCreateRequest struct {
	Title            string            `json:"title" binding:"required"`
	Abstract         string            `json:"abstract"`
	Content          string            `json:"content" binding:"required"`
	CategoryID       *ctype.ID         `json:"category_id"`
	TagIDs           []ctype.ID        `json:"tag_ids"`
	CoverImageID     *ctype.ID         `json:"cover_image_id"`
	CommentsToggle   bool              `json:"comments_toggle"`
	Status           ArticleStatus     `json:"status" binding:"required,oneof=1 2" swaggertype:"integer" enums:"1,2"`
	VisibilityStatus ArticleVisibility `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden" swaggertype:"string" enums:"visible,user_hidden"`
}

// ArticleUpdateRequest 更新文章；nil 表示不修改。
type ArticleUpdateRequest struct {
	Title            *string            `json:"title"`
	Abstract         *string            `json:"abstract"`
	Content          *string            `json:"content"`
	CategoryID       *ctype.ID          `json:"category_id"`
	TagIDs           *[]ctype.ID        `json:"tag_ids"`
	CoverImageID     *ctype.ID          `json:"cover_image_id"`
	CommentsToggle   *bool              `json:"comments_toggle"`
	Status           *ArticleStatus     `json:"status" binding:"omitempty,oneof=1 2" swaggertype:"integer" enums:"1,2"`
	VisibilityStatus *ArticleVisibility `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden" swaggertype:"string" enums:"visible,user_hidden"`
}

// ArticleCreateResponse 创建结果。
type ArticleCreateResponse struct {
	ID               ctype.ID          `json:"id"`
	Title            string            `json:"title"`
	CategoryID       *ctype.ID         `json:"category_id"`
	TagIDs           []ctype.ID        `json:"tag_ids"`
	CoverImageID     *ctype.ID         `json:"cover_image_id,omitempty"`
	CommentsToggle   bool              `json:"comments_toggle"`
	Status           ArticleStatus     `json:"status"`
	PublishStatus    ArticleStatus     `json:"publish_status"`
	VisibilityStatus ArticleVisibility `json:"visibility_status"`
}

// ArticleDetailResponse 详情。字段顺序与改造前一致，便于 diff。
type ArticleDetailResponse struct {
	ID                  ctype.ID          `json:"id"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	Title               string            `json:"title"`
	Abstract            string            `json:"abstract"`
	Content             string            `json:"content"`
	CategoryID          *ctype.ID         `json:"category_id"`
	TagIDs              []ctype.ID        `json:"tag_ids"`
	CoverImageID        *ctype.ID         `json:"cover_image_id,omitempty"`
	Cover               string            `json:"cover"`
	ViewCount           int               `json:"view_count"`
	DiggCount           int               `json:"digg_count"`
	CommentCount        int               `json:"comment_count"`
	FavorCount          int               `json:"favor_count"`
	CommentsToggle      bool              `json:"comments_toggle"`
	Status              ArticleStatus     `json:"status"`
	PublishStatus       ArticleStatus     `json:"publish_status"`
	VisibilityStatus    ArticleVisibility `json:"visibility_status"`
	Tags                []string          `json:"tags"`
	AuthorID            ctype.ID          `json:"author_id"`
	AuthorAvatarImageID *ctype.ID         `json:"author_avatar_image_id,omitempty"`
	AuthorAvatar        string            `json:"author_avatar"`
	AuthorAbstract      string            `json:"author_abstract"`
	AuthorCreatedAt     time.Time         `json:"author_created_time"`
	AuthorNickname      string            `json:"author_name"`
	AuthorUsername      string            `json:"author_username"`
	CategoryName        string            `json:"category_name"`
	IsDigg              bool              `json:"is_digg"`
	IsFavor             bool              `json:"is_favor"`
}

// ArticleFavoriteRequest 收藏。
type ArticleFavoriteRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
	FavorID   ctype.ID `json:"favor_id"`
}

// ArticleReviewTaskListRequest 审核任务列表。
type ArticleReviewTaskListRequest struct {
	Page   int              `form:"page"`
	Limit  int              `form:"limit"`
	Status ReviewTaskStatus `form:"status" binding:"omitempty,oneof=pending approved rejected canceled" enums:"pending,approved,rejected,canceled"`
}

// ArticleReviewTaskItem 审核任务项。
type ArticleReviewTaskItem struct {
	ID            ctype.ID         `json:"id"`
	ArticleID     ctype.ID         `json:"article_id"`
	AuthorID      ctype.ID         `json:"author_id"`
	ArticleTitle  string           `json:"article_title"`
	AuthorName    string           `json:"author_name"`
	PublishStatus ArticleStatus    `json:"publish_status"`
	Stage         ReviewTaskStage  `json:"stage"`
	Source        ReviewTaskSource `json:"source"`
	Status        ReviewTaskStatus `json:"status"`
	Reason        string           `json:"reason"`
	CreatedAt     time.Time        `json:"created_at"`
	ReviewedAt    *time.Time       `json:"reviewed_at"`
	ReviewedBy    *ctype.ID        `json:"reviewed_by"`
}

// ArticleReviewTaskListResponse 审核任务列表结果。
type ArticleReviewTaskListResponse struct {
	Count int64                   `json:"count"`
	List  []ArticleReviewTaskItem `json:"list"`
}

// ArticleReviewHandleRequest 处理审核任务。
type ArticleReviewHandleRequest struct {
	Status ArticleStatus `json:"status" binding:"required,oneof=3 4" swaggertype:"integer" enums:"3,4"`
	Reason string        `json:"reason"`
}

// ArticleAdminVisibilityURI 管理员改可见性。
type ArticleAdminVisibilityURI struct {
	ID         ctype.ID `uri:"id" binding:"required"`
	Visibility string   `uri:"visibility" binding:"required,oneof=hide show" enums:"hide,show"`
}

// ArticleViewCountRequest 浏览上报。
type ArticleViewCountRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
}

// ArticleAuthorInfoBindRequest 作者信息查询。
type ArticleAuthorInfoBindRequest struct {
	AuthorID ctype.ID `form:"author_id" binding:"required"`
}

// CategoryRequest 分类创建/更新。
type CategoryRequest struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title" binding:"required"`
}

// CategoryListRequest 分类列表。
type CategoryListRequest struct {
	UserID ctype.ID `form:"user_id"`
}

// CategoryOptionsRequest 分类下拉。
type CategoryOptionsRequest struct {
	UserID ctype.ID `form:"user_id"`
}

// FavoriteRequest 收藏夹创建/更新。
type FavoriteRequest struct {
	ID       ctype.ID `json:"id"`
	Title    string   `json:"title" binding:"required"`
	Cover    string   `json:"cover"`
	Abstract string   `json:"abstract"`
}

// FavoriteListRequest 收藏夹列表。
type FavoriteListRequest struct {
	UserID ctype.ID `form:"user_id"`
}

// FavoriteArticlesRequest 收藏夹内文章。
type FavoriteArticlesRequest struct {
	// 字段名沿用旧契约 favorite_id（不是 favor_id），前端零改动。
	FavorID ctype.ID `form:"favorite_id" binding:"required"`
	Page    int      `form:"page"`
	Limit   int      `form:"limit"`
}

// FavoriteRemovePatchModel 批量移除收藏。
type FavoriteRemovePatchModel struct {
	// 字段名沿用旧契约 articles / favorite_id。
	ArticleIDs []ctype.ID `json:"articles" binding:"required"`
	FavorID    ctype.ID   `json:"favorite_id"`
}

// TagRequest 标签创建/更新（管理员）。
type TagRequest struct {
	ID          ctype.ID `json:"id"`
	Title       string   `json:"title" binding:"required"`
	Sort        int      `json:"sort"`
	Description string   `json:"description"`
	IsEnabled   *bool    `json:"is_enabled"`
}

// TagListRequest 标签列表。
type TagListRequest struct {
	IncludeDisabled bool `form:"include_disabled"`
}

// ArticleTopSetRequest 置顶设置。
type ArticleTopSetRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// ArticleTopListRequest 置顶列表。
type ArticleTopListRequest struct {
	UserID ctype.ID `form:"user_id"`
}

// ArticleViewHistoryRequest 浏览历史。
type ArticleViewHistoryRequest struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

// ── 列表类响应 DTO ──
//
// 这些接口的 domain 实体是私有字段，不能直接序列化；字段与旧契约一致。

// CategoryListItem 分类列表项。
type CategoryListItem struct {
	ID           ctype.ID   `json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
	Title        string     `json:"title"`
	UserID       ctype.ID   `json:"user_id"`
	ArticleCount int        `json:"article_count"`
	Nickname     string     `json:"nickname,omitempty"`
	Avatar       string     `json:"avatar,omitempty"`
}

// TagListItem 标签列表项。
type TagListItem struct {
	ID           ctype.ID   `json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
	Title        string     `json:"title"`
	Sort         int        `json:"sort"`
	Description  string     `json:"description"`
	ArticleCount int        `json:"article_count"`
	IsEnabled    bool       `json:"is_enabled"`
	CreatedBy    ctype.ID   `json:"created_by"`
}

// FavoriteListItem 收藏夹列表项。
type FavoriteListItem struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ID           ctype.ID  `json:"id"`
	UserID       ctype.ID  `json:"user_id"`
	Title        string    `json:"title"`
	Cover        string    `json:"cover"`
	Abstract     string    `json:"abstract"`
	IsDefault    bool      `json:"is_default"`
	ArticleCount int       `json:"article_count"`
	Nickname     string    `json:"nickname,omitempty"`
	Avatar       string    `json:"avatar,omitempty"`
	HasArticle   bool      `json:"has_article"`
}

// FavoriteArticleListItem 收藏内容项。
type FavoriteArticleListItem struct {
	FavoritedAt   time.Time     `json:"favorited_at"`
	ArticleID     ctype.ID      `json:"article_id"`
	Title         string        `json:"title"`
	Abstract      string        `json:"abstract"`
	Cover         string        `json:"cover"`
	ViewCount     int           `json:"view_count"`
	DiggCount     int           `json:"digg_count"`
	CommentCount  int           `json:"comment_count"`
	FavorCount    int           `json:"favor_count"`
	UserNickname  string        `json:"user_nickname"`
	UserAvatar    string        `json:"user_avatar"`
	ArticleStatus ArticleStatus `json:"article_status"`
}

// ArticleViewHistoryItem 浏览历史项。
type ArticleViewHistoryItem struct {
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
	Cover     string    `json:"cover"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	UserID    ctype.ID  `json:"user_id"`
	ArticleID ctype.ID  `json:"article_id"`
}

// ── 以下两个响应类型是 P1 补上的：之前注解手写 object{...}，与真实返回不符 ──

// ArticleAuthorInfoResponse 作者展示信息。
//
// 注意：前端 `api.ts` 里的 `ArticleAuthorInfo` 期望的是
// `author_id / article_count / article_visited_count / fans_count`，
// 与这里的真实返回（id/username/nickname/avatar）**字段名和内容都不一致**，
// 是一个既有缺陷（页面上"文章数/访问量/粉丝数"因此恒为 0）。
// 这里只如实反映后端返回，不擅自改业务行为，待决策。
type ArticleAuthorInfoResponse struct {
	ID       ctype.ID `json:"id"`
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Avatar   string   `json:"avatar"`

	// 作者卡展示的三项累计值。之前注解写的是空 data、实际也只返回上面四个字段，
	// 而前端在渲染「文章数 / 访问量 / 粉丝数」，于是这三处恒显示 0。
	ArticleCount        int `json:"article_count"`
	ArticleVisitedCount int `json:"article_visited_count"`
	FansCount           int `json:"fans_count"`
}

// ArticleTopCardResponse 是「热门置顶」区块的一张卡片。
//
// 之前 `/articles/top` 只返回文章 ID 数组，前端却按对象渲染
// （article.id / article.cover / article.title），于是那块区域整块是空的、
// 链接指向 /article/undefined。这里补上渲染真正需要的字段。
type ArticleTopCardResponse struct {
	ID             ctype.ID `json:"id"`
	Title          string   `json:"title"`
	Abstract       string   `json:"abstract"`
	Cover          string   `json:"cover"`
	ViewCount      int      `json:"view_count"`
	AuthorNickname string   `json:"author_nickname"`
	CategoryTitle  string   `json:"category_title"`
}

// ArticleDiggResponse 点赞结果。
type ArticleDiggResponse struct {
	IsDigg bool `json:"is_digg"`
}
