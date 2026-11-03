package app

import (
	"errors"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/outline"
)

// snapshotCache 按文章 ID 缓存一次快照读取：同一篇文章只在整棵树上查一次仓储。
//
// 缓存口径与"节点指向的文章是否已不存在"是同一件事：
//   - `domain.ErrArticleNotFound` 表示文章真的没了，记为零值并缓存，由调用方
//     决定怎么摆这一行（后台标 missing、读者直接过滤掉）；
//   - 其它错误一律不缓存、原样上抛——数据库抖动不能被当成"文章没了"。
//
// 后台树（FindIncludingDeleted）与读者树（Find）的差异留在各自的 loader 里，
// 快照缓存与错误口径只有这一份实现（#224）。
type snapshotCache[V any] struct {
	values map[outline.ArticleID]V
	load   func(outline.ArticleID) (V, error)
}

func newSnapshotCache[V any](load func(outline.ArticleID) (V, error)) *snapshotCache[V] {
	return &snapshotCache[V]{values: make(map[outline.ArticleID]V), load: load}
}

// get 返回某篇文章的快照；同一 ID 第二次调用直接命中缓存。
func (c *snapshotCache[V]) get(articleID outline.ArticleID) (V, error) {
	if v, ok := c.values[articleID]; ok {
		return v, nil
	}
	var zero V
	v, err := c.load(articleID)
	if errors.Is(err, domain.ErrArticleNotFound) {
		c.values[articleID] = zero
		return zero, nil
	}
	if err != nil {
		return zero, err
	}
	c.values[articleID] = v
	return v, nil
}
