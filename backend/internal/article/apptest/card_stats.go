package apptest

import (
	"context"
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// outlineCardStats 从目录树 + 文章 / 用户 fake 复算卡片的公开篇数与封面回退。
//
// 与生产读面共用 outline.ReaderView：过滤失效文章、只数公开文章、封面取
// DFS 第一个公开文章节点的封面，全部只有一份实现。
func outlineCardStats(
	ctx context.Context, outs *OutlineRepo, arts *ArticleRepo, users *Users,
	ownerKind outline.OwnerKind, ownerID ctype.ID, onlyAdminAuthors bool,
) (count int, cover string, err error) {
	if outs == nil {
		return 0, "", nil
	}
	nodes, err := outs.ListNodes(ctx, ownerKind, ownerID)
	if err != nil {
		return 0, "", err
	}
	tree, err := outline.Build(nodes)
	if err != nil {
		return 0, "", err
	}

	view, err := outline.ReaderView(tree, func(articleID outline.ArticleID) (outline.ReaderSnapshot, error) {
		if arts == nil {
			// 只测编排、不关心读模型时允许没有文章 fake：节点都按可见处理。
			return outline.ReaderSnapshot{Visible: true}, nil
		}
		a, err := arts.Find(ctx, domain.ArticleID(articleID))
		if errors.Is(err, domain.ErrArticleNotFound) {
			return outline.ReaderSnapshot{}, nil
		}
		if err != nil {
			return outline.ReaderSnapshot{}, err
		}
		visible := a.IsPubliclyVisible()
		if onlyAdminAuthors {
			if users == nil {
				visible = false
			} else {
				p, err := users.Profile(ctx, a.AuthorID())
				if err != nil {
					return outline.ReaderSnapshot{}, err
				}
				visible = visible && p.Admin
			}
		}
		return outline.ReaderSnapshot{Visible: visible, Title: a.Title(), Cover: a.Cover()}, nil
	})
	if err != nil {
		return 0, "", err
	}
	return view.ArticleCount, view.Cover, nil
}
