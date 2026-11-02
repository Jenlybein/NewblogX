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
// 是三条互不覆盖的记录，与真实表的复合主键一致。这里不校验取值——校验是用例层
// 的职责（domain.Crop.Validate），替身只管"存了什么、读回什么"。
type CropRepo struct {
	mu   sync.Mutex
	rows map[cropKey]domain.Crop
}

// cropKey 对应表里的复合主键。
type cropKey struct {
	TargetType domain.CropTargetType
	TargetID   ctype.ID
}

var _ domain.PictureCropRepository = (*CropRepo)(nil)

// NewCropRepo 构造空的取景仓储替身。
func NewCropRepo() *CropRepo {
	return &CropRepo{rows: make(map[cropKey]domain.Crop)}
}

// Load 实现 domain.PictureCropRepository：没有记录的主体不出现在返回 map 里。
func (r *CropRepo) Load(_ context.Context, targetType domain.CropTargetType, targetIDs []uint64) (map[uint64]domain.Crop, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make(map[uint64]domain.Crop, len(targetIDs))
	for _, id := range targetIDs {
		if crop, ok := r.rows[cropKey{TargetType: targetType, TargetID: ctype.NewID(id)}]; ok {
			out[id] = crop
		}
	}
	return out, nil
}

// Upsert 实现 domain.PictureCropRepository：写入或覆盖一条取景参数。
func (r *CropRepo) Upsert(_ context.Context, targetType domain.CropTargetType, targetID uint64, crop domain.Crop) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows[cropKey{TargetType: targetType, TargetID: ctype.NewID(targetID)}] = crop
	return nil
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

// Get 直接读一条记录，供用例断言"写入落库了没有"。
func (r *CropRepo) Get(targetType domain.CropTargetType, targetID uint64) (domain.Crop, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	crop, ok := r.rows[cropKey{TargetType: targetType, TargetID: ctype.NewID(targetID)}]
	return crop, ok
}

// Snapshot 为事务回滚保存当前状态。
func (r *CropRepo) Snapshot() map[cropKey]domain.Crop {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make(map[cropKey]domain.Crop, len(r.rows))
	for k, v := range r.rows {
		out[k] = v
	}
	return out
}

// RestoreState 用快照覆盖当前状态（事务回滚时调用）。
func (r *CropRepo) RestoreState(state any) {
	snap, ok := state.(map[cropKey]domain.Crop)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows = make(map[cropKey]domain.Crop, len(snap))
	for k, v := range snap {
		r.rows[k] = v
	}
}
