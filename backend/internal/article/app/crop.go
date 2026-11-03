package app

import (
	"context"

	sitedomain "myblogx/internal/site/domain"
)

// deleteArticleCrop 真删一篇文章的取景行（取景表没有软删列）。
//
// 取景参数体系已在 #203 全量退场：本模块不再有校验、写入与读回取景参数的
// 代码，契约里也不再出现 crop。这里只剩下"删文章时清掉孤儿行"这一段——
// 表本身留到 #205 物理下线，期间历史行不该随着文章删除越积越多。
func deleteArticleCrop(ctx context.Context, r Repos, articleID uint64) error {
	return r.Crops().Delete(ctx, sitedomain.CropTargetArticle, []uint64{articleID})
}
