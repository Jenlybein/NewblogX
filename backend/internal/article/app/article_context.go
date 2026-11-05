package app

import (
	"context"
	"errors"
	"strings"

	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
)

// 本文件保留阅读页 `?from=` 来路的解析与树内相邻文章的纯规则。
//
// 旧共享端点 `GET /articles/{id}/outline-context` 已随 #277 收缩下线：它没有
// 归属信息，只能反查所有树再挑第一棵。现在阅读页左栏只经两个具名树端点
// （`GET /collections|/projects/{id}/outline-tree?from=...`）拿同一份读模型，
// 归属由路径一次给出，不再有"无来源时猜一棵树"的兼容分支。

// ArticleOutlineSource 是阅读页 `?from=` 解析出来的归属树。
type ArticleOutlineSource struct {
	OwnerKind outline.OwnerKind
	OwnerID   outline.ID
}

// ParseArticleOutlineSource 解析 `<kind>:<id>`（如 `project:375`）。
//
// 不完整、kind 不认识、ID 为空都视为**没有来源**：格式校验只在这里做一处。
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

// readerOutlineOwnerVisible 判断一棵树的归属实体对读者是否可见。
//
// 与读者详情页同一口径：隐藏的合集/项目按不存在处理，不因为树里挂着文章而泄露。
func readerOutlineOwnerVisible(ctx context.Context, r treeRepos, src ArticleOutlineSource) (bool, error) {
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
