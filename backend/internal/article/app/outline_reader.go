package app

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
)

// 本文件是「读者侧目录树」的用例：合集与项目共一份读模型组装。
//
// 与后台 AdminOutline 的区别：读者树只保留**公开文章节点**，文件夹保留（包括
// 空文件夹）；失效文章节点不出现，篇数只数公开文章；没上传封面时回退到深度
// 优先第一篇公开文章的封面。展示开关与「显示非管理员发布文章」开关的判定
// 仍在各自实体的用例里，这里不另写一份可见性规则。

// ReaderOutlineDetail 是读者详情页的读模型。
type ReaderOutlineDetail struct {
	OwnerKind    outline.OwnerKind
	OwnerID      outline.ID
	Tree         []outline.ViewNode
	ArticleCount int
	// Cover 是最终生效封面：显式封面优先，空则取 DFS 第一篇公开文章的封面。
	Cover string
}

// readerOutline 组装读者树：复用 #217 的 outline.Build + 文章快照读逻辑，
// 不另开一套 SQL，也不读写已冻结的扁平收录中间表。
func (s *Service) readerOutline(
	ctx context.Context, r Repos, ownerKind outline.OwnerKind, ownerID outline.ID,
	onlyAdminAuthors bool, explicitCover string,
) (*ReaderOutlineDetail, error) {
	nodes, err := r.Outlines().ListNodes(ctx, ownerKind, ownerID)
	if err != nil {
		return nil, err
	}
	tree, err := outline.Build(nodes)
	if err != nil {
		return nil, err
	}

	profiles := make(map[domain.UserID]AuthorView)
	if onlyAdminAuthors {
		// 「显示非管理员发布文章」开关关闭时，作者不是管理员的文章不算公开：
		// 与首页列表、搜索、详情同一口径（ADR-0017 决策 4）。
		authorIDs := make([]domain.UserID, 0)
		seen := make(map[domain.UserID]bool)
		for _, n := range nodes {
			if !n.IsArticle() {
				continue
			}
			a, err := r.Articles().Find(ctx, domain.ArticleID(n.ArticleID()))
			if errors.Is(err, domain.ErrArticleNotFound) || err != nil {
				continue
			}
			uid := a.AuthorID()
			if !seen[uid] {
				seen[uid] = true
				authorIDs = append(authorIDs, uid)
			}
		}
		if len(authorIDs) > 0 {
			loaded, err := r.Users().Profiles(ctx, authorIDs)
			if err != nil {
				return nil, err
			}
			profiles = loaded
		}
	}

	type snapshot struct {
		visible bool
		title   string
		cover   string
	}
	snaps := make(map[outline.ArticleID]snapshot)
	visibleArticle := func(articleID outline.ArticleID) (snapshot, error) {
		if v, ok := snaps[articleID]; ok {
			return v, nil
		}
		a, err := r.Articles().Find(ctx, domain.ArticleID(articleID))
		if errors.Is(err, domain.ErrArticleNotFound) {
			snaps[articleID] = snapshot{}
			return snapshot{}, nil
		}
		if err != nil {
			return snapshot{}, err
		}
		v := snapshot{
			visible: a.IsPubliclyVisible(),
			title:   a.Title(),
			cover:   a.Cover(),
		}
		if onlyAdminAuthors && !profiles[a.AuthorID()].Admin {
			v.visible = false
		}
		snaps[articleID] = v
		return v, nil
	}

	var attach func(items []outline.TreeNode) ([]outline.ViewNode, error)
	attach = func(items []outline.TreeNode) ([]outline.ViewNode, error) {
		out := make([]outline.ViewNode, 0, len(items))
		for _, item := range items {
			view := outline.ViewNode{Node: item.Node}
			if item.Node.IsArticle() {
				snap, err := visibleArticle(item.Node.ArticleID())
				if err != nil {
					return nil, err
				}
				if !snap.visible {
					continue
				}
				state := &domain.ArticleState{
					Status:     domain.StatusPublished,
					Visibility: domain.VisibilityVisible,
				}
				view.Article = state
				view.ArticleTitle = snap.title
				view.ArticleCover = snap.cover
			}
			children, err := attach(item.Children)
			if err != nil {
				return nil, err
			}
			view.Children = children
			out = append(out, view)
		}
		return out, nil
	}

	readerTree, err := attach(tree)
	if err != nil {
		return nil, err
	}

	cover := explicitCover
	if cover == "" {
		cover = firstVisibleCover(readerTree)
	}
	return &ReaderOutlineDetail{
		OwnerKind:    ownerKind,
		OwnerID:      ownerID,
		Tree:         readerTree,
		ArticleCount: countVisibleArticles(readerTree),
		Cover:        cover,
	}, nil
}

// firstVisibleCover 取深度优先第一篇有封面的公开文章节点的封面。
func firstVisibleCover(items []outline.ViewNode) string {
	for _, item := range items {
		if item.Node.IsArticle() && item.ArticleCover != "" {
			return item.ArticleCover
		}
		if cover := firstVisibleCover(item.Children); cover != "" {
			return cover
		}
	}
	return ""
}

// countVisibleArticles 篇数只数文章节点（文件夹不计，失效节点已过滤）。
func countVisibleArticles(items []outline.ViewNode) int {
	count := 0
	for _, item := range items {
		if item.Node.IsArticle() {
			count++
		}
		count += countVisibleArticles(item.Children)
	}
	return count
}

func collectionReaderOutlineOwner(id collection.ID) (outline.OwnerKind, outline.ID) {
	return outline.OwnerCollection, outline.ID(id)
}

func projectReaderOutlineOwner(id project.ID) (outline.OwnerKind, outline.ID) {
	return outline.OwnerProject, outline.ID(id)
}
