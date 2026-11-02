package domain

import "context"

// CropTargetType 是通用取景表里的主体类型。
//
// 取景的语义只有一套（焦点 0–1 + 缩放 1–2），主体类型却会持续增加，
// 所以按 (target_type, target_id) 存一张通用表，而不是每类主体各开一张。
// 类型值是写进表里的字符串：改名等于改数据，新增类型等于新数据，
// 都不需要改列。
type CropTargetType string

const (
	// CropTargetBanner 是首页轮播封面。
	CropTargetBanner CropTargetType = "banner"
	// CropTargetArticle 是文章封面。
	CropTargetArticle CropTargetType = "article"
	// CropTargetCollection 是合集封面。
	CropTargetCollection CropTargetType = "collection"
)

// Valid 报告主体类型是否已登记。
//
// 表里 target_type 没有枚举约束，写错类型不会报错、只会静默变成
// "没有取景"（读路径退化成居中、缩放 1）。所以写入口先拒绝未知类型。
func (t CropTargetType) Valid() bool {
	switch t {
	case CropTargetBanner, CropTargetArticle, CropTargetCollection:
		return true
	default:
		return false
	}
}

// PictureCropRepository 是通用取景表的持久化端口。
//
// 轮播封面、文章封面、合集封面共用这一套读写；具体存哪张表是适配器的事。
// "没有取景记录"与"记录里是缺省值"是两件事：Load 不返回缺省，由调用方补齐。
type PictureCropRepository interface {
	// Load 批量取一组主体的取景参数；没有记录的主体不出现在返回 map 里。
	Load(ctx context.Context, targetType CropTargetType, targetIDs []uint64) (map[uint64]Crop, error)
	// Upsert 写入或覆盖一条取景参数。
	Upsert(ctx context.Context, targetType CropTargetType, targetID uint64, crop Crop) error
	// Delete 真正删除一组主体的取景行（取景表没有软删列）。
	Delete(ctx context.Context, targetType CropTargetType, targetIDs []uint64) error
}
