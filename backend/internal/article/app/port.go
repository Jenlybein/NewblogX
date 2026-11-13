// Package app 是 article 模块的用例层。
//
// 职责边界：
//   - 事务边界在这里（通过 TxManager），一个用例 = 一次事务；
//   - 只依赖 domain 的端口与自己定义的 Repos 聚合，不认识 gorm、gin；
//   - 时间与 ID 都从外部注入，测试可用 FakeClock / SeqID 做到完全确定。
package app

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/hottop"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/risk"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
	"myblogx/internal/kernel/ctype"
)

// Repos 是"一次事务内可用的全部仓储"。
//
// 关键设计：app 拿到的是 Repos 而不是数据库句柄。
// 类型上就写不出 SQL，也就不会把 handler 里"随手查库"的老毛病带进来。
type Repos interface {
	Articles() domain.Repository
	Categories() taxonomy.CategoryRepository
	Tags() taxonomy.TagRepository
	Favorites() favorite.Repository
	Tops() top.Repository
	// HotTops 是站点运营位「热门置顶」（表 article_hot_top_models）。
	// 与 Tops（作者置顶）是两套数据，不要互相顶替。
	HotTops() hottop.Repository
	ViewHistory() viewhistory.Repository
	Reviews() review.Repository
	// AuthorStats 是唯一的跨模块写：删文章/浏览时更新作者统计。
	AuthorStats() AuthorStatsPort
	// Outbox 是领域事件的出口（当前仅用于记录与后续订阅）。
	Outbox() OutboxPort
	// Counters 是浏览/点赞/收藏/评论的增量计数（生产里落在 Redis）。
	Counters() CounterPort
	// Comments 是评论模块的跨模块能力（删文章级联、详情页评论数）。
	Comments() CommentPort
	// Users 是用户模块的只读能力（作者展示信息）。
	Users() UserPort
	// Diggs 是点赞关系（详情页"是否已赞"、级联删除）。
	Diggs() DiggPort
	// Collections 是合集（管理员编排的有序文章集合）。
	// 与 HotTops（热门置顶运营位）是两套数据：合集有标题/封面/展示开关，且无条数上限。
	Collections() collection.Repository
	// Projects 是项目（与合集并列的独立内容实体）。
	// 与 Collections 是两套数据、两套表：项目有自己的后台维护页，不共用合集的行。
	Projects() project.Repository
	// 目录树不在 Repos 上：两个实体各有一张具名树表，读树前必须先用
	// NamedOutlineRepository 把一次事务的仓储指向对应归属（见 named_outline.go）。

	// Risks 是合规判定记录（票 312 / ADR-0024）。
	//
	// 它是**新表**：结论写在这里，而不是给文章表或审核任务表加列（零 DDL）。
	Risks() risk.Repository
}

// DiggPort 是点赞关系端口。
//
// 计数增量走 CounterPort，关系本身（谁赞了哪篇）走这里——两者职责不同：
// 前者在 Redis 里是易失的，后者必须落库才能防重复。
type DiggPort interface {
	// Exists 判断某用户是否已点赞。
	Exists(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// Add 新增点赞关系；已存在返回 false 表示无变化。
	Add(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// Remove 取消点赞；本不存在返回 false。
	Remove(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (bool, error)
	// DeleteByArticle 删除某文章的全部点赞，返回条数。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
}

// CounterPort 是文章计数增量端口。
//
// 生产实现转发到现有 Redis 计数（redis_article）；测试用内存 fake。
// 设计成"只加增量、不做绝对值"：并发点赞不能靠读-改-写。
type CounterPort interface {
	// Add 累加计数增量。
	Add(ctx context.Context, id domain.ArticleID, delta domain.Counters) error
	// Load 批量读取增量；缺失的文章返回零值。
	Load(ctx context.Context, ids []domain.ArticleID) (map[domain.ArticleID]domain.Counters, error)
}

// CommentPort 是评论模块能力。
//
// 只暴露 article 需要的两个动作：级联删除与计数。
// 不把评论实体引进来，避免两个聚合互相依赖。
type CommentPort interface {
	// DeleteByArticle 删除某文章的全部评论，返回删除条数。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
	// CountByArticle 统计某文章的评论数。
	CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
}

// UserPort 是用户模块的只读能力。
type UserPort interface {
	// Profile 取单个用户的展示信息。
	Profile(ctx context.Context, id domain.UserID) (AuthorView, error)
	// Profiles 批量取；缺失的 ID 直接跳过。
	Profiles(ctx context.Context, ids []domain.UserID) (map[domain.UserID]AuthorView, error)
}

// TxManager 划定事务边界。
//
// 实现放在 adapter（用 platform/mysql 的句柄开事务，
// 再用同一个 tx 构造 Repos）；测试用 apptest.Tx 注入失败与回滚。
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r Repos) error) error
}

// AuthorStatsPort 是跨模块写端口。
//
// 同步实现：由 adapter 把**当前事务句柄**交给装配层注入的写入函数
// （internal/user/adapter/mysql.ApplyArticleDelta），因此仍然同事务，
// 但 article 不 import user 模块的任何实现包。
// 不做异步/outbox 的原因：会改变"删完文章作者文章数立刻减 1"的既有语义。
type AuthorStatsPort interface {
	ApplyArticleDelta(ctx context.Context, authorID domain.UserID, articleDelta, viewDelta int) error
}

// OutboxPort 是领域事件出口。
type OutboxPort interface {
	Emit(ctx context.Context, e Event) error
}

// Event 是一条领域事件。
//
// Payload 用 []byte 而不是 any：与日志治理的结论一致，
// 禁止把任意结构体塞进事件，避免出现无法审计的字段。
type Event struct {
	// Name 是事件名，如 "article.published"。
	Name string
	// Payload 是已序列化的 JSON。
	Payload []byte
	// AggregateID 是事件主体的 ID（文章事件就是文章 ID）。
	//
	// 单独带一份而不是让消费者去解 payload：outbox 表要按它建索引，
	// 运维排查"这篇文章的事件卡在哪一步"时可以直接查。
	AggregateID ctype.ID
	// AggregateType 是聚合类型，当前只有 "article"。
	AggregateType string
}

// SubmitQuotaPort 是「投稿配额」端口（票 304 / ADR-0022）。
//
// 由使用方（本包）定义、装配层接 Redis 适配器；形状对齐私信发送限流的预占 / 释放
// （domain.Limiter）：一个 Reserve 方法 + 一份可释放的预占，不引入"查询剩余额度"
// 这类读方法——额度只在提交那一刻被判定，其他地方要显示剩余量应当另开只读端口。
type SubmitQuotaPort interface {
	// Reserve 预占一次当日的投稿额度。
	//
	// 额度用完返回 domain.ErrSubmitQuotaExceeded；其余错误视为系统故障
	// （失败关闭：不放行提交）。成功时调用方在提交失败后必须 Release。
	Reserve(ctx context.Context, userID domain.UserID) (SubmitQuotaReservation, error)
}

// SubmitQuotaReservation 是一次已占用的投稿额度。
//
// Release 必须幂等：调用点常写成"显式归还 + defer 兜底"，重复归还会把
// 当日额度凭空放大。
type SubmitQuotaReservation interface {
	Release(ctx context.Context) error
}

// QualityGatePort 是「发布质量闸门」的评分端口（票 306 / ADR-0023）。
//
// 由使用方（本包）定义、装配层接到 AI 模块。形状只有一个方法：对一篇待提交
// 的文章打一次分，并返回分数与问题列表；"拿不到可用分数"必须返回错误。
//
// 这是对"article app 不直接依赖 AI 模块"的一次**有意突破**：文章应用层原本只
// 通过事务型发件箱产生副作用，而闸门是**同步决策** —— 分数决定这次提交落草稿
// 还是进审核，等不了异步中继，因此必须有这个窄口子。ADR-0023 记录了这条张力
// 与理由，不要在实现里把评分调用挪回事务内。
type QualityGatePort interface {
	// Evaluate 对一篇待提交文章评分。
	//
	// 网络错、超时、限流、5xx、返回非 JSON、缺总分等"没拿到可用分数"的情形
	// 一律返回错误（失败关闭）；拿到分数（哪怕 23 分）就返回结果，由用例层
	// 按阈值判定，不算系统故障。
	Evaluate(ctx context.Context, in QualityGateInput) (QualityGateScore, error)
}

// QualityGateInput 是一次评分请求。
//
// 故意不传 cmd / 文章聚合：端口只需要"评分所必需的四件事"，这样装配层实现
// 不必理解文章的状态机，测试也不必造一整个聚合。
type QualityGateInput struct {
	ArticleID domain.ArticleID
	AuthorID  domain.UserID
	Title     string
	Content   string
	// UpdatedAt 是这次提交对应内容的更新时间，随评分记录落库用于回溯。
	UpdatedAt time.Time
	// AuthorIsAdmin 表示提交者是管理员：管理员不受 AI 每日额度约束（ADR-0028）。
	AuthorIsAdmin bool
}

// QualityGateScore 是一次可用的评分结论。
type QualityGateScore struct {
	// TotalScore 是 0–100 的总分。
	TotalScore int
	// Dimensions 是六个维度的得分（ADR-0023）。
	//
	// 闸门是**木桶**判定：任何一项低于及格线都不能发布，总分再高也不行。
	// 为空表示这次评分没有给出维度分（旧记录 / 端口实现未填），此时只按总分判 ——
	// 拿不到维度分不该让所有人的提交都被拒，但也不能假装它过了。
	Dimensions []QualityDimension
	// Issues 是逐条问题，用于告诉作者"按哪几条改"。
	Issues []QualityIssue
}

// QualityDimension 是一个评分维度的得分。
//
// Name 是评分侧的英文键（clarity / structure / completeness / readability /
// persuasiveness / language），中文名由前端一处映射，不在后端重复维护。
type QualityDimension struct {
	Name  string
	Score int
}

// QualityIssue 是一条评分问题。
type QualityIssue struct {
	// Positions 是问题在正文里的定位；可能为空（模型没给位置）。
	Positions  []QualityPosition
	Reason     string
	Suggestion string
}

// QualityPosition 定位到正文的段落与原文引用。
type QualityPosition struct {
	Paragraph int
	Quote     string
}

// Clock 是可注入时钟。domain 不取时间，时间由用例层传入。
type Clock interface {
	Now() time.Time
}

// ModerationScorePort 是「机审第②层」读取已有评分记录的端口（票 311）。
//
// 由使用方（本包）定义、装配层接到 ai 模块的评分仓储；只读、批量、不触发
// 任何模型调用。没有评分记录的文章不出现在返回 map 里 —— "没有记录"是正常
// 状态，不是错误（审核列表照常渲染）。
type ModerationScorePort interface {
	LatestScores(ctx context.Context, articleIDs []domain.ArticleID) (map[domain.ArticleID]moderation.ScoreFacts, error)
}

// ModerationWordProvider 现读两档高危词表（机审第①层）。
//
// 词表是运行期配置：每次读取都现算，运营在后台改完，下一次列表请求立即
// 按新词表判，不缓存、不落库。返回的 WordLists 里空清单表示该档关闭。
type ModerationWordProvider func() moderation.WordLists

// IDGen 是可注入 ID 生成器。
//
// 生产用雪花，测试用固定序列；这样 golden 才能逐字节稳定。
type IDGen interface {
	NextArticleID() domain.ArticleID
	NextReviewTaskID() uint64
	NextFolderID() favorite.ID
	// NextCollectionID 生成合集 ID。
	//
	// 不借用 NextArticleID：两次新建的 ID 序列互相穿插时，日志里无法分辨
	// "这个 ID 是文章还是合集"，而合集的 ID 会出现在读者站 URL 上。
	NextCollectionID() collection.ID
	// NextProjectID 生成项目 ID。
	//
	// 与 NextCollectionID 分开：项目与合集是两套内容实体，ID 会出现在
	// 读者站 URL 上，日志里要能分辨"这是项目还是合集"。
	NextProjectID() project.ID
	// NextGroupID 生成分组 ID（ADR-0019）。
	//
	// 与合集 / 项目 ID 分开：分组 ID 出现在后台接口里，日志要能分辨"这一行是
	// 分组编排还是内容实体"。
	NextGroupID() grouping.ID
	// NextGroupIDItem 生成条目归属行的 ID。
	//
	// 归属行不是内容实体，只在本模块内部出现；单独一个方法是为了让"这一行是
	// 编排"在日志里可辨认，而不是混进内容实体的 ID 序列。
	NextGroupItemID() ctype.ID
	// NextOutlineNodeID 生成目录树节点 ID。
	//
	// 树节点 ID 会出现在后台接口里（整树提交带回 ID 才能幂等更新），
	// 与文章 / 合集 / 项目 ID 分开，日志里能分辨"这一行是树节点"。
	NextOutlineNodeID() outline.ID
}

// Service 是用例集合。
//
// 字段不导出，只能通过 New 构造；依赖全部是接口，便于替换 fake。
type Service struct {
	tx    TxManager
	clock Clock
	ids   IDGen
	// quota 是投稿配额端口（票 304）。为 nil 表示"不设上限"——只有测试装配会这样；
	// 生产装配在 backend/app/article_wiring.go 里必然注入 Redis 适配器，
	// 而适配器在 Redis 不可用时返回错误（失败关闭，不放行提交）。
	quota SubmitQuotaPort
	// scores 是机审第②层的评分记录来源（票 311）。为 nil 表示"没有评分数据"，
	// 此时审核列表只算词表层、不报错；生产装配接到 ai 模块的评分仓储。
	scores ModerationScorePort
	// words 现读机审词表（票 311）。为 nil 表示两档词表都为空。
	words ModerationWordProvider
	// quality 是发布质量闸门的评分端口（票 306 / ADR-0023）。
	//
	// 为 nil 表示闸门即使被打开也无法评分 —— 此时按失败关闭处理（拒绝提交，
	// 错误码 article.score_unavailable）。生产装配必然注入（article_wiring.go）。
	quality QualityGatePort
	// activity 是创作活动读端口（票 354）。为 nil 表示"没有活动数据"，
	// 个人概览的热力图退回全零 —— 分析库不可用不该让整页打不开。
	activity ActivityPort
}

// WithActivity 注入创作活动读端口（票 354）。
//
// 用可选注入而不是加进 New 的参数表：New 已经有七个参数、几十处调用点，
// 为一个只在个人概览里用到的只读端口再动一遍全部调用点没有收益。
func (s *Service) WithActivity(p ActivityPort) *Service {
	s.activity = p
	return s
}

// New 构造用例服务。
//
// quota / scores / words / quality 为 nil 时分别退化为"不设上限 / 没有评分数据 /
// 空词表 / 无法评分"（测试装配会这样）；生产装配在 backend/app/article_wiring.go 里给出实现。
func New(
	tx TxManager, clock Clock, ids IDGen,
	quota SubmitQuotaPort, scores ModerationScorePort, words ModerationWordProvider,
	quality QualityGatePort,
) *Service {
	return &Service{
		tx: tx, clock: clock, ids: ids,
		quota: quota, scores: scores, words: words, quality: quality,
	}
}

// ActivityPort 是「某用户最近 N 天每天有多少次创作动作」（票 354）。
//
// 由使用方（本包）定义、装配层接分析模块的存储适配器实现。为什么是端口而不是
// 直接 import 分析模块的 adapter：垂直模块口径要求 article 不认识别的模块的
// 实现包（archcheck R11），而个人数据概览要的恰恰是"分析模块才有的数据"。
//
// 只读、只按用户维度取：个人概览的每一个数字都必须限定在当前登录用户身上，
// 端口形状里写死 userID 就是让"忘了加过滤"在编译期就不可能。
type ActivityPort interface {
	// DailyActivity 返回最近 days 天里每天成功的创作动作数，key 是 YYYY-MM-DD。
	//
	// 没有动作的日期不出现（调用方按天补零），返回空 map 不是错误：
	// 新用户从来没创作过，这是正常状态。
	DailyActivity(ctx context.Context, userID domain.UserID, days int) (map[string]int, error)
}
