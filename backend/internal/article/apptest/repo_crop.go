package apptest

import (
	"context"
	"sync"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"
)

// CropRepo 是通用取景表（picture_crop_models）的内存实现。
//
// 键是 (target_type, target_id)：同一数值的 ID 在轮播 / 文章 / 合集三种主体下
// 是三条互不覆盖的记录，与真实表的复合主键一致：删文章只能清掉 target_type=article
// 的那一条。
//
// 取景参数体系已在 #203 退场，本模块只剩"删除文章时清孤儿行"这一条路径；
// 替身因此只存"行在不在"，不再存焦点与缩放。
type CropRepo struct {
	mu   sync.Mutex
	rows map[cropKey]struct{}
}

// cropKey 对应表里的复合主键。
type cropKey struct {
	TargetType domain.CropTargetType
	TargetID   ctype.ID
}

var _ domain.PictureCropRepository = (*CropRepo)(nil)

// NewCropRepo 构造空的取景仓储替身。
func NewCropRepo() *CropRepo {
	return &CropRepo{rows: make(map[cropKey]struct{})}
}

// Seed 预置一条历史取景行，供用例断言"删除文章会清掉它"。
//
// 取景行在生产代码里已经无处可写（#203），这里只是替身提供的播数据口。
func (r *CropRepo) Seed(targetType domain.CropTargetType, targetID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows[cropKey{TargetType: targetType, TargetID: ctype.NewID(targetID)}] = struct{}{}
}

// Has 报告某个主体还有没有取景行（用例断言用）。
func (r *CropRepo) Has(targetType domain.CropTargetType, targetID uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.rows[cropKey{TargetType: targetType, TargetID: ctype.NewID(targetID)}]
	return ok
}

// Delete 实现 domain.PictureCropRepository：真删一组主体的取景行。
func (r *CropRepo) Delete(_ context.Context, targetType domain.CropTargetType, targetIDs []uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range targetIDs {
		delete(r.rows, cropKey{TargetType: targetType, TargetID: ctype.NewID(id)})
	}
	return nil
}

// Snapshot 为事务回滚保存当前状态。
func (r *CropRepo) Snapshot() map[cropKey]struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make(map[cropKey]struct{}, len(r.rows))
	for k := range r.rows {
		out[k] = struct{}{}
	}
	return out
}

// RestoreState 用快照覆盖当前状态（事务回滚时调用）。
func (r *CropRepo) RestoreState(state any) {
	snap, ok := state.(map[cropKey]struct{})
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows = make(map[cropKey]struct{}, len(snap))
	for k := range snap {
		r.rows[k] = struct{}{}
	}
}
