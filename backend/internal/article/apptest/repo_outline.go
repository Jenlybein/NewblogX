package apptest

import (
	"context"
	"sync"
	"time"

	"myblogx/internal/article/domain/outline"
)

// OutlineRepo 是 outline.Repository 的内存实现。
//
// 有状态：整树提交的"新建 / 更新 / 软删 / 恢复"必须真的发生，才能断言
// "重复提交不产生新行""移出后再收录恢复原行"这类幂等语义。
type OutlineRepo struct {
	mu   sync.Mutex
	byID map[outline.ID]*outline.Node
	seq  []outline.ID // 保持插入顺序，读回结果稳定
	fault
	_count counter
}

// 字段名不能同时叫 count（与内嵌 counter 的方法冲突），用 _count 承装。
// fault 同理内嵌即可（无字段名冲突）。

// NewOutlineRepo 创建空仓储。
func NewOutlineRepo() *OutlineRepo {
	return &OutlineRepo{byID: make(map[outline.ID]*outline.Node)}
}

// Seed 预置节点（测试准备阶段用，不计入调用统计）。
func (r *OutlineRepo) Seed(nodes ...*outline.Node) *OutlineRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range nodes {
		cp := cloneNode(n)
		if _, ok := r.byID[cp.ID()]; !ok {
			r.seq = append(r.seq, cp.ID())
		}
		r.byID[cp.ID()] = cp
	}
	return r
}

// FailNext 让下一次 method 返回 err。
func (r *OutlineRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回方法调用次数。
func (r *OutlineRepo) Calls(method string) int { return r._count.Calls(method) }

// ActiveNodes 返回活跃节点（未软删），顺序按插入顺序。
func (r *OutlineRepo) ActiveNodes() []*outline.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*outline.Node, 0, len(r.seq))
	for _, id := range r.seq {
		if n, ok := r.byID[id]; ok && !n.Deleted() {
			out = append(out, cloneNode(n))
		}
	}
	return out
}

// AllNodes 返回全部节点（含软删行）。
func (r *OutlineRepo) AllNodes() []*outline.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*outline.Node, 0, len(r.seq))
	for _, id := range r.seq {
		if n, ok := r.byID[id]; ok {
			out = append(out, cloneNode(n))
		}
	}
	return out
}

// ListNodes 返回某棵树的活动节点。
func (r *OutlineRepo) ListNodes(_ context.Context, ownerKind outline.OwnerKind, ownerID outline.ID) ([]*outline.Node, error) {
	r._count.inc("ListNodes")
	if err := r.fault.take("ListNodes"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*outline.Node, 0, len(r.seq))
	for _, id := range r.seq {
		n, ok := r.byID[id]
		if !ok || n.Deleted() {
			continue
		}
		if n.OwnerKind() == ownerKind && n.OwnerID() == ownerID {
			out = append(out, cloneNode(n))
		}
	}
	return out, nil
}

// FindByArticle 按（owner, 文章）读节点，含软删行。
func (r *OutlineRepo) FindByArticle(
	_ context.Context, ownerKind outline.OwnerKind, ownerID outline.ID, articleID outline.ArticleID,
) (*outline.Node, error) {
	r._count.inc("FindByArticle")
	if err := r.fault.take("FindByArticle"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.seq {
		n, ok := r.byID[id]
		if !ok {
			continue
		}
		if n.OwnerKind() == ownerKind && n.OwnerID() == ownerID && n.ArticleID() == articleID {
			return cloneNode(n), nil
		}
	}
	return nil, outline.ErrNodeMissing
}

// Create 批量插入节点。
func (r *OutlineRepo) Create(_ context.Context, nodes []*outline.Node) error {
	r._count.inc("Create")
	if err := r.fault.take("Create"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range nodes {
		if _, ok := r.byID[n.ID()]; ok {
			return ErrConflict
		}
	}
	for _, n := range nodes {
		cp := cloneNode(n)
		r.byID[cp.ID()] = cp
		r.seq = append(r.seq, cp.ID())
	}
	return nil
}

// Update 批量保存节点。
func (r *OutlineRepo) Update(_ context.Context, nodes []*outline.Node) error {
	r._count.inc("Update")
	if err := r.fault.take("Update"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range nodes {
		if _, ok := r.byID[n.ID()]; !ok {
			return outline.ErrNodeMissing
		}
	}
	for _, n := range nodes {
		r.byID[n.ID()] = cloneNode(n)
	}
	return nil
}

// SoftDelete 批量软删。
func (r *OutlineRepo) SoftDelete(_ context.Context, ids []outline.ID, now time.Time) error {
	r._count.inc("SoftDelete")
	if err := r.fault.take("SoftDelete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		n, ok := r.byID[id]
		if !ok {
			return outline.ErrNodeMissing
		}
		n.SoftDelete(now)
	}
	return nil
}

// Snapshot 返回深拷贝状态（供事务回滚）。
func (r *OutlineRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := make(map[outline.ID]*outline.Node, len(r.byID))
	for id, n := range r.byID {
		byID[id] = cloneNode(n)
	}
	seq := append([]outline.ID(nil), r.seq...)
	return outlineSnapshot{byID: byID, seq: seq}
}

// RestoreState 用快照覆盖当前状态。
func (r *OutlineRepo) RestoreState(snap any) {
	s, ok := snap.(outlineSnapshot)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = s.byID
	r.seq = s.seq
}

type outlineSnapshot struct {
	byID map[outline.ID]*outline.Node
	seq  []outline.ID
}

func cloneNode(n *outline.Node) *outline.Node {
	return outline.Rebuild(outline.RebuildParams{
		ID:        n.ID(),
		OwnerKind: n.OwnerKind(),
		OwnerID:   n.OwnerID(),
		ParentID:  n.ParentID(),
		Kind:      n.Kind(),
		Title:     n.Title(),
		ArticleID: n.ArticleID(),
		SortOrder: n.SortOrder(),
		Deleted:   n.Deleted(),
		CreatedAt: n.CreatedAt(),
		UpdatedAt: n.UpdatedAt(),
	})
}
