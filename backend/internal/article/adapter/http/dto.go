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

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// ArticleCreateRequest 创建文章。
type ArticleCreateRequest struct {
	Title            string                       `json:"title" binding:"required"`
	Abstract         string                       `json:"abstract"`
	Content          string                       `json:"content" binding:"required"`
	CategoryID       *ctype.ID                    `json:"category_id"`
	TagIDs           []ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                    `json:"cover_image_id"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	Status           enum.ArticleStatus           `json:"status" binding:"required,oneof=1 2"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden"`
}

// ArticleUpdateRequest 更新文章；nil 表示不修改。
type ArticleUpdateRequest struct {
	Title            *string                       `json:"title"`
	Abstract         *string                       `json:"abstract"`
	Content          *string                       `json:"content"`
	CategoryID       *ctype.ID                     `json:"category_id"`
	TagIDs           *[]ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                     `json:"cover_image_id"`
	CommentsToggle   *bool                         `json:"comments_toggle"`
	Status           *enum.ArticleStatus           `json:"status" binding:"omitempty,oneof=1 2"`
	VisibilityStatus *enum.ArticleVisibilityStatus `json:"visibility_status" binding:"omitempty,oneof=visible user_hidden"`
}

// ArticleCreateResponse 创建结果。
type ArticleCreateResponse struct {
	ID               ctype.ID                     `json:"id"`
	Title            string                       `json:"title"`
	CategoryID       *ctype.ID                    `json:"category_id"`
	TagIDs           []ctype.ID                   `json:"tag_ids"`
	CoverImageID     *ctype.ID                    `json:"cover_image_id,omitempty"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	Status           enum.ArticleStatus           `json:"status"`
	PublishStatus    enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status"`
}

// ArticleDetailResponse 详情。字段顺序与改造前一致，便于 diff。
type ArticleDetailResponse struct {
	ID                  ctype.ID                     `json:"id"`
	CreatedAt           time.Time                    `json:"created_at"`
	UpdatedAt           time.Time                    `json:"updated_at"`
	Title               string                       `json:"title"`
	Abstract            string                       `json:"abstract"`
	Content             string                       `json:"content"`
	CategoryID          *ctype.ID                    `json:"category_id"`
	TagIDs              []ctype.ID                   `json:"tag_ids"`
	CoverImageID        *ctype.ID                    `json:"cover_image_id,omitempty"`
	Cover               string                       `json:"cover"`
	ViewCount           int                          `json:"view_count"`
	DiggCount           int                          `json:"digg_count"`
	CommentCount        int                          `json:"comment_count"`
	FavorCount          int                          `json:"favor_count"`
	CommentsToggle      bool                         `json:"comments_toggle"`
	Status              enum.ArticleStatus           `json:"status"`
	PublishStatus       enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus    enum.ArticleVisibilityStatus `json:"visibility_status"`
	Tags                []string                     `json:"tags"`
	AuthorID            ctype.ID                     `json:"author_id"`
	AuthorAvatarImageID *ctype.ID                    `json:"author_avatar_image_id,omitempty"`
	AuthorAvatar        string                       `json:"author_avatar"`
	AuthorAbstract      string                       `json:"author_abstract"`
	AuthorCreatedAt     time.Time                    `json:"author_created_time"`
	AuthorNickname      string                       `json:"author_name"`
	AuthorUsername      string                       `json:"author_username"`
	CategoryName        string                       `json:"category_name"`
	IsDigg              bool                         `json:"is_digg"`
	IsFavor             bool                         `json:"is_favor"`
}

// ArticleFavoriteRequest 收藏。
type ArticleFavoriteRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
	FavorID   ctype.ID `json:"favor_id"`
}

// ArticleReviewTaskListRequest 审核任务列表。
type ArticleReviewTaskListRequest struct {
	Page   int                            `form:"page"`
	Limit  int                            `form:"limit"`
	Status models.ArticleReviewTaskStatus `form:"status" binding:"omitempty,oneof=pending approved rejected canceled"`
}

// ArticleReviewTaskItem 审核任务项。
type ArticleReviewTaskItem struct {
	ID            ctype.ID                       `json:"id"`
	ArticleID     ctype.ID                       `json:"article_id"`
	AuthorID      ctype.ID                       `json:"author_id"`
	ArticleTitle  string                         `json:"article_title"`
	AuthorName    string                         `json:"author_name"`
	PublishStatus enum.ArticleStatus             `json:"publish_status"`
	Stage         models.ArticleReviewTaskStage  `json:"stage"`
	Source        models.ArticleReviewTaskSource `json:"source"`
	Status        models.ArticleReviewTaskStatus `json:"status"`
	Reason        string                         `json:"reason"`
	CreatedAt     time.Time                      `json:"created_at"`
	ReviewedAt    *time.Time                     `json:"reviewed_at"`
	ReviewedBy    *ctype.ID                      `json:"reviewed_by"`
}

// ArticleReviewTaskListResponse 审核任务列表结果。
type ArticleReviewTaskListResponse struct {
	Count int64                   `json:"count"`
	List  []ArticleReviewTaskItem `json:"list"`
}

// ArticleReviewHandleRequest 处理审核任务。
type ArticleReviewHandleRequest struct {
	Status enum.ArticleStatus `json:"status" binding:"required,oneof=3 4"`
	Reason string             `json:"reason"`
}

// ArticleAdminVisibilityURI 管理员改可见性。
type ArticleAdminVisibilityURI struct {
	ID         ctype.ID `uri:"id" binding:"required"`
	Visibility string   `uri:"visibility" binding:"required,oneof=hide show"`
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
	FavoritedAt   time.Time          `json:"favorited_at"`
	ArticleID     ctype.ID           `json:"article_id"`
	Title         string             `json:"title"`
	Abstract      string             `json:"abstract"`
	Cover         string             `json:"cover"`
	ViewCount     int                `json:"view_count"`
	DiggCount     int                `json:"digg_count"`
	CommentCount  int                `json:"comment_count"`
	FavorCount    int                `json:"favor_count"`
	UserNickname  string             `json:"user_nickname"`
	UserAvatar    string             `json:"user_avatar"`
	ArticleStatus enum.ArticleStatus `json:"article_status"`
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
