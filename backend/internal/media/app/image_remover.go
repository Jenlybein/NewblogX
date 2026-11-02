package app

import (
	"context"

	"myblogx/internal/media/domain"
)

// ImageRemover 是"先删存储对象、再 DeleteWithRefs 清账本并硬删行"的维护用窄实现。
//
// 它不是第二套删除逻辑：与 Service.Remove 共用同一个 removeImages，
// 后者唯一一次实现了删除顺序（issue #140 要求复用既有能力）。
// 单独构造是为了让维护命令不必拉起上传任务缓存等无关依赖。
type ImageRemover struct {
	repo    domain.Repository
	storage domain.ObjectStorage
}

// NewImageRemover 构造维护用删除器。
func NewImageRemover(repo domain.Repository, storage domain.ObjectStorage) *ImageRemover {
	if repo == nil {
		panic("media/app: 维护删除器缺少图片仓储")
	}
	if storage == nil {
		panic("media/app: 维护删除器缺少对象存储")
	}
	return &ImageRemover{repo: repo, storage: storage}
}

var _ domain.OrphanPurgeStore = (*ImageRemover)(nil)

// Remove 实现 domain.OrphanPurgeStore。
func (r *ImageRemover) Remove(ctx context.Context, ids []domain.ImageID) (int, error) {
	return removeImages(ctx, r.repo, r.storage, ids)
}
