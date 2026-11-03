package domain

import "context"

// CropTargetType 是通用取景表里的主体类型。
//
// 取景参数体系已在 #203 全量退场：没有任何代码再读写取景参数（焦点 0–1 +
// 缩放 1–2），表本身留到 #205 才物理下线。这里留下的只是主体类型的取值，
// 删除主体时靠它定位要清掉的孤儿行。
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
// 表里 target_type 没有枚举约束，写错类型不会报错、只会静默删错行。
func (t CropTargetType) Valid() bool {
	switch t {
	case CropTargetBanner, CropTargetArticle, CropTargetCollection:
		return true
	default:
		return false
	}
}

// PictureCropRepository 是通用取景表的清理端口。
//
// #203 之后取景参数不再被读写，端口只剩一件事：主体被删除时把它的取景行
// 真删掉（取景表没有软删列）。表留到 #205 物理下线，期间这段清理保证
// 历史行不会随着主体删除越积越多。
type PictureCropRepository interface {
	// Delete 真正删除一组主体的取景行（取景表没有软删列）。
	Delete(ctx context.Context, targetType CropTargetType, targetIDs []uint64) error
}
