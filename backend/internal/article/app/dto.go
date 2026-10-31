package app

import (
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/review"
)

// CreateArticleCmd 是创建文章的输入。
//
// Content / Abstract / Cover 都要求**已经处理过**：markdown 安全渲染、
// 摘要截取、图片 ID 转 URL 都在 adapter 完成。app 只做业务编排。
type CreateArticleCmd struct {
	// Actor 是操作者，必须已登录。
	Actor domain.Viewer
	// Title 标题。
	Title string
	// Abstract 摘要；为空时由 adapter 依据正文生成后传入。
	Abstract string
	// Content 已渲染的正文。
	Content string
	// CategoryID 可为 nil。
	CategoryID *domain.CategoryID
	// TagIDs 标签集合。
	TagIDs []domain.TagID
	// Cover 已解析的封面 URL。
	Cover string
	// CommentsToggle 是否允许评论。
	CommentsToggle bool
	// TargetStatus 是作者请求的状态：草稿或提交审核。
	TargetStatus domain.Status
	// Visibility 可见性。
	Visibility domain.Visibility
	// SkipExamining 来自站点运行时配置：跳过审核时提交即发布。
	SkipExamining bool
	// Policy 是站点「发布与展示」两个写作开关的取值快照。
	//
	// 由 adapter 从运行时配置的现读闭包取值，判定在 app/domain 层完成
	// （ADR-0017 当前生效约束 2）。零值 = 两项都关，所以调用方必须显式给出。
	Policy domain.WritePolicy
}

// UpdateArticleCmd 是更新文章的输入；nil 表示该字段不修改。
type UpdateArticleCmd struct {
	Actor      domain.Viewer
	ArticleID  domain.ArticleID
	Title      *string
	Abstract   *string
	Content    *string
	CategoryID *domain.CategoryID
	// CategorySet 为真表示要写 classification（含清空）。
	CategorySet    bool
	Cover          *string
	CommentsToggle *bool
	Visibility     *domain.Visibility
	// TagIDs 非 nil 表示替换标签集合。
	TagIDs *[]domain.TagID
	// TargetStatus 非 nil 时表示作者显式要求的状态（草稿/提交审核）。
	TargetStatus *domain.Status
	// SkipExamining 站点配置。
	SkipExamining bool
	// Policy 是站点「发布与展示」两个写作开关的取值快照。
	//
	// 语义注意：这里同时决定"能不能更新这篇文章"——关闭发布开关时，
	// 更新已发布（或审核中）的文章会被拒；关闭草稿开关时，保存草稿会被拒。
	Policy domain.WritePolicy
}

// UpdateResult 是更新结果。
type UpdateResult struct {
	// Article 变更后的聚合。
	Article *domain.Article
	// Changed 是否有任何字段变化（false 表示空操作）。
	Changed bool
	// Resubmitted 是否因编辑而重新进入审核。
	Resubmitted bool
	// TagsReplaced 标签是否被替换（adapter 需要同步 ES 投影）。
	TagsReplaced bool
}

// ArticleView 是聚合的只读快照，供 adapter 构造响应。
//
// 不用 domain.Article 直接出参：HTTP 需要的字段（作者昵称、分类名、
// 是否已点赞）不属于聚合，混进去会让 domain 变胖。
type ArticleView struct {
	Article domain.Article
	// Author 是作者展示信息，由 adapter 补。
	Author AuthorView
	// CategoryName 分类名，由 adapter 补。
	CategoryName string
	// Tags 标签展示信息，按 sort desc, id asc 排序。
	Tags []TagView
	// Digg/Favor 当前用户是否已点赞/收藏。
	Digg  bool
	Favor bool
}

// AuthorView 是作者展示信息。
type AuthorView struct {
	ID        domain.UserID
	Username  string
	Nickname  string
	Avatar    string
	Abstract  string
	CreatedAt time.Time

	// Admin 表示作者是不是管理员（来自 user_models.role）。
	//
	// 读面过滤要用它：「显示非管理员发布文章」开关关闭时，非管理员作者的文章
	// 在详情页按 404 处理（ADR-0017 决策 4）。放在作者视图里而不是让 handler
	// 自己 join，是为了让 article 模块仍然只通过 UserPort 认识用户模块。
	Admin bool

	// 以下三项来自 user_stat_models。文章页右侧的作者卡要显示
	// 「文章数 / 访问量 / 粉丝数」，之前没有这三个字段，前端拿到的恒为 undefined
	// （模板里有 `|| 0` 兜底，所以不报错、只是永远显示 0）。
	ArticleCount        int
	ArticleVisitedCount int
	FansCount           int
}

// TagView 是标签展示信息。
type TagView struct {
	ID    domain.TagID
	Title string
}

// ExamineCmd 是审核动作的输入。
type ExamineCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
	// Approve 为真表示通过，否则拒绝。
	Approve bool
	// Reason 拒绝原因。
	Reason string
}

// FavoriteCmd 是收藏动作的输入。
type FavoriteCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
	// FolderID 为 0 时落到默认收藏夹。
	FolderID favorite.ID
}

// TopCmd 是置顶/取消置顶的输入。
type TopCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
	// Remove 为真表示取消置顶。
	Remove bool
}

// HotTopCmd 是设置/取消「热门置顶」的输入（管理员运营位）。
//
// 与 TopCmd 分开而不是复用一个：作者置顶与热门置顶的权限、上限、
// 副作用都不同，共用一个命令结构只会让调用点看不出自己在改哪一套数据。
type HotTopCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
	// Remove 为真表示取消置顶（幂等，且不要求文章仍然存在）。
	Remove bool
}

// DeleteCmd 是删除文章的输入。
//
// 只支持软删：旧实现里 `DeleteArticles(tx, list, unscoped)` 的 unscoped 参数
// 在生产代码两处调用都是 false，硬删仅在测试里用过。为一个没人用的分支
// 增加端口面不划算，M4 明确不做硬删（真要清库走运维脚本）。
type DeleteCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
}

// DeleteResult 是删除结果，记录被清理的关联数据量，便于审计与测试断言。
type DeleteResult struct {
	ArticleID domain.ArticleID
	// Comments 被删除的评论数（旧实现由 comment 模块处理）。
	Comments int
	// Diggs / Favors / Tops / Histories / ArticleTags 各自清理数量。
	Diggs       int
	Favors      int
	Tops        int
	Histories   int
	ArticleTags int
}

// ReviewTaskView 是审核任务的只读视图。
type ReviewTaskView struct {
	Task review.Task
}
