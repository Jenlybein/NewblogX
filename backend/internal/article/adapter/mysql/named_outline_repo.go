package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是 #274 expand 期两张**按实体命名**的树表的持久化实现。
//
// 与旧共用表（outline_repo.go）的差别只有一处：归属不再是一列，而是由表名表达。
// 所以两张表共用同一份 SQL，只在构造时给定表名与固定的 OwnerKind；领域节点里的
// ownerKind 由仓储按表身份重建（不落库），domain 的校验规则因此原样复用。
//
// 唯一键 / 索引都是 (owner_id, article_id)，article_id 可空：文件夹节点落 NULL。
// 软删行仍占唯一键，FindByArticle 必须 Unscoped 才能恢复原行。
// 旧共用表在本票内仍由 OutlineRepo 读写；两套表并存只是 expand 期内部过渡，
// 不是兼容层，由 #277 收缩。

// namedOutlineRow 是两张具名树表共同的列形状。
//
// 刻意不声明 TableName()：它不参与 AutoMigrate、不是建表真相（真相是 schema.sql
// 与 testkit 的行结构），只在查询时按表名读写。table_names_test 因此不会把它
// 当成一张新表登记。
type namedOutlineRow struct {
	ID        ctype.ID       `gorm:"column:id;primaryKey"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	OwnerID   ctype.ID       `gorm:"column:owner_id"`
	ParentID  ctype.ID       `gorm:"column:parent_id"`
	Kind      int            `gorm:"column:kind"`
	Title     string         `gorm:"column:title"`
	ArticleID *ctype.ID      `gorm:"column:article_id"`
	SortOrder int            `gorm:"column:sort_order"`
}

// namedOutlineRepo 是一张具名树表上的 outline.Repository 实现。
type namedOutlineRepo struct {
	db    *gorm.DB
	table string
	kind  outline.OwnerKind
}

// NewCollectionOutlineRepo 构造合集具名树表仓储。
func NewCollectionOutlineRepo(db *gorm.DB) outline.Repository {
	return &namedOutlineRepo{db: db, table: "collection_outline_node_models", kind: outline.OwnerCollection}
}

// NewProjectOutlineRepo 构造项目具名树表仓储。
func NewProjectOutlineRepo(db *gorm.DB) outline.Repository {
	return &namedOutlineRepo{db: db, table: "project_outline_node_models", kind: outline.OwnerProject}
}

// ListNodes 返回某棵树的活动节点（不含软删行）。
func (r *namedOutlineRepo) ListNodes(
	ctx context.Context, _ outline.OwnerKind, ownerID outline.ID,
) ([]*outline.Node, error) {
	var rows []namedOutlineRow
	err := r.db.WithContext(ctx).Table(r.table).
		Where("owner_id = ? AND deleted_at IS NULL", ctype.ID(ownerID)).
		Order("parent_id ASC, sort_order ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return r.toNodes(rows), nil
}

// FindByArticle 按（owner, 文章）读节点，**含软删行**。
func (r *namedOutlineRepo) FindByArticle(
	ctx context.Context, _ outline.OwnerKind, ownerID outline.ID, articleID outline.ArticleID,
) (*outline.Node, error) {
	var row namedOutlineRow
	err := r.db.WithContext(ctx).Table(r.table).
		Where("owner_id = ? AND article_id = ?", ctype.ID(ownerID), ctype.ID(articleID)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, outline.ErrNodeMissing
	}
	if err != nil {
		return nil, err
	}
	return r.toNodes([]namedOutlineRow{row})[0], nil
}

// Create 批量插入节点。用 map 写：文件夹的 article_id 必须是 NULL 而不是零值。
func (r *namedOutlineRepo) Create(ctx context.Context, nodes []*outline.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, namedOutlineFields(n))
	}
	return r.db.WithContext(ctx).Table(r.table).Create(&rows).Error
}

// Update 批量保存节点。零值也真的落库；恢复软删行时显式把 deleted_at 写回 NULL。
func (r *namedOutlineRepo) Update(ctx context.Context, nodes []*outline.Node) error {
	for _, n := range nodes {
		updates := namedOutlineFields(n)
		delete(updates, "id")
		delete(updates, "created_at")
		updates["updated_at"] = n.UpdatedAt()
		if n.Deleted() {
			updates["deleted_at"] = n.UpdatedAt()
		} else {
			updates["deleted_at"] = nil
		}
		res := r.db.WithContext(ctx).Table(r.table).
			Where("id = ?", ctype.ID(n.ID())).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// MySQL 相等值的 UPDATE 报 0 行；单独查一次才能区分"幂等"与"页面过期"。
			var count int64
			if err := r.db.WithContext(ctx).Table(r.table).
				Where("id = ?", ctype.ID(n.ID())).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return outline.ErrNodeMissing
			}
		}
	}
	return nil
}

// SoftDelete 批量软删。
func (r *namedOutlineRepo) SoftDelete(ctx context.Context, ids []outline.ID, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, ctype.ID(id))
	}
	var alive int64
	if err := r.db.WithContext(ctx).Table(r.table).
		Where("id IN ? AND deleted_at IS NULL", raw).
		Count(&alive).Error; err != nil {
		return err
	}
	if int(alive) != len(ids) {
		return outline.ErrNodeMissing
	}
	return r.db.WithContext(ctx).Table(r.table).
		Where("id IN ?", raw).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
}

// namedOutlineFields 把领域节点映射成落库字段（不含 owner_kind：归属由表名表达）。
func namedOutlineFields(n *outline.Node) map[string]any {
	fields := map[string]any{
		"id":         ctype.ID(n.ID()),
		"created_at": n.CreatedAt(),
		"updated_at": n.UpdatedAt(),
		"owner_id":   ctype.ID(n.OwnerID()),
		"parent_id":  ctype.ID(n.ParentID()),
		"kind":       int(n.Kind()),
		"title":      n.Title(),
		"sort_order": n.SortOrder(),
	}
	if n.IsArticle() {
		fields["article_id"] = ctype.ID(n.ArticleID())
	} else {
		fields["article_id"] = nil
	}
	return fields
}

// toNodes 把行映射成领域节点；ownerKind 由表身份给出，不读库里的列。
func (r *namedOutlineRepo) toNodes(rows []namedOutlineRow) []*outline.Node {
	out := make([]*outline.Node, 0, len(rows))
	for _, row := range rows {
		var articleID outline.ArticleID
		if row.ArticleID != nil {
			articleID = outline.ArticleID(*row.ArticleID)
		}
		out = append(out, outline.Rebuild(outline.RebuildParams{
			ID:        outline.ID(row.ID),
			OwnerKind: r.kind,
			OwnerID:   outline.ID(row.OwnerID),
			ParentID:  outline.ID(row.ParentID),
			Kind:      outline.NodeKind(row.Kind),
			Title:     row.Title,
			ArticleID: articleID,
			SortOrder: row.SortOrder,
			Deleted:   row.DeletedAt.Valid,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}))
	}
	return out
}
