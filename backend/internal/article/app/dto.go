package app

import (
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/viewhistory"
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
	// QualityGate 是「发布质量闸门」的生效配置快照（票 306 / ADR-0032）。
	//
	// 零值 = 闸门关闭，提交路径与引入闸门之前完全一致。由 adapter 每请求
	// 从运行时配置现读，收紧 / 放宽在下一次提交即生效，不需要重启。
	QualityGate QualityGatePolicy
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
	// QualityGate 是「发布质量闸门」的生效配置快照（票 306 / ADR-0032）。
	// 零值 = 闸门关闭，更新路径与引入闸门之前完全一致。
	QualityGate QualityGatePolicy
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
	// FolderID 为 0 时落到默认收藏夹；收藏与取消收藏共用这一口径。
	// 取消收藏时它同时是**目标范围**：只删这个夹子里的关系，不留「任意夹」的兜底。
	FolderID favorite.ID
}

// FavoriteResult 是收藏/取消收藏后的权威状态与计数（ADR-0024 决策 4/6）。
//
// 前端拿到它就直接 patch 界面、主动失效客户端缓存，不再为一次收藏重取整篇正文。
// 计数是**文章级**绝对值（库内快照 + 缓存增量），不是"这次 +1 / -1"。
type FavoriteResult struct {
	// IsFavor 操作后该用户是否仍收藏着这篇文章（多收藏夹时移除其中一个仍为 true）。
	IsFavor bool
	// FavorCount 操作后文章的收藏总数。
	FavorCount int
}

// DiggCmd 是设置点赞目标态的输入（ADR-0024 决策 3）。
//
// 与旧 toggle 的区别：调用方说的是"我要它处于已赞 / 未赞"，服务端按目标收敛。
// 重复提交同一个目标态就是无操作 —— 这消除了"按钮显示未赞、实际已赞，再点一下
// 把真实点赞取消掉"的数据损坏风险。
type DiggCmd struct {
	Actor     domain.Viewer
	ArticleID domain.ArticleID
	// Liked 是目标态：true 表示点赞，false 表示取消。
	Liked bool
}

// DiggResult 是设置点赞后的权威状态与计数。
type DiggResult struct {
	// IsDigg 收敛后的点赞状态（幂等重复时也是 true/false，不是"这次变了没有"）。
	IsDigg bool
	// DiggCount 操作后文章的点赞总数（库内快照 + 缓存增量）。
	DiggCount int
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
	// Content 是**文章当前正文**，不是提交那一刻的快照。
	//
	// 审核任务表没有正文列（零 DDL 硬约束），提交时只快照了标题与作者，
	// 所以这里取 article_models 里的当前内容。这正是审核员要判断的东西，
	// 也比"再存一份快照"更不容易让人读到旧稿。
	// 文章已被删除时留空，任务条目仍然出现（见 ListReviewTasks）。
	Content string
}

// ── 合集 ──

// CreateCollectionCmd 是建合集的输入。
//
// 封面传空串表示"退回首篇文章的封面"（由读模型处理），不是错误。
type CreateCollectionCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// Title 标题，非空且不超过 collection.MaxTitleLen。
	Title string
	// Description 描述，可空。
	Description string
	// Cover 封面 URL，可空。
	Cover string
	// Displayed 是否一开始就展示给读者。
	Displayed bool
}

// UpdateCollectionCmd 是改合集内容的输入。
//
// 只有标题 / 描述 / 封面三个字段：展示开关走 SetCollectionDisplayCmd（它有自己的
// 语义与调用点），顺序不在这里改。把三者混进一个"整体更新"会让后台改一次标题
// 就顺手覆盖掉别人刚切换的展示开关。
type UpdateCollectionCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// CollectionID 目标合集。
	CollectionID collection.ID
	// Title 标题，非空且不超过 collection.MaxTitleLen。
	Title string
	// Description 描述，可空。
	Description string
	// Cover 封面 URL，传空串表示回退到首篇文章的封面。
	Cover string
}

// DeleteCollectionCmd 是删合集的输入。
//
// 软删：合集查不到，但目录树里的节点与文章本身都保留。
type DeleteCollectionCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// CollectionID 目标合集。
	CollectionID collection.ID
}

// SetCollectionDisplayCmd 是切换合集展示开关的输入。
type SetCollectionDisplayCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// CollectionID 目标合集。
	CollectionID collection.ID
	// Displayed 目标展示状态。
	Displayed bool
}

// ── 项目 ──
//
// 命令结构与合集**逐字对应**：两套运营位的心智复用，但各自的集合 ID 类型
// 不同（collection.ID / project.ID），所以命令不能互相顶替。

// CreateProjectCmd 是建项目的输入。
//
// 封面传空串表示"退回首篇公开文章的封面"（由读模型处理），不是错误。
type CreateProjectCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// Title 标题，非空且不超过 project.MaxTitleLen。
	Title string
	// Description 描述，可空。
	Description string
	// Cover 封面 URL，可空。
	Cover string
	// Displayed 是否一开始就展示给读者。
	Displayed bool
}

// UpdateProjectCmd 是改项目内容的输入。
//
// 只有标题 / 描述 / 封面三个字段：展示开关走 SetProjectDisplayCmd，顺序走
// 项目间排序。把三者混进一个"整体更新"会让后台改一次标题就顺手覆盖掉
// 别人刚切换的展示开关。
type UpdateProjectCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// ProjectID 目标项目。
	ProjectID project.ID
	// Title 标题，非空且不超过 project.MaxTitleLen。
	Title string
	// Description 描述，可空。
	Description string
	// Cover 封面 URL，传空串表示回退到首篇文章的封面。
	Cover string
}

// DeleteProjectCmd 是删项目的输入。软删：项目查不到，但目录树里的节点都保留。
type DeleteProjectCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// ProjectID 目标项目。
	ProjectID project.ID
}

// SetProjectDisplayCmd 是切换项目展示开关的输入。
type SetProjectDisplayCmd struct {
	// Actor 是操作者，必须是管理员。
	Actor domain.Viewer
	// ProjectID 目标项目。
	ProjectID project.ID
	// Displayed 目标展示状态。
	Displayed bool
}

// ── 浏览历史 ──

// ViewHistoryQuery 是浏览历史列表的查询条件（用户由 viewer 决定，不来自请求）。
//
// 与 domain.Page 不同，这里没有 offset：历史页按天分组（今天 / 昨天 / 近一周 /
// 更早），offset 在删除或置顶之后会让下一页整体位移，分组就会跨页断开。
type ViewHistoryQuery struct {
	// Keyword 按文章标题模糊匹配；空白视为不筛选。
	Keyword string
	// Before 为 nil 表示第一页；非 nil 表示续页位置。
	Before *viewhistory.Cursor
	// Limit 期望条数（零值=默认 10，上限 100，见 viewhistory.ListQuery.Normalize）。
	Limit int
}

// ViewHistoryPage 是一页浏览历史。
//
// HasMore 由"多取一条"得出，而不是 total：游标分页不需要总数，历史页也不显示它。
type ViewHistoryPage struct {
	Records []*viewhistory.Record
	HasMore bool
}
