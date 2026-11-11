// Package grouping 是「合集 / 项目分组」的纯规则：什么是合法分组、
// 整树顺序提交要满足什么条件。
//
// 为什么要单独一个包而不是并进 collection / project：两个实体用的是**同一套**
// 分组语义（一个分组标题 + 一组按顺序排列的条目 + 一个固定的「未分组」段），
// 差异只在表名。规则写两份的话，"未分组固定第一""删除分组不删条目"这些口径
// 早晚会漂成两种。
//
// 本包不含 SQL、不认识 gin：仓储契约由使用方（app 层）定义在 port.go，
// 这里只有聚合与校验。
package grouping

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// ID 是分组标识。
type ID = ctype.ID

// MaxTitleLen 是分组标题上限。
//
// 与合集标题同一个数字：分组标题在后台是一条可拖动的横条，128 已经远超
// 一行的容量，再长只会被截断。
const MaxTitleLen = 128

// 领域错误。
var (
	// ErrTitleRequired 分组标题为空。
	ErrTitleRequired = apperr.New(apperr.CodeGroupTitleRequired)
	// ErrTitleTooLong 分组标题超出 MaxTitleLen。
	ErrTitleTooLong = apperr.New(apperr.CodeGroupTitleTooLong)
	// ErrNotFound 分组不存在。
	ErrNotFound = apperr.New(apperr.CodeGroupNotFound)
	// ErrOrderMismatch 整树顺序提交与当前集合不一致。
	ErrOrderMismatch = apperr.New(apperr.CodeGroupOrderMismatch)
	// ErrWiring 装配层没有提供分组仓储。
	//
	// 单独一个错误码而不是复用 ErrNotFound：那是"这个分组不存在"（用户能理解、
	// 刷新一下就好），这是"服务端少装了一个端口"（要人修）。两者混在一起会让
	// 装配 bug 在线上表现成 1001「分组不存在」，查起来毫无线索。
	ErrWiring = apperr.New(apperr.CodeGroupWiring)
)

// Group 是一个分组。
//
// 字段私有：改标题只能走 Rename，"什么算合法标题"只有一份判断。
type Group struct {
	id        ID
	title     string
	sortOrder int
	createdAt time.Time
	updatedAt time.Time
}


// NewParams 是新建分组的输入。
type NewParams struct {
	ID        ID
	Title     string
	SortOrder int
	Now       time.Time
}

// New 建一个分组。
func New(p NewParams) (*Group, error) {
	title, err := normalizeTitle(p.Title)
	if err != nil {
		return nil, err
	}
	return &Group{
		id:        p.ID,
		title:     title,
		sortOrder: p.SortOrder,
		createdAt: p.Now,
		updatedAt: p.Now,
	}, nil
}

// RestoreParams 是从存储还原分组的输入。
type RestoreParams struct {
	ID        ID
	Title     string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Restore 从存储还原分组（不做校验：库里的行就是权威）。
func Restore(p RestoreParams) *Group {
	return &Group{
		id:        p.ID,
		title:     p.Title,
		sortOrder: p.SortOrder,
		createdAt: p.CreatedAt,
		updatedAt: p.UpdatedAt,
	}
}

// ID 返回分组标识。
func (g *Group) ID() ID { return g.id }

// Title 返回标题。
func (g *Group) Title() string { return g.title }

// SortOrder 返回分组之间的顺序号。
func (g *Group) SortOrder() int { return g.sortOrder }

// CreatedAt 返回创建时间。
func (g *Group) CreatedAt() time.Time { return g.createdAt }

// UpdatedAt 返回更新时间。
func (g *Group) UpdatedAt() time.Time { return g.updatedAt }

// Rename 改标题。
func (g *Group) Rename(title string, now time.Time) error {
	normalized, err := normalizeTitle(title)
	if err != nil {
		return err
	}
	g.title = normalized
	g.updatedAt = now
	return nil
}

// SetSortOrder 设置分组之间的顺序号。
func (g *Group) SetSortOrder(order int, now time.Time) {
	g.sortOrder = order
	g.updatedAt = now
}

// normalizeTitle 是标题的唯一校验点。
func normalizeTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", ErrTitleRequired
	}
	// 按字符数而不是字节数：中文标题按字节算会被提前截断。
	if len([]rune(title)) > MaxTitleLen {
		return "", ErrTitleTooLong
	}
	return title, nil
}

// Assignment 是"某个条目落在哪个分组、组内第几位"。
//
// GroupID 为零值表示**未分组**：未分组不是一种分组，它是"还没被放进任何分组"，
// 所以不建一行分组数据来代表它（那样每个实体都要维护一条永远存在的特殊行）。
type Assignment struct {
	OwnerID   ctype.ID
	GroupID   ID
	SortOrder int
}

// TreeOrder 是一次整树顺序提交。
//
// 语义是**幂等整份覆盖**：提交的一定是当前界面的最终排列，与拖了几次无关；
// 重复提交同一份内容结果相同。两个段都必须给全：
//   - Groups：分组的顺序与每组内的条目顺序；
//   - Ungrouped：未分组条目的顺序（界面上固定排在最前）。
type TreeOrder struct {
	Groups    []GroupOrder
	Ungrouped []ctype.ID
}

// GroupOrder 是一个分组在整树提交里的位置与内容。
type GroupOrder struct {
	ID    ID
	Items []ctype.ID
}

// CheckOrder 校验整树顺序提交的输入。
//
// 三条规则，与标签 / 项目 / 热门置顶的全量换序同一条语义：
//  1. 分组集合必须是当前分组的一个排列（缺 / 多 / 重复 / 未知 ID 全拒）；
//  2. 每个条目必须**恰好出现一次**（漏掉的会被静默排丢，重复的无法判定归属）；
//  3. 条目集合必须是当前条目全集的一个排列。
//
// 集合不一致意味着"后台拿到的是上一次读到的列表"，此时整单拒绝让前端刷新后
// 重排，比"缺几个也照写"安全得多。
func CheckOrder(currentGroups []ID, currentOwners []ctype.ID, next TreeOrder) error {
	if !sameIDSet(currentGroups, groupIDs(next)) {
		return ErrOrderMismatch
	}

	seen := make(map[ctype.ID]struct{}, len(currentOwners))
	for _, group := range next.Groups {
		for _, owner := range group.Items {
			if _, dup := seen[owner]; dup {
				return ErrOrderMismatch
			}
			seen[owner] = struct{}{}
		}
	}
	for _, owner := range next.Ungrouped {
		if _, dup := seen[owner]; dup {
			return ErrOrderMismatch
		}
		seen[owner] = struct{}{}
	}
	if len(seen) != len(currentOwners) {
		return ErrOrderMismatch
	}
	for _, owner := range currentOwners {
		if _, ok := seen[owner]; !ok {
			return ErrOrderMismatch
		}
	}
	return nil
}

// groupIDs 取出提交里的分组 ID。
func groupIDs(next TreeOrder) []ID {
	out := make([]ID, 0, len(next.Groups))
	for _, group := range next.Groups {
		out = append(out, group.ID)
	}
	return out
}

// sameIDSet 判断两个集合是否相等（忽略顺序与重复）。
func sameIDSet(a, b []ID) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[ID]int, len(a))
	for _, id := range a {
		counts[id]++
	}
	for _, id := range b {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// Repository 是分组的存储契约（由 app 层使用，adapter 实现）。
//
// 两个实体各有一个实现（表名不同），但契约只有这一份：语义完全一致，
// 差异只在 SQL 里的表名。
type Repository interface {
	// ListGroups 返回全部分组（未软删），按 sort_order、id 升序。
	ListGroups(ctx context.Context) ([]*Group, error)

	// FindGroup 按 ID 读取；不存在时返回 ErrNotFound。
	FindGroup(ctx context.Context, id ID) (*Group, error)

	// SaveGroup 插入或更新一行分组（标题 / 顺序）。
	SaveGroup(ctx context.Context, g *Group) error

	// DeleteGroup 软删分组，并把它下面的条目移回未分组。
	//
	// **不删条目本身**：分组只是编排，删掉分组不该让合集 / 项目消失。
	DeleteGroup(ctx context.Context, id ID, now time.Time) error

	// ListAssignments 返回全部条目的归属与组内顺序。
	//
	// 包含未分组条目（GroupID 为零值），因为整树提交与列表读面都要它们的顺序。
	ListAssignments(ctx context.Context) ([]Assignment, error)

	// ReplaceOrder 用给定顺序整份覆盖分组顺序与条目归属。
	//
	// 幂等：同一份输入重复提交结果相同。集合一致性由 CheckOrder 保证，
	// 本方法只负责落库。
	ReplaceOrder(ctx context.Context, tree TreeOrder, now time.Time) error
}
