//go:build integration

package mysql_test

import (
	"context"
	"testing"

	"myblogx/internal/kernel/ctype"
	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/site/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是右栏「推荐合集」数据源的持久化契约。
//
// #223 起，合集卡片的篇数与封面回退由 article 模块的目录树读模型提供（经
// SidebarCollectionReader 端口注入）：站点模块不再直连 collection_article_models，
// 也不自己实现 DFS 封面回退。所以这里验证的是"右栏把请求原样交给读模型端口、
// 并把返回项映射进右栏形状"，而不是站点侧的 SQL。

// stubCollectionReader 记录收到的条数上限并返回固定条目。
type stubCollectionReader struct {
	items []domain.SidebarCollection
	limit int
	calls int
}

func (s *stubCollectionReader) RecommendedCollections(
	_ context.Context, limit int,
) ([]domain.SidebarCollection, error) {
	s.calls++
	s.limit = limit
	return s.items, nil
}

// TestSidebarDataCollectionsDelegatesToReader 右栏合集卡把（收敛后的）条数上限
// 交给 article 模块的读模型，并原样返回它的条目。
func TestSidebarDataCollectionsDelegatesToReader(t *testing.T) {
	t.Parallel()

	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Collection, schema.Outline)

	reader := &stubCollectionReader{items: []domain.SidebarCollection{{
		ID: ctype.NewID(1), Title: "从零搭博客", Description: "简介",
		Cover: "https://cdn.example.com/first.png", ArticleCount: 2,
	}}}
	data := sitemysql.NewSidebarData(db, reader)

	got, err := data.Collections(context.Background(), 0)
	if err != nil {
		t.Fatalf("读取推荐合集失败: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("应把请求交给读模型端口，实际调用 %d 次", reader.calls)
	}
	if reader.limit != domain.SidebarCardDataLimit {
		t.Fatalf("limit=0 应收敛到右栏默认上限 %d，实际 %d", domain.SidebarCardDataLimit, reader.limit)
	}
	if len(got) != 1 || got[0].ID.Uint64() != 1 || got[0].ArticleCount != 2 ||
		got[0].Cover != "https://cdn.example.com/first.png" {
		t.Fatalf("右栏应原样返回读模型的卡片: %+v", got)
	}
}

// TestSidebarDataCollectionsWithoutReader 没装配合集读模型时该卡为空，
// 不影响其它卡片与页面（不报错）。
func TestSidebarDataCollectionsWithoutReader(t *testing.T) {
	t.Parallel()

	data := sitemysql.NewSidebarData(nil, nil)
	got, err := data.Collections(context.Background(), domain.SidebarCardDataLimit)
	if err != nil {
		t.Fatalf("未装配读模型时不应报错: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("未装配读模型时应返回空列表，实际 %+v", got)
	}
}
