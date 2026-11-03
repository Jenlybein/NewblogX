package domain

import (
	"context"
	"time"

	"myblogx/internal/kernel/ctype"
)

// SidebarCardKind 是右栏卡片的种类。
//
// 种类是**前端渲染分派**的依据，也是后端取数分派的依据：两端各自按同一个
// 字符串常量分派，所以这里是唯一来源。新增种类时必须同步契约与两端渲染。
type SidebarCardKind string

const (
	// CardKindHotArticles 热门文章（按点赞数排序）。
	CardKindHotArticles SidebarCardKind = "hot_articles"
	// CardKindLatestComments 最新评论。
	CardKindLatestComments SidebarCardKind = "latest_comments"
	// CardKindTagCloud 标签云（按出现权重决定字号）。
	CardKindTagCloud SidebarCardKind = "tag_cloud"
	// CardKindRecommendedCollections 推荐合集（合集模块自己的展示开关与顺序即运营配置）。
	//
	// 这一种类**没有**额外的"展示哪个合集"配置列：合集是否可推荐由合集自己的
	// is_display 决定，顺序由合集自己的 sort_order 决定。右栏卡的 sort_order 只决定
	// 这张卡在右栏里的位置，title 只覆盖卡片标题 —— 两套顺序各管各的，不互相顶替。
	CardKindRecommendedCollections SidebarCardKind = "recommended_collections"
	// CardKindRichText 富文本卡：运营在后台写一段带排版的 HTML，读者站清洗后渲染。
	//
	// 正文不走 markdown 渲染器，而是原样存 HTML（保存前过既有用户内容白名单）。
	// 这是右栏唯一一种"内容由运营自由填写"的卡，也是唯一需要清洗正文的卡。
	CardKindRichText SidebarCardKind = "rich_text"
)

// sidebarCardDefaultTitles 是每种卡的默认标题（运营没填标题时的回退）。
var sidebarCardDefaultTitles = map[SidebarCardKind]string{
	CardKindHotArticles:            "热门文章",
	CardKindLatestComments:         "最新评论",
	CardKindTagCloud:               "标签云",
	CardKindRecommendedCollections: "推荐合集",
	CardKindRichText:               "公告",
}

// Valid 报告种类是否是内置类型。
//
// 未知种类不拒绝读取（库里历史脏数据不能让整个右栏 500），
// 但管理端写入时必须拒绝，避免脏类型再流进来。
func (k SidebarCardKind) Valid() bool {
	_, ok := sidebarCardDefaultTitles[k]
	return ok
}

// DefaultSidebarCardTitle 返回该种类的默认标题；未知种类返回空串。
func DefaultSidebarCardTitle(k SidebarCardKind) string { return sidebarCardDefaultTitles[k] }

// 右栏卡片的数量上限与每张卡的数据条数。
//
// 上限是防误操作：一次提交 500 张卡既没有意义，也会让读者站首屏多出
// 500 次数据查询。数据条数是"一张侧栏卡能放下多少行"的产品口径，
// 与续票的分页无关。
const (
	SidebarCardMaxCount  = 20
	SidebarCardDataLimit = 5
)

// SidebarCard 是右栏的一张卡片配置。
type SidebarCard struct {
	ID        ctype.ID
	Kind      SidebarCardKind
	Title     string
	SortOrder int
	Enabled   bool
	// Content 是富文本卡的正文（其它种类恒为空串）。
	//
	// 保存前已经过既有用户内容白名单清洗；读侧拿到的是可渲染的 HTML 片段。
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SidebarArticle 是热门文章卡的数据项。
//
// 只带右栏展示需要的最小字段：标题、点赞数、阅读数。
type SidebarArticle struct {
	ID        ctype.ID
	Title     string
	DiggCount int
	ViewCount int
}

// SidebarComment 是最新评论卡的数据项。
type SidebarComment struct {
	ID           ctype.ID
	Content      string
	ArticleID    ctype.ID
	ArticleTitle string
	UserNickname string
	CreatedAt    time.Time
}

// SidebarTag 是标签云卡的数据项：ArticleCount 决定字号权重。
type SidebarTag struct {
	ID           ctype.ID
	Title        string
	ArticleCount int
}

// SidebarCollection 是推荐合集卡的数据项。
//
// 只带右栏一行能放下的内容：标题、简介、封面、文章数。
// 可见性（is_display）与顺序（合集自己的 sort_order）都已在查询里生效，
// 用例层拿到的每一行都是"读者此刻点得进去的合集"。
type SidebarCollection struct {
	ID           ctype.ID
	Title        string
	Description  string
	Cover        string
	ArticleCount int
}

// SidebarCardRepository 是右栏卡片配置的持久化端口。
type SidebarCardRepository interface {
	// ListEnabled 只返回启用中的卡片，按 SortOrder、ID 升序。
	ListEnabled(ctx context.Context) ([]SidebarCard, error)
	// ListAll 返回全部卡片（含停用），顺序与 ListEnabled 一致。
	ListAll(ctx context.Context) ([]SidebarCard, error)
	// Replace 全量替换卡片配置。
	//
	// 用全量替换而不是逐条增删：后台的排序是"整条列表一起改"，
	// 逐条更新在并发下会出现两套顺序互相覆盖。
	Replace(ctx context.Context, cards []SidebarCard) error
}

// SidebarDataSource 是右栏各卡数据的只读端口。
//
// 三种卡的数据来自三个不同模块，但都以"展示项"的形状收口在这里：
// 用例层只关心"这张卡有哪些条目"，不关心它们从哪张表来。
type SidebarDataSource interface {
	// HotArticles 按点赞数降序取热门文章。
	HotArticles(ctx context.Context, limit int) ([]SidebarArticle, error)
	// LatestComments 按时间降序取最新评论。
	LatestComments(ctx context.Context, limit int) ([]SidebarComment, error)
	// TagCloud 按文章数降序取标签。
	TagCloud(ctx context.Context, limit int) ([]SidebarTag, error)
	// Collections 按合集自己的运营顺序取**展示中**的合集。
	//
	// 只返回 is_display 为真的合集（与读者侧合集列表同一可见性口径）：
	// 未展示的合集留在表里给后台继续编辑，读者侧连卡片数据都拿不到。
	// ArticleCount 只数仍对匿名访客可见的文章，与合集列表页的读数一致。
	Collections(ctx context.Context, limit int) ([]SidebarCollection, error)
}

// SidebarCollectionReader 是推荐合集卡的数据源端口（由 article 模块提供实现）。
//
// 合集卡片的篇数与封面回退依赖目录树的深度优先遍历，这套口径只应有**一份**
// 实现：右栏不直连 article 的表，也不自己再数一遍，只消费它的读模型。
type SidebarCollectionReader interface {
	// RecommendedCollections 按合集自己的运营顺序取展示中的合集。
	//
	// 返回的 Cover 已经是最终生效封面（显式封面优先，空则 DFS 第一个公开
	// 文章节点的封面）；ArticleCount 只数仍公开的文章。是否过滤非管理员
	// 作者的文章由实现侧按站点开关现读，右栏不再重复判定。
	RecommendedCollections(ctx context.Context, limit int) ([]SidebarCollection, error)
}
