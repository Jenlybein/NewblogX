package app

import (
	"context"
	"errors"
	"strings"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
)

// 本文件是「文章上下文」用例：从阅读页 `?from=` 找回同一棵读者树，并回答
// 当前节点、按 DFS 顺序的上一篇 / 下一篇。
//
// 树**不另写一份 SQL**：这里直接复用 readerOutline 的组装（只留公开文章节点、
// 空文件夹保留、可见性沿用合集/项目既有规则），本文件只做"选哪棵树"与
// "在树里找相邻文章节点"两件纯编排的事。

// ArticleOutlineSource 是阅读页 `?from=` 解析出来的归属树。
type ArticleOutlineSource struct {
	OwnerKind outline.OwnerKind
	OwnerID   outline.ID
}

// ParseArticleOutlineSource 解析 `<kind>:<id>`（如 `project:375`）。
//
// 不完整、kind 不认识、ID 为空都视为**没有来源**：格式校验只在这里做一处，
// 调用方（用例）据此决定"回退到第一棵归属树"还是"只认这一棵"。
func ParseArticleOutlineSource(raw string) (ArticleOutlineSource, bool) {
	trimmed := strings.TrimSpace(raw)
	kindPart, idPart, ok := strings.Cut(trimmed, ":")
	if !ok {
		return ArticleOutlineSource{}, false
	}
	id := outline.ID(strings.TrimSpace(idPart))
	if id.IsZero() {
		return ArticleOutlineSource{}, false
	}
	var kind outline.OwnerKind
	switch kindPart {
	case "collection":
		kind = outline.OwnerCollection
	case "project":
		kind = outline.OwnerProject
	default:
		return ArticleOutlineSource{}, false
	}
	return ArticleOutlineSource{OwnerKind: kind, OwnerID: id}, true
}

// ArticleOutlineContext 是阅读页左栏的读模型。
//
// HasTree=false 表示"这篇文章没有可展示的归属树"：页面据此不渲染左栏，
// 其余字段为空值。Current/Previous/Next 只在 HasTree=true 时非空（Previous
// 与 Next 可以各自为空：第一篇没有上一篇，最后一篇没有下一篇）。
type ArticleOutlineContext struct {
	HasTree   bool
	OwnerKind outline.OwnerKind
	OwnerID   outline.ID
	Tree      []outline.ViewNode
	Current   *outline.ViewNode
	Previous  *outline.ViewNode
	Next      *outline.ViewNode
}

// ArticleOutlineContext 返回文章所在树、当前节点与相邻文章节点（读者侧公开读）。
//
// 来源优先：`from` 指向哪棵树就只认那一棵（一篇文章同时属于多个专题时，读者
// 只看自己点进来的那棵，不串专题）。`from` 缺失/不合法时回退到第一棵归属树；
// 都没有就返回 HasTree=false 的空结果（页面左栏不出现）。
func (s *Service) ArticleOutlineContext(
	ctx context.Context, articleID domain.ArticleID, from string, onlyAdminAuthors bool,
) (*ArticleOutlineContext, error) {
	var out *ArticleOutlineContext
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		out = &ArticleOutlineContext{}
		if articleID.IsZero() {
			return nil
		}

		src, hasSource := ParseArticleOutlineSource(from)
		if hasSource {
			// 来源也按读者可见性判定：不能因为 URL 里带了 ID 就打开隐藏专题的树。
			visible, err := readerOutlineOwnerVisible(ctx, r, src)
			if err != nil {
				return err
			}
			if !visible {
				return nil
			}
			context, err := s.articleContextForOwner(ctx, r, articleID, src, onlyAdminAuthors)
			if err != nil {
				return err
			}
			if context != nil {
				*out = *context
			}
			return nil
		}

		// 没有来源：按文章反查它出现在哪些树里，取第一棵**读者可见**且
		// 这篇文章在树里仍可见的树（隐藏合集/项目、被过滤的失效节点都跳过）。
		nodes, err := r.Outlines().ListByArticle(ctx, outline.ArticleID(articleID))
		if err != nil {
			return err
		}
		seen := make(map[ArticleOutlineSource]bool, len(nodes))
		for _, n := range nodes {
			src := ArticleOutlineSource{OwnerKind: n.OwnerKind(), OwnerID: n.OwnerID()}
			if seen[src] {
				continue
			}
			seen[src] = true

			visible, err := readerOutlineOwnerVisible(ctx, r, src)
			if err != nil {
				return err
			}
			if !visible {
				continue
			}
			context, err := s.articleContextForOwner(ctx, r, articleID, src, onlyAdminAuthors)
			if err != nil {
				return err
			}
			if context != nil {
				*out = *context
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// articleContextForOwner 组装某一棵树的上下文；当前文章在这棵树里不可见时返回 nil。
//
// 树必须复用 readerOutline：如果这里自己再组装一遍，失效行过滤、空文件夹保留、
// 「显示非管理员发布文章」开关会立刻长出第二份会漂移的真相。
func (s *Service) articleContextForOwner(
	ctx context.Context, r Repos, articleID domain.ArticleID,
	src ArticleOutlineSource, onlyAdminAuthors bool,
) (*ArticleOutlineContext, error) {
	tree, err := s.readerOutline(ctx, r, src.OwnerKind, src.OwnerID, onlyAdminAuthors, "")
	if err != nil {
		return nil, err
	}
	current, previous, next := articleNeighbours(tree.Tree, outline.ArticleID(articleID))
	if current == nil {
		return nil, nil
	}
	return &ArticleOutlineContext{
		HasTree:   true,
		OwnerKind: src.OwnerKind,
		OwnerID:   src.OwnerID,
		Tree:      tree.Tree,
		Current:   current,
		Previous:  previous,
		Next:      next,
	}, nil
}

// readerOutlineOwnerVisible 判断一棵树的归属实体对读者是否可见。
//
// 与读者详情页同一口径：隐藏的合集/项目按不存在处理，不因为树里挂着文章而泄露。
func readerOutlineOwnerVisible(ctx context.Context, r Repos, src ArticleOutlineSource) (bool, error) {
	switch src.OwnerKind {
	case outline.OwnerCollection:
		c, err := r.Collections().Find(ctx, collection.ID(src.OwnerID))
		if errors.Is(err, collection.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return c.Displayed(), nil
	case outline.OwnerProject:
		p, err := r.Projects().Find(ctx, project.ID(src.OwnerID))
		if errors.Is(err, project.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return p.Displayed(), nil
	default:
		return false, nil
	}
}

// articleNeighbours 在读者树里按 DFS 顺序找这篇文章的节点与相邻文章节点。
//
// 只把**文章节点**排成序列：文件夹和空文件夹不参与上一篇/下一篇；跨文件夹时
// 相邻关系仍然成立（文档式阅读顺序）。
func articleNeighbours(
	tree []outline.ViewNode, articleID outline.ArticleID,
) (current, previous, next *outline.ViewNode) {
	var articles []outline.ViewNode
	var walk func(items []outline.ViewNode)
	walk = func(items []outline.ViewNode) {
		for _, item := range items {
			if item.Node.IsArticle() {
				articles = append(articles, item)
			}
			walk(item.Children)
		}
	}
	walk(tree)

	for i := range articles {
		if articles[i].Node.ArticleID() != articleID {
			continue
		}
		current = &articles[i]
		if i > 0 {
			previous = &articles[i-1]
		}
		if i+1 < len(articles) {
			next = &articles[i+1]
		}
		return current, previous, next
	}
	return nil, nil, nil
}
