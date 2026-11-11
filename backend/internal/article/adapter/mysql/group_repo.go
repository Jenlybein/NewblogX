package mysql

import (
	"context"
	"time"

	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

// 本文件是两个实体共用的分组存储实现。
//
// 表名不同、语义完全一致，所以逻辑只写一份：`groupTables` 描述一对表名，
// 两个构造入口各绑一份。复制一份 SQL 的代价是"删除分组时忘记把条目退回
// 未分组"这种差异会只修在一边。
type groupTables struct {
	// groups 是分组表名（如 collection_group_models）。
	groups string
	// items 是条目归属表名（如 collection_group_item_models）。
	items string
	// ownerColumn 是条目表里指向实体主键的列名（collection_id / project_id）。
	ownerColumn string
}

// GroupRepo 是 grouping.Repository 的 MySQL 实现。
type GroupRepo struct {
	db     *gorm.DB
	tables groupTables
	// nextID 生成条目归属行的主键。分组本身的主键由调用方（app 层）给好。
	nextID func() ctype.ID
}

var _ grouping.Repository = (*GroupRepo)(nil)

// NewCollectionGroupRepo 建合集分组的仓储。
func NewCollectionGroupRepo(db *gorm.DB, nextID func() ctype.ID) *GroupRepo {
	return &GroupRepo{
		db:     db,
		tables: groupTables{groups: "collection_group_models", items: "collection_group_item_models", ownerColumn: "collection_id"},
		nextID: nextID,
	}
}

// NewProjectGroupRepo 建项目分组的仓储。
func NewProjectGroupRepo(db *gorm.DB, nextID func() ctype.ID) *GroupRepo {
	return &GroupRepo{
		db:     db,
		tables: groupTables{groups: "project_group_models", items: "project_group_item_models", ownerColumn: "project_id"},
		nextID: nextID,
	}
}

// groupRow 是分组表的一行（两张分组表列完全相同）。
type groupRow struct {
	BaseRow
	Title     string
	SortOrder int
}

// groupItemRow 是条目归属表的一行（两张表列完全相同，只有 owner 列名不同）。
type groupItemRow struct {
	BaseRow
	GroupID   ctype.ID
	OwnerID   ctype.ID
	SortOrder int
}

// ListGroups 返回全部分组，按 sort_order、id 升序。
func (r *GroupRepo) ListGroups(ctx context.Context) ([]*grouping.Group, error) {
	var rows []groupRow
	if err := r.db.WithContext(ctx).Table(r.tables.groups).
		Where("deleted_at IS NULL").
		Order("sort_order asc, id asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*grouping.Group, 0, len(rows))
	for _, row := range rows {
		out = append(out, grouping.Restore(grouping.RestoreParams{
			ID:        grouping.ID(row.ID),
			Title:     row.Title,
			SortOrder: row.SortOrder,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}))
	}
	return out, nil
}

// FindGroup 按 ID 读取分组。
func (r *GroupRepo) FindGroup(ctx context.Context, id grouping.ID) (*grouping.Group, error) {
	var row groupRow
	err := r.db.WithContext(ctx).Table(r.tables.groups).
		Where("id = ? AND deleted_at IS NULL", ctype.ID(id)).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, grouping.ErrNotFound
		}
		return nil, err
	}
	return grouping.Restore(grouping.RestoreParams{
		ID:        grouping.ID(row.ID),
		Title:     row.Title,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}), nil
}

// SaveGroup 插入或更新一行分组。
func (r *GroupRepo) SaveGroup(ctx context.Context, g *grouping.Group) error {
	db := r.db.WithContext(ctx)
	updated := db.Table(r.tables.groups).
		Where("id = ?", ctype.ID(g.ID())).
		Updates(map[string]any{
			"title":      g.Title(),
			"sort_order": g.SortOrder(),
			"updated_at": g.UpdatedAt(),
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected > 0 {
		return nil
	}
	return db.Table(r.tables.groups).Create(map[string]any{
		"id":         ctype.ID(g.ID()),
		"created_at": g.CreatedAt(),
		"updated_at": g.UpdatedAt(),
		"title":      g.Title(),
		"sort_order": g.SortOrder(),
	}).Error
}

// DeleteGroup 软删分组，并把它下面的条目退回未分组。
//
// 两步在同一个事务里：只软删分组而不动条目，那些条目会指向一个不存在的分组，
// 读面既不在任何分组里、也不在未分组里 —— 界面上直接消失。
func (r *GroupRepo) DeleteGroup(ctx context.Context, id grouping.ID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Table(r.tables.items).
			Where("group_id = ?", ctype.ID(id)).
			Updates(map[string]any{"group_id": ctype.ID(""), "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Table(r.tables.groups).
			Where("id = ?", ctype.ID(id)).
			Update("deleted_at", now).Error
	})
}

// ListAssignments 返回全部条目的归属与组内顺序（含未分组）。
func (r *GroupRepo) ListAssignments(ctx context.Context) ([]grouping.Assignment, error) {
	var rows []groupItemRow
	if err := r.db.WithContext(ctx).Table(r.tables.items).
		Select("group_id, "+r.tables.ownerColumn+" AS owner_id, sort_order").
		Where("deleted_at IS NULL").
		Order("group_id asc, sort_order asc, id asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]grouping.Assignment, 0, len(rows))
	for _, row := range rows {
		out = append(out, grouping.Assignment{
			OwnerID:   row.OwnerID,
			GroupID:   grouping.ID(row.GroupID),
			SortOrder: row.SortOrder,
		})
	}
	return out, nil
}

// ReplaceOrder 用给定顺序整份覆盖分组顺序与条目归属。
//
// 幂等：每次都是"先按提交算出每行应有的 group_id/sort_order，再逐行 upsert"。
// 条目行不存在就插入（第一次把某个合集拖进分组时就是这样），存在就更新。
func (r *GroupRepo) ReplaceOrder(ctx context.Context, tree grouping.TreeOrder) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// 分组顺序：第 0 个拿最大序号？不需要——读面是 sort_order asc，
		// 所以第 0 个拿 1、依次递增，与列表顺序一致。
		for i, group := range tree.Groups {
			if err := tx.Table(r.tables.groups).
				Where("id = ?", ctype.ID(group.ID)).
				Updates(map[string]any{"sort_order": i + 1, "updated_at": now}).Error; err != nil {
				return err
			}
			for j, owner := range group.Items {
				if err := r.upsertItem(tx, owner, group.ID, j+1, now); err != nil {
					return err
				}
			}
		}

		for i, owner := range tree.Ungrouped {
			if err := r.upsertItem(tx, owner, "", i+1, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// upsertItem 把一条条目落到指定分组与位置。
func (r *GroupRepo) upsertItem(tx *gorm.DB, owner ctype.ID, groupID grouping.ID, order int, now time.Time) error {
	updated := tx.Table(r.tables.items).
		Where(r.tables.ownerColumn+" = ?", owner).
		Updates(map[string]any{
			"group_id":   ctype.ID(groupID),
			"sort_order": order,
			"updated_at": now,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected > 0 {
		return nil
	}

	row := map[string]any{
		"id":         r.nextID(),
		"created_at": now,
		"updated_at": now,
		"group_id":   ctype.ID(groupID),
		"sort_order": order,
		r.tables.ownerColumn: owner,
	}
	return tx.Table(r.tables.items).Create(row).Error
}
