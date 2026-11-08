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

// ArticleDiggRequest 点赞目标态（ADR-0024 决策 3）。
//
// 旧契约没有请求体（toggle）。现在必须显式说"我要它处于已赞还是未赞"：
// `Liked=true` 是确保已赞，`Liked=false` 是确保未赞；重复提交同态即无操作。
//
// 用指针 + `required`：Go 的 `bool` 零值 false 会被 validator 当成"没传"，
// 而"取消点赞"恰恰要传 false，所以必须用 `*bool` 区分"没传"与"传了 false"。
type ArticleDiggRequest struct {
	Liked *bool `json:"liked" binding:"required" example:"true"`
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
	// Sort 为空等价于 created_at：默认按提交时间升序，老投稿不会被新投稿饿死。
	Sort ReviewTaskSort `form:"sort" binding:"omitempty,oneof=created_at risk" enums:"created_at,risk"`
}

// ArticleReviewTaskItem 审核任务项。
//
// content 是文章当前正文（不是提交时的快照）：审核员要在列表里直接判断，
// 不必再跳文章页。文章已被删除时为空串。
//
// risk_level / risk_labels 是机审前两层（词表 + 评分派生标签）的结论，
// 每次读取现算、不落库；没有评分记录时 risk_level 仍可能是 none，不是错误。
// reason_code / reason 是驳回原因的**两个部分**：reason_code 是固定枚举值
// （前端按值渲染文案，绝不按中文匹配），reason 是审核员补的那句自由文本。
// 历史行（#308 之前、撤销）没有枚举，reason_code 为 null，reason 是整段原文——
// 前端因此只剩"枚举 + 补充"与"只有一段话"两种情形要显示。
type ArticleReviewTaskItem struct {
	ID            ctype.ID          `json:"id"`
	ArticleID     ctype.ID          `json:"article_id"`
	AuthorID      ctype.ID          `json:"author_id"`
	ArticleTitle  string            `json:"article_title"`
	AuthorName    string            `json:"author_name"`
	Content       string            `json:"content"`
	PublishStatus ArticleStatus     `json:"publish_status"`
	Stage         ReviewTaskStage   `json:"stage"`
	Source        ReviewTaskSource  `json:"source"`
	Status        ReviewTaskStatus  `json:"status"`
	ReasonCode    *ReviewReasonCode `json:"reason_code"`
	Reason        string            `json:"reason"`
	CreatedAt     time.Time         `json:"created_at"`
	ReviewedAt    *time.Time        `json:"reviewed_at"`
	ReviewedBy    *ctype.ID         `json:"reviewed_by"`
	// RiskLevel 是风险等级：none / low / medium / high。
	RiskLevel ReviewRiskLevel `json:"risk_level" enums:"none,low,medium,high"`
	// RiskLabels 是命中的风险标签（稳定 code，前端映射中文）：
	// block_word / watch_word / low_score / dimension_collapse / length_abnormal。
	RiskLabels []string `json:"risk_labels"`
	// Compliance 是第③层模型合规结论（票 312 / ADR-0033）。
	//
	// nil 表示**还没判定**（AI 关闭、调用失败、任务刚建还没排到），
	// 与 verdict=clean 的"判定过且干净"不是一回事；前端必须能区分这两种形态。
	Compliance *ArticleReviewCompliance `json:"compliance"`
}

// ArticleReviewCompliance 是模型合规结论的展示形状。
//
// categories / reasons 是**稳定 code 与一句话依据**：前者前端映射中文，
// 后者只给管理员看（把模型依据直接给作者等于把判定规则公开）。
type ArticleReviewCompliance struct {
	Verdict    string     `json:"verdict" enums:"clean,suspect,violation"`
	RiskLevel  int        `json:"risk_level"`
	Categories []string   `json:"categories"`
	Reasons    []string   `json:"reasons"`
	PromptVersion string  `json:"prompt_version"`
	ModelName  string     `json:"model_name"`
	CheckedAt  *time.Time `json:"checked_at"`
}

// ArticleReviewReasonItem 是一条可选的驳回原因（出站）。
//
// 值与文案都来自后端唯一词表：前端拿它渲染下拉框，选中的是 value，
// 显示的是 label——两边都不需要认识中文。
type ArticleReviewReasonItem struct {
	Value ReviewReasonCode `json:"value"`
	Label string           `json:"label"`
}

// ArticleReviewTaskListResponse 审核任务列表结果。
type ArticleReviewTaskListResponse struct {
	Count int64                   `json:"count"`
	List  []ArticleReviewTaskItem `json:"list"`
}

// ArticleReviewHandleRequest 处理审核任务。
//
// 驳回必须带 reason_code（固定枚举里的一条）；reason 是可选的一句自由文本。
// 取值校验在用例层（app.reviewReason）：能选哪些值由契约里的 enum 说了算，
// 这里不再手抄一份 oneof 清单。
type ArticleReviewHandleRequest struct {
	Status     ArticleStatus    `json:"status" binding:"required,oneof=3 4" swaggertype:"integer" enums:"3,4"`
	ReasonCode ReviewReasonCode `json:"reason_code"`
	Reason     string           `json:"reason"`
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

// ArticleHotTopOrderRequest 热门置顶的全量顺序替换。
//
// 传的是"完整的有序列表"而不是"把 A 移到 B 前面"：后台只有 12 条，
// 整表替换让服务端不必推断意图，也让并发下的结果可预测（ADR-0017 决策 5）。
type ArticleHotTopOrderRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// ArticleViewHistoryRequest 浏览历史（服务端标题搜索 + 游标分页）。
//
// 没有 page：历史页按天分组（今天 / 昨天 / 近一周 / 更早），offset 分页在
// 删除或重复浏览把某条顶到最前之后会让下一页整体位移，分组因此跨页断开。
//
// 游标是 before_updated_at + before_id 两个参数，**成对出现**（两者都缺省 =
// 第一页）；单个不透明字符串不做，因为契约里已经暴露了 updated_at 与
// article_id，前端续页直接用列表最后一条的这两个字段即可。
type ArticleViewHistoryRequest struct {
	Keyword         string `form:"keyword"`
	BeforeUpdatedAt string `form:"before_updated_at"`
	BeforeID        string `form:"before_id"`
	Limit           int    `form:"limit"`
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
//
// 除了收敛后的状态，还回填**权威计数**（库内快照 + 缓存增量）：
// 前端直接 patch，不再本地 +1、也不再为等数字而重新取整篇正文（ADR-0024 决策 4）。
type ArticleDiggResponse struct {
	IsDigg    bool `json:"is_digg"`
	DiggCount int  `json:"digg_count"`
}

// ArticleFavoriteResponse 收藏/取消收藏结果。
//
// 与点赞同理：写响应回填权威 `favor_count` / `is_favor`（ADR-0024 决策 4/6）。
type ArticleFavoriteResponse struct {
	IsFavor    bool `json:"is_favor"`
	FavorCount int  `json:"favor_count"`
}

// ArticleFavoriteRemoveResponse 批量取消收藏结果。
//
// 为什么与 ArticleFavoriteResponse 分开：批量接口必须让调用方分辨「部分成功」
// （评审 #248 的 P0-2），所以除了权威计数还给出 `removed_count` / `failed_count`；
// 如果沿用单条的形状，部分失败就只能靠猜。
//
// `is_favor` / `favor_count` 仍按「最后一条成功移除的文章」给出：当前唯一消费方
// （收藏夹弹窗）每次只传当前这一篇；多篇批量场景不应依赖这两个字段，只看计数。
// 全部失败不会走到这里（handler 返回非 0 业务码）。
type ArticleFavoriteRemoveResponse struct {
	IsFavor      bool `json:"is_favor"`
	FavorCount   int  `json:"favor_count"`
	RemovedCount int  `json:"removed_count"`
	FailedCount  int  `json:"failed_count"`
}

// ArticleHotTopCardResponse 是首页「热门置顶」运营位的一张卡片。
//
// 字段与 ArticleTopCardResponse（/articles/top，作者置顶）保持一致形状：
// 两个区块的版式相同，前端可以复用同一套卡片渲染。
type ArticleHotTopCardResponse struct {
	ID             ctype.ID `json:"id"`
	Title          string   `json:"title"`
	Abstract       string   `json:"abstract"`
	Cover          string   `json:"cover"`
	ViewCount      int      `json:"view_count"`
	AuthorNickname string   `json:"author_nickname"`
	CategoryTitle  string   `json:"category_title"`
}

// ArticleHotTopAdminItem 是后台「热门置顶」页的一行。
//
// 与公开卡片分开：后台要看见**已失效**的行（文章被隐藏/退回草稿/删除），
// 否则没法排序也没法清理。
//
// 这里刻意**不**返回 publish_status / visibility_status：Go 侧它们是可空指针，
// 而 swag v2 会把指向具名枚举的指针印成不可空的 $ref —— 契约会撒谎
// （详见 GLOSSARY「可空性三态」）。「为什么失效」由 stale 一字段回答，
// 文章自身的状态在文章管理列表里已经有了。
type ArticleHotTopAdminItem struct {
	ArticleID ctype.ID `json:"article_id"`
	SortOrder int      `json:"sort_order"`
	Stale     bool     `json:"stale"`
	Title     string   `json:"title"`
	Cover     string   `json:"cover"`
}
