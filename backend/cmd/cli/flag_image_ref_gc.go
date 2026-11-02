package cli

import (
	"fmt"

	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/platform/cdc/imageref"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 本文件是"轮播已消失、账本还没清"的一次性清理：重建轮播表是 DDL 事件，
// 不触发任何行级 CDC 事件，旧轮播的图片引用行会留在账本里。
//
// 两种模式：
//   - 不带 -apply：只读清单，绝不写库；
//   - 带 -apply：删掉清单里的引用行（走 CDC 已有的 DeleteOwnerRefs）。
//
// 与 -t imagegc 的区别：那条命令清的是"没有引用的图片"，这条清的是
// "指向已消失主体的引用行"；后者不清，图片的引用计数会一直偏高。

// FlagImageRefGCOrphans 输出孤儿图片引用清单，并按需清理。
func FlagImageRefGCOrphans(db *gorm.DB, logger *logrus.Logger, apply bool) error {
	if db == nil {
		return fmt.Errorf("孤儿图片引用清理失败: 数据库未初始化")
	}

	// 目前只登记了轮播一类主体；新增主体类型时在 imageref 里显式登记。
	plan, err := imageref.ScanOrphanRefs(db, image_ref_enum.RefTypeBanner)
	if err != nil {
		return fmt.Errorf("扫描孤儿图片引用失败: %w", err)
	}
	printOrphanRefPlan(plan)

	if !apply {
		fmt.Println("\n这是预演：图片引用账本没有任何改动。带 -apply 才会删除这些行。")
		return nil
	}

	deleted, err := imageref.CleanupOrphanRefs(db, plan)
	if err != nil {
		return fmt.Errorf("清理孤儿图片引用失败: %w", err)
	}
	fmt.Printf("\n已清理 %d 行孤儿图片引用。\n", deleted)
	if logger != nil {
		logger.Infof("孤儿图片引用清理完成: ref_type=%s owners=%d rows=%d",
			plan.RefType, len(plan.OwnerIDs), deleted)
	}
	return nil
}

// printOrphanRefPlan 把清单打到标准输出。
func printOrphanRefPlan(plan imageref.OrphanRefPlan) {
	fmt.Println("── 孤儿图片引用清单（只读）──")
	fmt.Printf("主体类型:     %s\n", plan.RefType)
	fmt.Printf("孤儿主体:     %d 个\n", len(plan.OwnerIDs))
	for _, ownerID := range plan.OwnerIDs {
		fmt.Printf("  - owner_id=%s\n", ownerID)
	}
	if plan.NullOwnerRows > 0 {
		fmt.Printf("owner_id 为空: %d 行（无法按主体删除，会整行清掉）\n", plan.NullOwnerRows)
	}
	fmt.Printf("待清理引用行: %d 行\n", plan.Rows)
}
