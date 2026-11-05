package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// cardStats 是列表卡片从目录树派生的两个数字。
type cardStats struct {
	ArticleCount int
	// Cover 是 DFS 第一个公开文章节点的封面；它可能为空（交给前端空白兜底）。
	Cover string
}

// outlineCardStats 一次性读出某类 owner 的全部树与文章快照，返回每棵树的
// 公开篇数与封面回退。
//
// 为什么在 Go 里组装而不是 SQL 子查询：封面回退要按**树本身的 DFS 顺序**
// 取第一个公开文章节点，这条顺序只有 outline.Build + ReaderView 一份实现；
// 在 SQL 里再写一遍（递归 CTE + 路径排序）就是第二套真相。合集 / 项目以十计，
// 一次读全量树的成本可以接受。
//
// 可见性口径与读者详情完全一致：文章用 domain 的 IsPubliclyVisible，
// onlyAdminAuthors 为真时再按作者角色过滤。
func outlineCardStats(
	ctx context.Context, db *gorm.DB, ownerKind outline.OwnerKind, onlyAdminAuthors bool,
) (map[ctype.ID]cardStats, error) {
	var nodeRows []outlineNodeRow
	err := db.WithContext(ctx).
		Where("owner_kind = ?", int(ownerKind)).
		Find(&nodeRows).Error
	if err != nil {
		return nil, err
	}

	byOwner := make(map[ctype.ID][]*outline.Node)
	articleIDs := make([]ctype.ID, 0)
	seenArticle := make(map[ctype.ID]bool)
	for _, n := range toOutlineNodes(nodeRows) {
		ownerID := ctype.ID(n.OwnerID())
		byOwner[ownerID] = append(byOwner[ownerID], n)
		if n.IsArticle() {
			raw := ctype.ID(n.ArticleID())
			if !seenArticle[raw] {
				seenArticle[raw] = true
				articleIDs = append(articleIDs, raw)
			}
		}
	}

	snaps, err := loadCardArticleSnapshots(ctx, db, articleIDs, onlyAdminAuthors)
	if err != nil {
		return nil, err
	}

	stats := make(map[ctype.ID]cardStats, len(byOwner))
	for ownerID, nodes := range byOwner {
		tree, err := outline.Build(nodes)
		if err != nil {
			return nil, err
		}
		view, err := outline.ReaderView(tree, func(articleID outline.ArticleID) (outline.ReaderSnapshot, error) {
			return snaps[ctype.ID(articleID)], nil
		})
		if err != nil {
			return nil, err
		}
		stats[ownerID] = cardStats{ArticleCount: view.ArticleCount, Cover: view.Cover}
	}
	return stats, nil
}

// projectCardStats 是 #276 项目侧迁移后的卡片读面：篇数与封面回退只从
// project_outline_node_models 读，与项目具名端点、项目详情页同一张表。
//
// 合集侧调用仍走 outlineCardStats（旧共用表），逐字不动、归 #275 / #277 收；
// 这里不按开关二选一，也不把两张表合并成第二套分派：一次读卡片由调用方的
// 实体身份决定表名。领域节点里的 ownerKind 由表名给出（不落库）。
func projectCardStats(
	ctx context.Context, db *gorm.DB, onlyAdminAuthors bool,
) (map[ctype.ID]cardStats, error) {
	var nodeRows []namedOutlineRow
	if err := db.WithContext(ctx).Table("project_outline_node_models").
		Where("deleted_at IS NULL").Find(&nodeRows).Error; err != nil {
		return nil, err
	}

	byOwner := make(map[ctype.ID][]*outline.Node)
	articleIDs := make([]ctype.ID, 0)
	seenArticle := make(map[ctype.ID]bool)
	for _, row := range nodeRows {
		var articleID outline.ArticleID
		if row.ArticleID != nil {
			articleID = outline.ArticleID(*row.ArticleID)
		}
		n := outline.Rebuild(outline.RebuildParams{
			ID:        outline.ID(row.ID),
			OwnerKind: outline.OwnerProject,
			OwnerID:   outline.ID(row.OwnerID),
			ParentID:  outline.ID(row.ParentID),
			Kind:      outline.NodeKind(row.Kind),
			Title:     row.Title,
			ArticleID: articleID,
			SortOrder: row.SortOrder,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
		ownerID := ctype.ID(n.OwnerID())
		byOwner[ownerID] = append(byOwner[ownerID], n)
		if n.IsArticle() {
			raw := ctype.ID(n.ArticleID())
			if !seenArticle[raw] {
				seenArticle[raw] = true
				articleIDs = append(articleIDs, raw)
			}
		}
	}

	snaps, err := loadCardArticleSnapshots(ctx, db, articleIDs, onlyAdminAuthors)
	if err != nil {
		return nil, err
	}

	stats := make(map[ctype.ID]cardStats, len(byOwner))
	for ownerID, nodes := range byOwner {
		tree, err := outline.Build(nodes)
		if err != nil {
			return nil, err
		}
		view, err := outline.ReaderView(tree, func(articleID outline.ArticleID) (outline.ReaderSnapshot, error) {
			return snaps[ctype.ID(articleID)], nil
		})
		if err != nil {
			return nil, err
		}
		stats[ownerID] = cardStats{ArticleCount: view.ArticleCount, Cover: view.Cover}
	}
	return stats, nil
}

// cardArticleRow 是卡片读面需要的文章快照列。
type cardArticleRow struct {
	ID               ctype.ID
	Cover            string
	AuthorID         ctype.ID
	PublishStatus    int8
	VisibilityStatus string
	Deleted          bool
	AuthorRole       int8
}

// loadCardArticleSnapshots 批量读文章快照（包含已软删行，失效在 Go 里判定）。
//
// LEFT JOIN user_models 且排除软删用户：软删作者不再算管理员（与旧卡片查询
// 的 `u.deleted_at IS NULL` 同一口径）。
func loadCardArticleSnapshots(
	ctx context.Context, db *gorm.DB, articleIDs []ctype.ID, onlyAdminAuthors bool,
) (map[ctype.ID]outline.ReaderSnapshot, error) {
	out := make(map[ctype.ID]outline.ReaderSnapshot, len(articleIDs))
	if len(articleIDs) == 0 {
		return out, nil
	}

	var rows []cardArticleRow
	err := db.WithContext(ctx).
		Table("article_models AS a").
		Select("a.id, a.cover, a.author_id, a.publish_status, a.visibility_status, "+
			"a.deleted_at IS NOT NULL AS deleted, COALESCE(u.role, 0) AS author_role").
		Joins("LEFT JOIN user_models AS u ON u.id = a.author_id AND u.deleted_at IS NULL").
		Where("a.id IN ?", articleIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		a := domain.Restore(domain.RestoreParams{
			ID:         domain.ArticleID(row.ID),
			AuthorID:   domain.UserID(row.AuthorID),
			Cover:      row.Cover,
			Status:     toStatus(row.PublishStatus),
			Visibility: toVisibility(row.VisibilityStatus),
			Deleted:    row.Deleted,
		})
		visible := a.IsPubliclyVisible()
		if onlyAdminAuthors && row.AuthorRole != int8(enum.RoleAdmin) {
			visible = false
		}
		out[row.ID] = outline.ReaderSnapshot{Visible: visible, Cover: row.Cover}
	}
	return out, nil
}
