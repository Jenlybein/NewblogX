package app

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
)

// 本文件是「读者侧目录树」的用例：合集与项目共一份读模型组装。
//
// 与后台具名树读面的区别：读者树只保留**公开文章节点**，文件夹保留（包括
// 空文件夹）；失效文章节点不出现，篇数只数公开文章；没上传封面时回退到深度
// 优先第一个公开文章节点的封面（该节点没有封面时就是空）。展示开关与
// 「显示非管理员发布文章」开关的判定仍在各自实体的用例里，这里不另写一份
// 可见性规则。

// ReaderOutlineDetail 是读者详情页的读模型。
type ReaderOutlineDetail struct {
	OwnerKind    outline.OwnerKind
	OwnerID      outline.ID
	Tree         []outline.ViewNode
	ArticleCount int
	// Cover 是最终生效封面：显式封面优先，空则取 DFS 第一个公开文章节点的封面。
	Cover string
}

// readerOutline 组装读者树：复用 #217 的 outline.Build + 文章快照读逻辑，
// 不另开一套 SQL，也不读写已冻结的扁平收录中间表。
func (s *Service) readerOutline(
	ctx context.Context, r treeRepos, ownerKind outline.OwnerKind, ownerID outline.ID,
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
		// 与首页列表、搜索、详情同一口径（ADR-0015 决策 4）。
		authorIDs := make([]domain.UserID, 0)
		seen := make(map[domain.UserID]bool)
		for _, n := range nodes {
			if !n.IsArticle() {
				continue
			}
			a, err := r.Articles().Find(ctx, domain.ArticleID(n.ArticleID()))
			if errors.Is(err, domain.ErrArticleNotFound) {
				// 节点指向的文章已不存在：快照阶段按"不可见"把它过滤掉。
				continue
			}
			if err != nil {
				return nil, err
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

	snaps := newSnapshotCache(func(articleID outline.ArticleID) (outline.ReaderSnapshot, error) {
		a, err := r.Articles().Find(ctx, domain.ArticleID(articleID))
		if err != nil {
			return outline.ReaderSnapshot{}, err
		}
		visible := a.IsPubliclyVisible()
		if onlyAdminAuthors && !profiles[a.AuthorID()].Admin {
			visible = false
		}
		return outline.ReaderSnapshot{
			Visible: visible,
			Title:   a.Title(),
			Cover:   a.Cover(),
		}, nil
	})

	view, err := outline.ReaderView(tree, snaps.get)
	if err != nil {
		return nil, err
	}

	// 显式封面优先；为空时取 DFS 第一个公开文章节点的封面（它为空就是空）。
	cover := explicitCover
	if cover == "" {
		cover = view.Cover
	}
	return &ReaderOutlineDetail{
		OwnerKind:    ownerKind,
		OwnerID:      ownerID,
		Tree:         view.Tree,
		ArticleCount: view.ArticleCount,
		Cover:        cover,
	}, nil
}
