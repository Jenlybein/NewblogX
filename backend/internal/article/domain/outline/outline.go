// Package outline 是「目录树」子域：合集与项目共用的层次与顺序。
//
// 一张树表（content_outline_node_models）同时服务两个 owner，节点分两种：
//   - 文件夹节点：只分组，不可阅读、不可点击，不带封面 / 摘要 / 正文；
//   - 文章节点：指向一篇文章。
//
// 规则只有一份，合集与项目的差异留在各自的主表与文案层（ADR-0023）。
// 本包只放"什么算合法树"的纯规则与纯函数，不含 SQL 也不认识 gin。
package outline

import (
	"context"
	"sort"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 类型别名：ID / ArticleID 都是 ctype.ID（与 collection 子域同一做法）。
type (
	// ID 是树节点标识。
	ID = ctype.ID
	// ArticleID 是文章节点所指文章的标识。
	ArticleID = ctype.ID
)

// OwnerKind 是树归属的实体类型。
//
// 一张表服务合集与项目：1 = 合集、2 = 项目。数值是落库口径，不要重排。
type OwnerKind int

const (
	// OwnerCollection 合集。
	OwnerCollection OwnerKind = 1
	// OwnerProject 项目。
	OwnerProject OwnerKind = 2
)

// Valid 判断归属类型是否已定义。
func (k OwnerKind) Valid() bool {
	return k == OwnerCollection || k == OwnerProject
}

// NodeKind 是节点类型。
type NodeKind int

const (
	// NodeKindFolder 文件夹节点：只分组。
	NodeKindFolder NodeKind = 1
	// NodeKindArticle 文章节点：指向一篇文章。
	NodeKindArticle NodeKind = 2
)

// Valid 判断节点类型是否已定义。
func (k NodeKind) Valid() bool {
	return k == NodeKindFolder || k == NodeKindArticle
}

// MaxTitleLen 是文件夹标题长度上限。
//
// 与合集标题同一个口径（卡片只显示一行标题，128 足够）；上限只在 app 层
// 校验，零 DDL 下加不了 CHECK。文章节点的标题不落库（读文章自身的标题）。
const MaxTitleLen = 128

// 领域错误。只写错误码，中文文案在 internal/apperr。
var (
	// ErrTitleRequired 文件夹标题为空。
	ErrTitleRequired = apperr.New(apperr.CodeOutlineTitleRequired)
	// ErrTitleTooLong 文件夹标题超出 MaxTitleLen。
	ErrTitleTooLong = apperr.New(apperr.CodeOutlineTitleTooLong)
	// ErrDuplicateArticle 同一棵树内重复挂载同一篇文章。
	ErrDuplicateArticle = apperr.New(apperr.CodeOutlineArticleDuplicated)
	// ErrArticleNotIncludable 文章不是"已发布且可见"，不能收录。
	ErrArticleNotIncludable = apperr.New(apperr.CodeOutlineArticleNotIncludable)
	// ErrStructureInvalid 树结构不合法（成环 / 父节点不是文件夹 / 跨 owner 混入）。
	ErrStructureInvalid = apperr.New(apperr.CodeOutlineStructureInvalid)
	// ErrNodeMissing 节点不存在（页面已过期）。
	ErrNodeMissing = apperr.New(apperr.CodeOutlineNodeMissing)
)

// Node 是目录树的一行。
//
// 字段私有：外部只能通过构造与方法改变状态，"什么算合法节点"只有一份判断。
type Node struct {
	id        ID
	ownerKind OwnerKind
	ownerID   ID
	parentID  ID
	kind      NodeKind
	title     string
	articleID ArticleID
	sortOrder int
	createdAt time.Time
	updatedAt time.Time
	deleted   bool
}

// NewParams 是创建一个新节点所需的全部输入。
//
// 不含 ID 生成与时间获取：两者都由调用方传入，domain 不依赖 time.Now 与雪花算法。
type NewParams struct {
	ID        ID
	OwnerKind OwnerKind
	OwnerID   ID
	ParentID  ID
	Kind      NodeKind
	Title     string
	ArticleID ArticleID
	SortOrder int
	Now       time.Time
}

// New 创建一个节点，并校验"创建时就必须成立"的规则。
func New(p NewParams) (*Node, error) {
	if !p.OwnerKind.Valid() {
		return nil, ErrStructureInvalid
	}
	if p.OwnerID.IsZero() {
		return nil, ErrStructureInvalid
	}
	if !p.Kind.Valid() {
		return nil, ErrStructureInvalid
	}
	title, err := checkNode(p.Kind, p.Title, p.ArticleID)
	if err != nil {
		return nil, err
	}
	return &Node{
		id:        p.ID,
		ownerKind: p.OwnerKind,
		ownerID:   p.OwnerID,
		parentID:  p.ParentID,
		kind:      p.Kind,
		title:     title,
		articleID: p.ArticleID,
		sortOrder: p.SortOrder,
		createdAt: p.Now,
		updatedAt: p.Now,
	}, nil
}

// RestoreParams 是从持久化数据重建节点的输入。
type RestoreParams struct {
	ID        ID
	OwnerKind OwnerKind
	OwnerID   ID
	ParentID  ID
	Kind      NodeKind
	Title     string
	ArticleID ArticleID
	SortOrder int
	Deleted   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Restore 从持久化数据重建节点（不做校验：库里已有的行必须能读出来）。
func Restore(p RestoreParams) *Node {
	return &Node{
		id:        p.ID,
		ownerKind: p.OwnerKind,
		ownerID:   p.OwnerID,
		parentID:  p.ParentID,
		kind:      p.Kind,
		title:     p.Title,
		articleID: p.ArticleID,
		sortOrder: p.SortOrder,
		createdAt: p.CreatedAt,
		updatedAt: p.UpdatedAt,
		deleted:   p.Deleted,
	}
}

// 只读访问器。
func (n *Node) ID() ID               { return n.id }
func (n *Node) OwnerKind() OwnerKind { return n.ownerKind }
func (n *Node) OwnerID() ID          { return n.ownerID }
func (n *Node) ParentID() ID         { return n.parentID }
func (n *Node) Kind() NodeKind       { return n.kind }
func (n *Node) Title() string        { return n.title }
func (n *Node) ArticleID() ArticleID { return n.articleID }
func (n *Node) SortOrder() int       { return n.sortOrder }
func (n *Node) CreatedAt() time.Time { return n.createdAt }
func (n *Node) UpdatedAt() time.Time { return n.updatedAt }
func (n *Node) Deleted() bool        { return n.deleted }

// IsFolder 判断是否是文件夹节点。
func (n *Node) IsFolder() bool { return n.kind == NodeKindFolder }

// IsArticle 判断是否是文章节点。
func (n *Node) IsArticle() bool { return n.kind == NodeKindArticle }

// Rename 改文件夹标题（走与创建同一条不变量）。
func (n *Node) Rename(title string, now time.Time) error {
	if !n.IsFolder() {
		return ErrStructureInvalid
	}
	trimmed, err := checkNode(NodeKindFolder, title, "")
	if err != nil {
		return err
	}
	n.title = trimmed
	n.updatedAt = now
	return nil
}

// MoveTo 把节点挂到新父节点下并设置兄弟顺序（零值 parent 表示根层）。
func (n *Node) MoveTo(parentID ID, sortOrder int, now time.Time) {
	n.parentID = parentID
	n.sortOrder = sortOrder
	n.updatedAt = now
}

// SetSortOrder 设置兄弟顺序。
func (n *Node) SetSortOrder(order int, now time.Time) {
	n.sortOrder = order
	n.updatedAt = now
}

// SoftDelete 标记删除（不物理删）。
func (n *Node) SoftDelete(now time.Time) {
	n.deleted = true
	n.updatedAt = now
}

// Restore 把软删行恢复成活跃行（重新收录同一篇文章时沿用原行）。
func (n *Node) Restore(now time.Time) {
	n.deleted = false
	n.updatedAt = now
}

// checkNode 是节点类型相关的唯一不变量检查。
func checkNode(kind NodeKind, title string, articleID ArticleID) (string, error) {
	switch kind {
	case NodeKindFolder:
		if !articleID.IsZero() {
			return "", ErrStructureInvalid
		}
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			return "", ErrTitleRequired
		}
		if len([]rune(trimmed)) > MaxTitleLen {
			return "", ErrTitleTooLong
		}
		return trimmed, nil
	case NodeKindArticle:
		if articleID.IsZero() {
			return "", ErrStructureInvalid
		}
		// 文章节点的标题读文章自身，不落库。
		return "", nil
	default:
		return "", ErrStructureInvalid
	}
}

// TreeNode 是嵌套视图的一层。
type TreeNode struct {
	Node     *Node
	Children []TreeNode
}

// Build 校验并组装成一棵嵌套树。
//
// 父节点在前、子节点跟随；兄弟按 (sort_order, id) 升序。空文件夹保留：
// 它表达"先搭章节骨架再挂文章"（#209 用户故事 33）。
func Build(nodes []*Node) ([]TreeNode, error) {
	if err := Validate(nodes); err != nil {
		return nil, err
	}

	children := make(map[ID][]*Node, len(nodes))
	for _, n := range nodes {
		children[n.parentID] = append(children[n.parentID], n)
	}
	for parentID := range children {
		rows := children[parentID]
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].sortOrder != rows[j].sortOrder {
				return rows[i].sortOrder < rows[j].sortOrder
			}
			return rows[i].id.Uint64() < rows[j].id.Uint64()
		})
	}

	var attach func(parentID ID) []TreeNode
	attach = func(parentID ID) []TreeNode {
		rows := children[parentID]
		out := make([]TreeNode, 0, len(rows))
		for _, row := range rows {
			out = append(out, TreeNode{Node: row, Children: attach(row.id)})
		}
		return out
	}
	return attach(""), nil
}

// Validate 校验整棵树的不变量。
//
// 规则：
//  1. 所有节点属于同一个 owner；
//  2. 父节点必须在树里、且必须是文件夹（文章节点不能有子节点）；
//  3. 不允许成环（含自引用）；
//  4. 同一棵树内一篇文章最多一个节点（跨树允许）。
func Validate(nodes []*Node) error {
	if len(nodes) == 0 {
		return nil
	}
	ownerKind := nodes[0].ownerKind
	ownerID := nodes[0].ownerID
	if !ownerKind.Valid() || ownerID.IsZero() {
		return ErrStructureInvalid
	}

	byID := make(map[ID]*Node, len(nodes))
	for _, n := range nodes {
		if n == nil {
			return ErrStructureInvalid
		}
		if n.ownerKind != ownerKind || n.ownerID != ownerID {
			return ErrStructureInvalid
		}
		if !n.kind.Valid() {
			return ErrStructureInvalid
		}
		if n.kind == NodeKindFolder {
			if n.articleID != "" {
				return ErrStructureInvalid
			}
			if _, err := checkNode(NodeKindFolder, n.title, ""); err != nil {
				return err
			}
		}
		if n.kind == NodeKindArticle && n.articleID.IsZero() {
			return ErrStructureInvalid
		}
		if !n.id.IsZero() {
			if _, dup := byID[n.id]; dup {
				return ErrStructureInvalid
			}
			byID[n.id] = n
		}
	}

	for _, n := range nodes {
		if n.parentID.IsZero() {
			continue
		}
		if n.parentID == n.id {
			return ErrStructureInvalid
		}
		parent, ok := byID[n.parentID]
		if !ok || parent.kind != NodeKindFolder {
			return ErrStructureInvalid
		}
	}

	// 成环检测：沿 parent 链向上走，步数超过节点数即说明回到了自己。
	for _, n := range nodes {
		steps := 0
		for cur := n; !cur.parentID.IsZero(); {
			parent, ok := byID[cur.parentID]
			if !ok {
				return ErrStructureInvalid
			}
			cur = parent
			steps++
			if steps > len(nodes) {
				return ErrStructureInvalid
			}
		}
	}

	seen := make(map[ArticleID]bool, len(nodes))
	for _, n := range nodes {
		if n.kind != NodeKindArticle {
			continue
		}
		if seen[n.articleID] {
			return ErrDuplicateArticle
		}
		seen[n.articleID] = true
	}
	return nil
}

// ViewNode 是后台树上的一行：节点 + 文章快照（文件夹节点没有文章信息）。
//
// 嵌套结构的每一层都带自己的文章快照，页面据此渲染文章标题与「已失效」标记。
type ViewNode struct {
	Node         *Node
	Article      *ArticleState
	ArticleTitle string
	ArticleCover string
	Children     []ViewNode
}

// Stale 判断这一行是否已失效（文件夹节点永远不失效）。
func (v ViewNode) Stale() bool { return v.Article.IsStale() }

// StaleReason 返回失效原因；有效或文件夹节点时为空值。
func (v ViewNode) StaleReason() domain.StaleReason { return v.Article.StaleReason() }

// Detail 是后台树编辑器的读模型：整棵嵌套树（含空文件夹）。
type Detail struct {
	OwnerKind OwnerKind
	OwnerID   ID
	Tree      []ViewNode
}

// Repository 是目录树的持久化契约。
//
// 写方法都要求调用方已在事务内（app 的 WithinTx）：整棵树一次提交会同时改
// 多行（重排、软删），半套顺序落库正是要避免的事。
type Repository interface {
	// ListNodes 返回某棵树**活跃**节点（不含软删行），顺序未定义。
	ListNodes(ctx context.Context, ownerKind OwnerKind, ownerID ID) ([]*Node, error)

	// FindByArticle 按（owner, 文章）读节点，**含软删行**。
	//
	// 唯一键不含 deleted_at：软删行仍占着文章键，所以"移除后再收录"必须恢复
	// 原行而不是新插入（与点赞表、旧收录中间表是同一个坑）。
	// 没有该文章节点时返回 ErrNodeMissing。
	FindByArticle(ctx context.Context, ownerKind OwnerKind, ownerID ID, articleID ArticleID) (*Node, error)

	// Create 批量插入节点（ID 已由调用方给好）。
	Create(ctx context.Context, nodes []*Node) error

	// Update 批量保存节点的父节点 / 标题 / 顺序 / 软删状态。
	//
	// 恢复软删行时会把 deleted_at 清空（调用方把 Deleted 置回 false）。
	Update(ctx context.Context, nodes []*Node) error

	// SoftDelete 批量软删节点。
	SoftDelete(ctx context.Context, ids []ID, now time.Time) error
}

// ArticleState 是文章节点所指文章**此刻**的状态（失效口径与运营位共用）。
//
// 编排动作（上移 / 下移 / 缩进 / 移出）只发生在后台页面：整棵树一次性提交，
// 领域层不再放一套导出但没人调用的单步 Move（ADR-0023 当前约束 7）。
type ArticleState = domain.ArticleState
