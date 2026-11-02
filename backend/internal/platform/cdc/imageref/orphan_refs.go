package imageref

import (
	"fmt"
	"sort"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"

	"gorm.io/gorm"
)

// 本文件是"主体已消失、账本还没清"的引用行扫描与清理。
//
// 为什么需要它：引用账本靠 CDC 维护，而 DROP / 重建表是 DDL 事件，
// 不会触发任何行级事件。重建轮播表之后，账本里会留下一批 owner_id
// 指向"已经不存在（或已软删）的轮播"的行。它们不报错，只会让
// 图片的引用计数偏高、孤儿图清单迟迟不敢删图。
//
// 判定口径刻意与 CDC 一致：主体表里 `deleted_at IS NULL` 才算活着，
// 软删与已物理消失一样算孤儿。

// OrphanRefPlan 是一次扫描的结论。
//
// 只读扫描与真删共用同一份计划：不带 -apply 时只打印它、绝不写库。
type OrphanRefPlan struct {
	// RefType 是本轮扫描的主体类型。
	RefType image_ref_enum.RefType
	// OwnerIDs 是"主体已消失"的非空 owner_id，升序去重。
	OwnerIDs []ctype.ID
	// NullOwnerRows 是 owner_id 为 NULL 的孤儿行数（无法按 owner_id 删）。
	NullOwnerRows int64
	// Rows 是计划要清理的引用行总数。
	Rows int64
}

// orphanSubject 是"引用主体"在库里的位置。目前只登记轮播；
// 新增一类主体时要在这里显式登记，而不是拿轮播表硬套。
type orphanSubject struct {
	table string
}

// orphanSubjectOf 返回主体类型对应的主体表。
func orphanSubjectOf(refType image_ref_enum.RefType) (orphanSubject, error) {
	switch refType {
	case image_ref_enum.RefTypeBanner:
		return orphanSubject{table: "banner_models"}, nil
	default:
		return orphanSubject{}, fmt.Errorf("imageref: no orphan-ref scan registered for ref type %q", refType.String())
	}
}

// ScanOrphanRefs 找出 ref_type 指向"已不存在或已软删"主体的引用行。
//
// 只读：不写任何一张表。主体表名来自固定映射，不拼接外部输入。
func ScanOrphanRefs(db *gorm.DB, refType image_ref_enum.RefType) (OrphanRefPlan, error) {
	plan := OrphanRefPlan{RefType: refType}
	if db == nil {
		return plan, gorm.ErrInvalidDB
	}
	subject, err := orphanSubjectOf(refType)
	if err != nil {
		return plan, err
	}

	// 非空 owner_id：有主体行但已软删，或干脆没有主体行。
	var owners []struct {
		OwnerID ctype.ID `gorm:"column:owner_id"`
	}
	ownerSQL := fmt.Sprintf(`
		SELECT DISTINCT refs.owner_id AS owner_id
		FROM image_ref_models AS refs
		LEFT JOIN %s AS subjects
		  ON subjects.id = refs.owner_id
		 AND subjects.deleted_at IS NULL
		WHERE refs.ref_type = ?
		  AND refs.owner_id IS NOT NULL
		  AND subjects.id IS NULL
		ORDER BY refs.owner_id ASC`, subject.table)
	if err := db.Raw(ownerSQL, uint(refType)).Scan(&owners).Error; err != nil {
		return plan, err
	}
	for _, owner := range owners {
		if owner.OwnerID.IsZero() {
			continue
		}
		plan.OwnerIDs = append(plan.OwnerIDs, owner.OwnerID)
	}

	// 行数：上面那些 owner 的全部账本行，加上 owner_id 为 NULL 的孤儿行。
	countSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM image_ref_models AS refs
		LEFT JOIN %s AS subjects
		  ON subjects.id = refs.owner_id
		 AND subjects.deleted_at IS NULL
		WHERE refs.ref_type = ?
		  AND (refs.owner_id IS NULL OR subjects.id IS NULL)`, subject.table)
	if err := db.Raw(countSQL, uint(refType)).Scan(&plan.Rows).Error; err != nil {
		return plan, err
	}
	nullSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM image_ref_models AS refs
		LEFT JOIN %s AS subjects
		  ON subjects.id = refs.owner_id
		 AND subjects.deleted_at IS NULL
		WHERE refs.ref_type = ?
		  AND refs.owner_id IS NULL`, subject.table)
	if err := db.Raw(nullSQL, uint(refType)).Scan(&plan.NullOwnerRows).Error; err != nil {
		return plan, err
	}
	return plan, nil
}

// CleanupOrphanRefs 删除计划里的孤儿引用行，返回删除的行数。
//
// 复用 CDC 已有的 DeleteOwnerRefs（按 ref_type + owner_id 清账本），
// 不另写一套删除语义；owner_id 为 NULL 的行无法按 id 命中，单独清一遍。
// 整批在一个事务里：中途失败不留下"删了一半"的账本。
func CleanupOrphanRefs(db *gorm.DB, plan OrphanRefPlan) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if _, err := orphanSubjectOf(plan.RefType); err != nil {
		return 0, err
	}
	// 排序只影响删除顺序，不影响结果；固定顺序让日志可复现。
	sort.SliceStable(plan.OwnerIDs, func(i, j int) bool {
		return plan.OwnerIDs[i].Uint64() < plan.OwnerIDs[j].Uint64()
	})

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, ownerID := range plan.OwnerIDs {
			if err := DeleteOwnerRefs(tx, plan.RefType, ownerID); err != nil {
				return err
			}
		}
		if plan.NullOwnerRows > 0 {
			if err := tx.Unscoped().
				Where("ref_type = ? AND owner_id IS NULL", plan.RefType).
				Delete(&imageRefRow{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return plan.Rows, nil
}
