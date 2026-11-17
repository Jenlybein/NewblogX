// Package domain 是内容过滤模块的领域层：关键词风险的命中记录，以及后台
// 「审核 → 关键词风险」页需要的读面形状。
//
// 只管"一条命中记录是什么、风险页能看到什么"；不碰数据库、不碰 HTTP，
// 也不认识文章的评分闸门或评论的状态机（那些留在各自模块里）。
package domain

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// Surface 是文本面（ADR-0032 决策 3 的六个面）。
//
// 用字符串而不是 int8：风险页的筛选条件直接来自查参数，字符串既是外部形状
// 也是内部键，少一层"数字 ↔ 名字"的对照表。取值集合由 ParseSurface 收口 ——
// 新加一个面必须同时改这里，否则写入口拿不到合法值。
type Surface string

// 六个文本面。
const (
	SurfaceArticle    Surface = "article"
	SurfaceComment    Surface = "comment"
	SurfaceNickname   Surface = "nickname"
	SurfaceAbstract   Surface = "abstract"
	SurfaceChat       Surface = "chat"
	SurfaceCollection Surface = "collection"
)

// ParseSurface 把外部字符串解析成文本面；不认识返回 false。
//
// 空串按"不筛选"处理，由调用方判断，不在这里变成 SurfaceArticle —— 那会让
// "传错参数"静默退化成"只看文章"，比报错更难查。
func ParseSurface(raw string) (Surface, bool) {
	switch Surface(raw) {
	case SurfaceArticle:
		return SurfaceArticle, true
	case SurfaceComment:
		return SurfaceComment, true
	case SurfaceNickname:
		return SurfaceNickname, true
	case SurfaceAbstract:
		return SurfaceAbstract, true
	case SurfaceChat:
		return SurfaceChat, true
	case SurfaceCollection:
		return SurfaceCollection, true
	}
	return "", false
}

// Valid 判断文本面是否是登记过的六个之一。
func (s Surface) Valid() bool {
	_, ok := ParseSurface(string(s))
	return ok
}

// Status 是命中记录的处理状态。
//
// 四个取值对应风险页的四种归宿：还没人看（待处理）、看了判定为误报（已忽略）、
// 内容已被删除（已删除内容）、账号已被处置（封禁 / 禁言后内容保留）。
// "账号已处置"是票 399 新增的：封禁与禁言不删内容，但记录必须从待处理里退场，
// 否则管理员会反复看到同一条已经处理过的风险。
type Status string

// 四种状态。
const (
	// StatusPending 待处理：命中后默认落在这里。
	StatusPending Status = "pending"
	// StatusIgnored 已忽略：管理员判定为误报。
	StatusIgnored Status = "ignored"
	// StatusDeleted 已删除内容：命中的内容已被处置删除。
	StatusDeleted Status = "deleted"
	// StatusAccount 账号已处置：对命中用户做了封禁或禁言，内容本身保留。
	StatusAccount Status = "account"
)

// ParseStatus 把外部字符串解析成状态；不认识返回 false。
func ParseStatus(raw string) (Status, bool) {
	switch Status(raw) {
	case StatusPending:
		return StatusPending, true
	case StatusIgnored:
		return StatusIgnored, true
	case StatusDeleted:
		return StatusDeleted, true
	case StatusAccount:
		return StatusAccount, true
	}
	return "", false
}

// Valid 判断状态是否是登记过的四个之一。
func (s Status) Valid() bool {
	_, ok := ParseStatus(string(s))
	return ok
}

// Snippet 是一条命中的上下文（命中位置前后各 20 字）。
//
// Start / End 是命中词在 Text 内的 rune 下标（左闭右开），供风险页高亮。
// 用"相对 Text 的下标"而不是"相对全文的下标"：记录里只存 Text 与这条区间，
// 存全文下标会让消费方拿到一份无法自洽的数据。
type Snippet struct {
	Start int
	End   int
	Text  string
}

// Word 是一个词在本段文本里的全部命中。
//
// 同一个词出现多次只产生一条：风险页要的是"哪个词、在哪出现、上下文是什么"。
type Word struct {
	Word     string
	Tier     contentfilter.Tier
	Category contentfilter.Category
	Snippets []Snippet
}

// Hit 是一条关键词风险记录。
//
// 它**不存原文**（ADR-0032 决策 5）：原文在落库前就被替换掉了，这里只有命中词
// 与上下文片段。误报靠上下文判断，处置靠 #398 的四个动作。
type Hit struct {
	ID        ctype.ID
	CreatedAt time.Time
	Surface   Surface
	// ContentID 是命中内容在其所属实体表里的 ID。评论用新评论 ID。
	ContentID ctype.ID
	// UserID 是内容作者；风险页的"命中用户 Top"按它聚合。
	UserID ctype.ID
	// UserNickname 是读面填充的展示名，**不落库**：昵称会变，命中记录不该存快照。
	UserNickname string
	Words        []Word
	// HitCount 是命中处数（含白名单命中），统计口径见 hit_count 列注释。
	HitCount  int
	Status    Status
	HandlerID ctype.ID
	HandledAt time.Time
	Note      string
}

// Page 是风险页的分页参数（与其它列表同一口径：默认 10、上限 100）。
type Page struct {
	Page  int
	Limit int
}

// Normalize 收敛分页参数。
func (p Page) Normalize() Page {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return p
}

// Offset 返回 SQL 偏移量。
func (p Page) Offset() int {
	normalized := p.Normalize()
	return (normalized.Page - 1) * normalized.Limit
}

// Query 是风险页列表的筛选条件。零值字段表示"不按这一项筛"。
type Query struct {
	Surface Surface
	Status  Status
	UserID  ctype.ID
	Page    Page
}

// TopUser 是"命中用户 Top"的一行。
type TopUser struct {
	UserID   ctype.ID
	Nickname string
	// HitCount 是窗口内该用户的命中处数（按记录里的 hit_count 汇总）。
	HitCount int
}

// TopWindowDays 是"命中用户 Top"的统计窗口（天）。
//
// 它与**保留期**是两件独立的事，刻意不共用配置：Top 是一个固定口径的展示
// （"最近 90 天谁命中最多"），保留期是运营对审计数据的取舍（`content_filter.retain_days`）。
// 把两者绑在一起会让"调长保留期"顺手改掉 Top 的含义。
const TopWindowDays = 90

// DefaultRetainDays 是命中记录保留天数的兜底值（配置里没写或写歪时用它）。
//
// 领域层写一份而不是从 conf 读：清理任务的输入是"天数"，它不该反过来依赖配置包。
const DefaultRetainDays = 90

// Repository 是命中记录的持久化端口。
type Repository interface {
	// Create 写入一条命中记录。
	Create(ctx context.Context, hit *Hit) error
	// List 分页查风险记录（created_at 倒序），并返回满足条件的总条数。
	List(ctx context.Context, q Query) ([]Hit, int64, error)
	// TopUsers 返回 since 之后命中处数最多的前 limit 个用户。
	TopUsers(ctx context.Context, since time.Time, limit int) ([]TopUser, error)
	// FindByID 按 ID 查一条命中记录；不存在时返回 CodeFilterHitNotFound。
	FindByID(ctx context.Context, id ctype.ID) (Hit, error)
	// MarkHandled 把一条**待处理**记录改写成已处置（状态 / 处理人 / 时间 / 备注）。
	//
	// 只在 status = pending 时更新（条件更新），返回是否真的改写了：
	// 并发下两个管理员同时点处置，第二个会拿到 false，应用层据此重读当前状态
	// 并幂等返回 —— 而不是覆盖第一个人的处理痕迹。
	MarkHandled(ctx context.Context, id ctype.ID, h Handle) (bool, error)
	// PurgeBefore 物理删除 createdAt 早于 cutoff 的记录，返回删除条数。
	//
	// 物理删除而不是软删：命中记录本身就是审计副本，软删只会在同一张表里
	// 留一份"看不见但还在"的上下文片段 —— 那与"保留 90 天"的承诺相反。
	PurgeBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// WordStats 统计 since 之后每个命中词的出现处数（词云字号的数据源）。
	//
	// 按**处数**而不是按记录数：同一个词出现在三篇文章里，词云上的字号该更大。
	// 类目与档位取该词最近一次记录里的值（同一个词在两处被标了不同档位时以最新为准）。
	WordStats(ctx context.Context, since time.Time) ([]WordStat, error)
}

// WordStat 是一个命中词在统计窗口内的出现处数（词云的一格）。
type WordStat struct {
	Word string
	// Tier / Category 只用于颜色与筛选，不参与排序。
	Tier     contentfilter.Tier
	Category contentfilter.Category
	// Count 是窗口内的出现处数（同一记录里同一个词命中多处时按处数累加）。
	Count int
}

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}
