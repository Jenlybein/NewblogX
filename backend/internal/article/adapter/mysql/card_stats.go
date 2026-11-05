package mysql

import (
	"context"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// cardStats 是列表卡片从归属树派生的两个数字。
type cardStats struct {
	ArticleCount int
	// Cover 是 DFS 第一个公开文章节点的封面；它可能为空（交给前端空白兜底）。
	Cover string
}

// ownerCardStats 是两处列表卡片**唯一**的归属树读模型：调用方只传「哪个 owner」，
// 实现内部按归属走各自的具名树表（合集 / 项目）。
//
// 为什么在 Go 里组装而不是 SQL 子查询：封面回退要按**树本身的 DFS 顺序**
// 取第一个公开文章节点，这条顺序只有 outline.Build + ReaderView 一份实现；
// 在 SQL 里再写一遍（递归 CTE + 路径排序）就是第二套真相。合集 / 项目以十计，
// 一次读全量树的成本可以接受。
//
// 可见性口径与读者详情完全一致：文章用 domain 的 IsPubliclyVisible，
// onlyAdminAuthors 为真时再按作者角色过滤。旧共用树表不再有任何读取点。
func ownerCardStats(
	ctx context.Context, db *gorm.DB, kind outline.OwnerKind, onlyAdminAuthors bool,
) (map[ctype.ID]cardStats, error) {
	table := "collection_outline_node_models"
	if kind == outline.OwnerProject {
		table = "project_outline_node_models"
	}

	var nodeRows []namedOutlineRow
	if err := db.WithContext(ctx).Table(table).
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
			OwnerKind: kind,
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
