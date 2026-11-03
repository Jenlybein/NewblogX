// Package project 是「项目」子域：管理员编排的一组有顺序的文章。
//
// 项目与「合集」是**并列的独立内容实体**：两套表、两套接口、两套后台维护页，
// 页面形态复用但数据不共用。新入口内部文案一律写「项目」，不混用「合集」字样。
//
// 与「热门置顶」是三套东西，不要混用：
//   - 热门置顶：首页运营位，上限 12 条，表 article_hot_top_models；
//   - 合集：表 collection_models + collection_article_models；
//   - 项目：表 project_models + project_article_models（本包）。
//
// 项目是**管理员专属**的运营产物：作者不参与创建，读者只读。
// 本包只放"什么是合法项目"的纯规则与读模型，不含 SQL 也不认识 gin。
package project

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 类型别名。
//
// ID / ArticleID 都是 ctype.ID 的别名（与合集同一做法）：
// 底层是 string，契约里就是 string，不需要生成后再修补。
type (
	// ID 是项目标识。
	ID = ctype.ID
	// ArticleID 是被收录文章的标识。
	ArticleID = ctype.ID
)

// 标题长度上限（与合集逐字一致：卡片只显示一行标题）。
//
// 上限只在 app 层校验，不靠数据库约束（零 DDL 口径下加不了 CHECK，
// 超长会给 1406 而不是可读错误）。
const MaxTitleLen = 128

// 描述长度上限：卡片上就是一段两行以内的说明。
const MaxDescriptionLen = 512

// 领域错误。
//
// 与合集同一套做法：只写错误码，中文文案在 internal/apperr。
var (
	// ErrTitleRequired 标题为空。
	ErrTitleRequired = apperr.New(apperr.CodeProjectTitleRequired)
	// ErrTitleTooLong 标题超出 MaxTitleLen。
	ErrTitleTooLong = apperr.New(apperr.CodeProjectTitleRequired)
	// ErrNotFound 项目不存在（也覆盖"未展示，对读者不可见"）。
	//
	// 两种情形合并成一个码：读者侧不能靠响应差异探测还没公布的项目。
	ErrNotFound = apperr.New(apperr.CodeProjectNotFound)
)

// Project 是项目聚合根。
//
// 字段全部私有：外部只能通过构造与方法改变状态，"什么算合法项目"只有一份判断。
type Project struct {
	id          ID
	title       string
	description string
	cover       string
	creatorID   domain.UserID
	displayed   bool
	sortOrder   int
	deleted     bool
	createdAt   time.Time
	updatedAt   time.Time
}

// NewParams 是创建项目所需的全部输入。
//
// 不含 ID 生成与时间获取：两者都由调用方传入，domain 不依赖 time.Now 与雪花算法。
type NewParams struct {
	ID          ID
	Title       string
	Description string
	Cover       string
	CreatorID   domain.UserID
	Displayed   bool
	SortOrder   int
	Now         time.Time
}

// New 创建一个项目聚合。
//
// 只做"创建时就必须成立"的校验：标题非空且不超长。展示开关、封面、描述
// 都可以为空——一张没有封面的项目卡片要能退回到首篇公开文章的封面（由读模型负责）。
func New(p NewParams) (*Project, error) {
	if err := checkTitle(p.Title); err != nil {
		return nil, err
	}
	return &Project{
		id:          p.ID,
		title:       strings.TrimSpace(p.Title),
		description: p.Description,
		cover:       p.Cover,
		creatorID:   p.CreatorID,
		displayed:   p.Displayed,
		sortOrder:   p.SortOrder,
		createdAt:   p.Now,
		updatedAt:   p.Now,
	}, nil
}

// RestoreParams 是从持久化数据重建聚合的输入。
type RestoreParams struct {
	ID          ID
	Title       string
	Description string
	Cover       string
	CreatorID   domain.UserID
	Displayed   bool
	SortOrder   int
	Deleted     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Restore 从持久化数据重建聚合（不做校验：库里已有的行必须能读出来）。
func Restore(p RestoreParams) *Project {
	return &Project{
		id:          p.ID,
		title:       p.Title,
		description: p.Description,
		cover:       p.Cover,
		creatorID:   p.CreatorID,
		displayed:   p.Displayed,
		sortOrder:   p.SortOrder,
		deleted:     p.Deleted,
		createdAt:   p.CreatedAt,
		updatedAt:   p.UpdatedAt,
	}
}

// 只读访问器。

// ID 返回项目 ID。
func (p *Project) ID() ID { return p.id }

// Title 返回标题。
func (p *Project) Title() string { return p.title }

// Description 返回描述。
func (p *Project) Description() string { return p.description }

// Cover 返回封面 URL（空串表示退回首篇公开文章的封面）。
func (p *Project) Cover() string { return p.cover }

// CreatorID 返回创建者（管理员）ID。
func (p *Project) CreatorID() domain.UserID { return p.creatorID }

// Displayed 返回是否展示给读者。
func (p *Project) Displayed() bool { return p.displayed }

// SortOrder 返回项目之间的展示顺序（从 1 开始）。
func (p *Project) SortOrder() int { return p.sortOrder }

// Deleted 返回是否已软删。
func (p *Project) Deleted() bool { return p.deleted }

// CreatedAt 返回创建时间。
func (p *Project) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt 返回最后修改时间。
func (p *Project) UpdatedAt() time.Time { return p.updatedAt }

// checkTitle 是标题的唯一条不变量检查。
func checkTitle(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrTitleRequired
	}
	if len([]rune(trimmed)) > MaxTitleLen {
		return ErrTitleTooLong
	}
	return nil
}

// Card 是读者站项目列表页的一张卡片。
//
// ArticleCount 只数**仍公开**的文章：一张写着"20 篇"点进去只有 18 篇是坏体验。
type Card struct {
	ID           ID
	Title        string
	Description  string
	Cover        string
	ArticleCount int
	CreatedAt    time.Time
}

// IncludedArticle 是项目详情页里的一篇文章（按顺序排列）。
type IncludedArticle struct {
	ArticleID      ArticleID
	SortOrder      int
	Title          string
	Abstract       string
	Cover          string
	ViewCount      int
	AuthorNickname string
	CategoryTitle  string
}

// Detail 是项目详情：项目本身 + 按顺序排列的已公开文章。
//
// 为什么在 domain 而不是 app 拼：它是"读者看到的项目"的完整读模型，
// 一旦分叉（比如详情页少过滤一次失效行），入口就会多一张死链卡片。
type Detail struct {
	Project  *Project
	Articles []IncludedArticle
}

// Repository 是项目的持久化契约（本票只含读者侧公开读）。
//
// 管理端增删改与编排（#197）会在同一个接口上补写方法；这里刻意先不写，
// 避免为未实现的路径预埋空洞。
type Repository interface {
	// Find 按 ID 读取；不存在（含已软删）时返回 ErrNotFound。
	//
	// **包含未展示的项目**：展示开关的判定在 app 层（读者侧拿到未展示的
	// 项目一律按不存在处理），后台 #197 要能读到未展示的项目来编辑它。
	Find(ctx context.Context, id ID) (*Project, error)

	// ListCards 列出**展示中**的项目卡片（按 sort_order, id 升序）。
	//
	// onlyAdminAuthors 为真时，把"一篇公开文章都没有"（或收录文章的作者
	// 全是非管理员）的项目也过滤掉——站点「显示非管理员发布文章」开关关闭时，
	// 项目是读者端公开读面，与首页列表、详情同一口径（ADR-0017 决策 4）。
	ListCards(ctx context.Context, onlyAdminAuthors bool, p domain.Page) ([]Card, int64, error)

	// ListIncludedArticles 列出**仍公开**的收录文章，按顺序升序。
	//
	// 失效行不出现：读者侧靠这条查询过滤，不靠前端判断。
	ListIncludedArticles(ctx context.Context, id ID, onlyAdminAuthors bool) ([]IncludedArticle, error)
}
