package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// OutlineRepo 是 outline.Repository 的 MySQL 实现。
//
// 合集与项目共用这一张树表：owner_kind 区分，SQL 只写一份。
// 旧扁平收录中间表（collection_article_models / project_article_models）
// 在这里**完全不出现**：本仓储只读写 content_outline_node_models。
type OutlineRepo struct {
	db *gorm.DB
}

// NewOutlineRepo 构造仓储。
func NewOutlineRepo(db *gorm.DB) *OutlineRepo { return &OutlineRepo{db: db} }

// ListNodes 返回某棵树的活动节点（不含软删行）。
func (r *OutlineRepo) ListNodes(
	ctx context.Context, ownerKind outline.OwnerKind, ownerID outline.ID,
) ([]*outline.Node, error) {
	var rows []outlineNodeRow
	err := r.db.WithContext(ctx).
		Where("owner_kind = ? AND owner_id = ?", int(ownerKind), ctype.ID(ownerID)).
		Order("parent_id ASC, sort_order ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toOutlineNodes(rows), nil
}

// FindByArticle 按（owner, 文章）读节点，**含软删行**。
//
// 软删行仍占着唯一键，重新收录同一篇文章时恢复它而不是新插入。
func (r *OutlineRepo) FindByArticle(
	ctx context.Context, ownerKind outline.OwnerKind, ownerID outline.ID, articleID outline.ArticleID,
) (*outline.Node, error) {
	var row outlineNodeRow
	err := r.db.WithContext(ctx).Unscoped().
		Take(&row, "owner_kind = ? AND owner_id = ? AND article_id = ?",
			int(ownerKind), ctype.ID(ownerID), ctype.ID(articleID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, outline.ErrNodeMissing
	}
	if err != nil {
		return nil, err
	}
	nodes := toOutlineNodes([]outlineNodeRow{row})
	return nodes[0], nil
}

// ListByArticle 返回这篇文章在**所有树**里的活跃文章节点（不含软删行）。
//
// 排序固定为 (owner_kind, owner_id, id)：阅读页 `?from=` 缺失时据此回退到
// 第一棵归属树，同一份数据每次得到同一棵。
func (r *OutlineRepo) ListByArticle(
	ctx context.Context, articleID outline.ArticleID,
) ([]*outline.Node, error) {
	var rows []outlineNodeRow
	err := r.db.WithContext(ctx).
		Where("article_id = ?", ctype.ID(articleID)).
		Order("owner_kind ASC, owner_id ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toOutlineNodes(rows), nil
}

// Create 批量插入节点。
//
// 用 map 写而不是结构体：文章节点的 article_id 为 NULL 与文件夹节点的
// 零值必须严格区分（NULL 是"文件夹"，不是"文章 ID=0"）。
func (r *OutlineRepo) Create(ctx context.Context, nodes []*outline.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, outlineNodeFields(n))
	}
	return r.db.WithContext(ctx).Model(&outlineNodeRow{}).Create(&rows).Error
}

// Update 批量保存节点的父节点 / 标题 / 顺序 / 软删状态。
//
// 用 map：sort_order=0、parent_id=0 这类零值也真的落库；恢复软删行时显式把
// deleted_at 写回 NULL。
func (r *OutlineRepo) Update(ctx context.Context, nodes []*outline.Node) error {
	for _, n := range nodes {
		updates := outlineNodeFields(n)
		delete(updates, "id")
		delete(updates, "created_at")
		updates["updated_at"] = n.UpdatedAt()
		if n.Deleted() {
			updates["deleted_at"] = n.UpdatedAt()
		} else {
			updates["deleted_at"] = nil
		}
		res := r.db.WithContext(ctx).Model(&outlineNodeRow{}).
			Where("id = ?", ctype.ID(n.ID())).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// MySQL 默认返回"实际变更行数"而不是匹配行数：提交的字段与库里完全
			// 一致时（幂等保存、只调顺序的保存）就是 0 行。所以行数不能当存在性用，
			// 只有真的查不到这一行才说明页面过期。
			exists, err := r.nodeExists(ctx, n.ID(), true)
			if err != nil {
				return err
			}
			if !exists {
				return outline.ErrNodeMissing
			}
		}
	}
	return nil
}

// nodeExists 判断一条节点行是否仍然存在。
//
// includeDeleted=true 时把软删行也算"存在"：Update 既要改活行，也要能恢复
// 软删行（Restore），所以它必须看到软删行。
//
// 写入路径不能拿 RowsAffected 当"行是否存在"：相等值的 UPDATE 在 MySQL 里
// 报 0 行。单独查一次才准。
func (r *OutlineRepo) nodeExists(ctx context.Context, id outline.ID, includeDeleted bool) (bool, error) {
	q := r.db.WithContext(ctx)
	if includeDeleted {
		q = q.Unscoped()
	}
	var count int64
	err := q.Model(&outlineNodeRow{}).
		Where("id = ?", ctype.ID(id)).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SoftDelete 批量软删。
func (r *OutlineRepo) SoftDelete(ctx context.Context, ids []outline.ID, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	raw := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, ctype.ID(id))
	}
	// 先确认这批行都在（且未软删），再落软删：同样不用 RowsAffected 判断存在性。
	var alive int64
	if err := r.db.WithContext(ctx).Model(&outlineNodeRow{}).
		Where("id IN ?", raw).
		Count(&alive).Error; err != nil {
		return err
	}
	if int(alive) != len(ids) {
		return outline.ErrNodeMissing
	}
	res := r.db.WithContext(ctx).Model(&outlineNodeRow{}).
		Where("id IN ?", raw).
		Updates(map[string]any{"deleted_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// outlineNodeFields 把节点映射成落库字段。
func outlineNodeFields(n *outline.Node) map[string]any {
	fields := map[string]any{
		"id":         ctype.ID(n.ID()),
		"created_at": n.CreatedAt(),
		"updated_at": n.UpdatedAt(),
		"owner_kind": int(n.OwnerKind()),
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

// toOutlineNodes 把行映射成领域节点。
func toOutlineNodes(rows []outlineNodeRow) []*outline.Node {
	out := make([]*outline.Node, 0, len(rows))
	for _, row := range rows {
		var articleID outline.ArticleID
		if row.ArticleID != nil {
			articleID = outline.ArticleID(*row.ArticleID)
		}
		out = append(out, outline.Rebuild(outline.RebuildParams{
			ID:        outline.ID(row.ID),
			OwnerKind: outline.OwnerKind(row.OwnerKind),
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
