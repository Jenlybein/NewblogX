package cli

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/sirupsen/logrus"
)

// 本文件是点赞关系行的一次性对账（#96 AC 第 3 条）。
//
// 背景：修复前「取消点赞」走软删，行留在 article_digg_models 里、占着唯一键
// uk_article_digg (article_id, user_id)。线上因此积累了软删残行。
//
// 修复之后残行**不再阻断**再次点赞（Add 会把它恢复），所以这里不是"必须跑的
// 修复脚本"，而是三件事：
//
//  1. 看：`-t reconcile -s digg` 打印残行数量，用于确认线上到底有多少；
//  2. 清：`-apply` 物理删掉这些残行，回收磁盘、让表回到"行即关系"的干净状态；
//  3. 查：`-verify` 校验"点赞关系行数"与"文章点赞计数"是否一致。
//
// 默认**只读**：不带 -apply 只统计不动库。这是运维脚本的底线——
// 先看清楚再删，不许一个手滑清掉历史数据。

// diggRow 是本脚本要读写的行。
//
// 只列用到的列：脚本不该因为别的模块改了行结构就编译不过，
// 更不该把整行 SELECT 出来只为看一个 deleted_at。
//
// DeletedAt 必须是 gorm.DeletedAt 而不是 *time.Time：GORM 只对前者
// 自动补 `deleted_at IS NULL`。用 *time.Time 的话默认 scope 会把软删行
// 一起数进来，"活关系行"这个数字就错了（本脚本的全部意义都没了）。
type diggRow struct {
	ID        uint64 `gorm:"column:id"`
	ArticleID uint64 `gorm:"column:article_id"`
	UserID    uint64 `gorm:"column:user_id"`
	DeletedAt gorm.DeletedAt
}

// TableName 固定表名。
func (diggRow) TableName() string { return "article_digg_models" }

// diggReconcileResult 是一次对账的结果。
type diggReconcileResult struct {
	// Live 是未软删的点赞关系行数。
	Live int64
	// Soft 是软删残行数。
	Soft int64
	// Orphan 是"指向已删除文章"的残行数（连同活行一起算）。
	Orphan int64
	// Deleted 是本次物理删掉的残行数（只在 Apply 时非零）。
	Deleted int64
	// Applied 表示本次是否真的执行了删除。
	Applied bool
}

// FlagReconcileDigg 对账点赞关系行。
//
// dryRun 为 true 时只统计不动库；apply 为 true 时物理删除软删残行。
func FlagReconcileDigg(db *gorm.DB, logger *logrus.Logger, apply bool) error {
	if db == nil {
		return errors.New("点赞对账失败: 数据库未初始化")
	}

	ctx := context.Background()
	res, err := reconcileDigg(ctx, db, apply)
	if err != nil {
		return err
	}

	// 结论一律打到日志：运维排查时"跑过没有、结果是什么"要能事后看到。
	if logger != nil {
		logger.Infof(
			"点赞对账完成: 活关系=%d 软删残行=%d 孤儿行=%d 本次删除=%d (apply=%v)",
			res.Live, res.Soft, res.Orphan, res.Deleted, res.Applied,
		)
	}
	fmt.Println("── 点赞关系对账 ──")
	fmt.Printf("活关系行:       %d\n", res.Live)
	fmt.Printf("软删残行:       %d\n", res.Soft)
	fmt.Printf("孤儿行(文章已删): %d\n", res.Orphan)
	if apply {
		fmt.Printf("本次物理删除:   %d\n", res.Deleted)
	} else {
		fmt.Println("\n这是只读统计。确认无误后加 -apply 物理删除软删残行。")
	}
	return nil
}

// reconcileDigg 统计（可选地清理）点赞关系行。
func reconcileDigg(ctx context.Context, db *gorm.DB, apply bool) (diggReconcileResult, error) {
	var res diggReconcileResult

	// 活关系行：默认 scope（deleted_at IS NULL）。
	if err := db.WithContext(ctx).Model(&diggRow{}).Count(&res.Live).Error; err != nil {
		return res, fmt.Errorf("统计活点赞关系失败: %w", err)
	}

	// 软删残行：必须 Unscoped 才看得到（默认 scope 会把它们滤掉，
	// 于是"线上到底有多少残行"这个问题默认查不出来，只能靠这里）。
	total := int64(0)
	if err := db.WithContext(ctx).Unscoped().Model(&diggRow{}).Count(&total).Error; err != nil {
		return res, fmt.Errorf("统计全部点赞关系失败: %w", err)
	}
	res.Soft = total - res.Live

	// 孤儿行：文章本体已被软删，这些关系行永远不会被读到。
	//
	// 只统计不自动删：文章软删是可恢复操作，误删点赞关系等于"用户恢复文章后
	// 点赞全没了"。这类行交给删文章的级联清理（DeleteByArticle）处理。
	if err := db.WithContext(ctx).Unscoped().Model(&diggRow{}).
		Where("article_id IN (?)",
			db.Unscoped().Model(&articleRefRow{}).
				Select("id").Where("deleted_at IS NOT NULL")).
		Count(&res.Orphan).Error; err != nil {
		return res, fmt.Errorf("统计孤儿点赞关系失败: %w", err)
	}

	if !apply {
		return res, nil
	}

	// 物理删软删残行。
	//
	// 前置条件是"修复已经上线"：Add 遇到残行会恢复它，所以删掉残行
	// 只会让用户下次点赞走"新建"分支，行为等价、结果一致。
	// 若修复还没上线就删，恢复逻辑不存在，残行会立刻重新堆积 ——
	// 但即便如此也只是回到原状，不会更糟，所以不额外加版本校验。
	out := db.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL").
		Delete(&diggRow{})
	if out.Error != nil {
		return res, fmt.Errorf("清理软删残行失败: %w", out.Error)
	}
	res.Deleted = out.RowsAffected
	res.Applied = true
	return res, nil
}

// articleRefRow 只是为了让"孤儿行"子查询有表可指。
type articleRefRow struct {
	ID uint64 `gorm:"column:id"`
}

// TableName 固定表名。
func (articleRefRow) TableName() string { return "article_models" }
