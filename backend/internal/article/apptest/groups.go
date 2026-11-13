package apptest

import (
	"context"
	"time"

	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// FakeGroups 是分组仓储的内存实现（ADR-0019）。
//
// 与具名树 fake 同一做法：两个实体共用一份内存状态，用例层的规则与表名无关；
// 生产实现按表名分派，真 MySQL 契约用例负责钉住那一份行为。
//
// 它刻意保留"顺序"这一维：整树换序的语义就是顺序，如果 fake 只存集合，
// "拖完顺序没变"这种 bug 在用例里永远看不出来。
type FakeGroups struct {
	groups []*grouping.Group
	// assignments 是"条目 → (分组, 组内位置)"。
	assignments map[ctype.ID]grouping.Assignment
	// order 记录条目在各自段内的排列顺序（key 是分组 ID，未分组用零值）。
	// 用切片而不是只靠 SortOrder 数字：整树换序写的就是位置。
	order map[grouping.ID][]ctype.ID
	// nextItemID 给新插入的归属行发 ID（与生产同口径：唯一即可）。
	//
	// 类型是 ctype.ID 而不是裸整型：雪花 ID 超出 JS 安全整数，裸整型会让
	// 契约印成 integer（archcheck R12）。
	nextItemID ctype.ID
}

// NewFakeGroups 建一个空的分组 fake。
func NewFakeGroups() *FakeGroups {
	return &FakeGroups{
		assignments: make(map[ctype.ID]grouping.Assignment),
		order:       make(map[grouping.ID][]ctype.ID),
	}
}

var _ grouping.Repository = (*FakeGroups)(nil)

// SeedGroup 放一个分组进去（用例准备数据用）。
func (f *FakeGroups) SeedGroup(id grouping.ID, title string, sortOrder int) {
	f.groups = append(f.groups, grouping.Restore(grouping.RestoreParams{ID: id, Title: title, SortOrder: sortOrder}))
}

// SeedAssignment 放一条条目归属（用例准备数据用）。
func (f *FakeGroups) SeedAssignment(owner ctype.ID, groupID grouping.ID, order int) {
	f.assignments[owner] = grouping.Assignment{OwnerID: owner, GroupID: groupID, SortOrder: order}
	f.order[groupID] = append(f.order[groupID], owner)
}

// ListGroups 返回全部分组，按 sort_order 升序。
func (f *FakeGroups) ListGroups(_ context.Context) ([]*grouping.Group, error) {
	out := make([]*grouping.Group, len(f.groups))
	copy(out, f.groups)
	// 按 sort_order 升序：读面口径与生产一致，否则用例观察不到"新建排末尾"。
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].SortOrder() < out[i].SortOrder() {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// FindGroup 按 ID 读取分组。
func (f *FakeGroups) FindGroup(_ context.Context, id grouping.ID) (*grouping.Group, error) {
	for _, g := range f.groups {
		if g.ID() == id {
			return g, nil
		}
	}
	return nil, grouping.ErrNotFound
}

// SaveGroup 插入或更新一行分组。
func (f *FakeGroups) SaveGroup(_ context.Context, g *grouping.Group) error {
	for i, existing := range f.groups {
		if existing.ID() == g.ID() {
			f.groups[i] = g
			return nil
		}
	}
	f.groups = append(f.groups, g)
	return nil
}

// DeleteGroup 删分组，并把它的条目退回未分组。
func (f *FakeGroups) DeleteGroup(_ context.Context, id grouping.ID, _ time.Time) error {
	for i, g := range f.groups {
		if g.ID() != id {
			continue
		}
		f.groups = append(f.groups[:i], f.groups[i+1:]...)
		break
	}
	// 条目退回未分组，顺序接在现有未分组条目之后。
	orphans := f.order[id]
	delete(f.order, id)
	for _, owner := range orphans {
		a := f.assignments[owner]
		a.GroupID = ""
		f.assignments[owner] = a
		f.order[""] = append(f.order[""], owner)
	}
	return nil
}

// ListAssignments 返回全部条目的归属与组内顺序。
func (f *FakeGroups) ListAssignments(_ context.Context) ([]grouping.Assignment, error) {
	out := make([]grouping.Assignment, 0, len(f.assignments))
	// 先未分组（与读面口径一致），再按分组顺序。
	for i, owner := range f.order[""] {
		out = append(out, grouping.Assignment{OwnerID: owner, GroupID: "", SortOrder: i + 1})
	}
	for _, g := range f.groups {
		for i, owner := range f.order[g.ID()] {
			out = append(out, grouping.Assignment{OwnerID: owner, GroupID: g.ID(), SortOrder: i + 1})
		}
	}
	return out, nil
}

// ReplaceOrder 整份覆盖分组顺序与条目归属。
func (f *FakeGroups) ReplaceOrder(_ context.Context, tree grouping.TreeOrder, _ time.Time) error {
	// 分组顺序：数组位置即新顺序。
	byID := make(map[grouping.ID]*grouping.Group, len(f.groups))
	for _, g := range f.groups {
		byID[g.ID()] = g
	}
	next := make([]*grouping.Group, 0, len(f.groups))
	for i, node := range tree.Groups {
		g, ok := byID[node.ID]
		if !ok {
			continue
		}
		g.SetSortOrder(i+1, g.UpdatedAt())
		next = append(next, g)
	}
	f.groups = next

	// 归属：整份重写，不保留旧值。
	f.assignments = make(map[ctype.ID]grouping.Assignment)
	f.order = make(map[grouping.ID][]ctype.ID)
	for _, node := range tree.Groups {
		for i, owner := range node.Items {
			f.assignments[owner] = grouping.Assignment{OwnerID: owner, GroupID: node.ID, SortOrder: i + 1}
			f.order[node.ID] = append(f.order[node.ID], owner)
		}
	}
	for i, owner := range tree.Ungrouped {
		f.assignments[owner] = grouping.Assignment{OwnerID: owner, GroupID: "", SortOrder: i + 1}
		f.order[""] = append(f.order[""], owner)
	}
	return nil
}

// GroupsFor 让假仓储满足 app.GroupRepositoryFor。
//
// 与 NamedOutlineFor 同一做法：不按实体拆数据，两种 kind 指向同一份内存状态。
func (r *Repos) GroupsFor(_ context.Context, _ outline.OwnerKind) grouping.Repository {
	return r.groupRepo
}
