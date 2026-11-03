//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件是目录树（content_outline_node_models）的持久化契约。
//
// 只跑真 MySQL：要验的正是 SQL 行为——唯一键（含 NULL article_id 占位）、
// 软删行恢复、按 owner 隔离、列名契约。SQLite 上唯一索引与 NULL 语义不一样。

var outlineBaseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func migrateOutlines(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.Outline)
	return db
}

func newOutlineFolder(t *testing.T, id, parentID uint64, title string, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(900),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindFolder,
		Title:     title,
		SortOrder: order,
		Now:       outlineBaseTime,
	})
	if err != nil {
		t.Fatalf("构造文件夹失败: %v", err)
	}
	return n
}

func newOutlineArticleNode(t *testing.T, id, parentID, articleID uint64, order int) *outline.Node {
	t.Helper()
	n, err := outline.New(outline.NewParams{
		ID:        ctype.NewID(id),
		OwnerKind: outline.OwnerCollection,
		OwnerID:   ctype.NewID(900),
		ParentID:  ctype.NewID(parentID),
		Kind:      outline.NodeKindArticle,
		ArticleID: ctype.NewID(articleID),
		SortOrder: order,
		Now:       outlineBaseTime,
	})
	if err != nil {
		t.Fatalf("构造文章节点失败: %v", err)
	}
	return n
}

// TestOutlineRowsMatchSchema 逐列比对：adapter 的行结构与真表列一致。
func TestOutlineRowsMatchSchema(t *testing.T) {
	t.Parallel()

	db := migrateOutlines(t)
	schema.AssertColumns(t, db, schema.OutlineNodeRow{})
}

// TestOutlineRepoRoundTripAndUniqueArticle 覆盖写入、读回、唯一约束与软删恢复。
func TestOutlineRepoRoundTripAndUniqueArticle(t *testing.T) {
	t.Parallel()

	db := migrateOutlines(t)
	repo := mysql.NewOutlineRepo(db)
	ctx := context.Background()

	folder := newOutlineFolder(t, 1, 0, "第一章", 1)
	child := newOutlineArticleNode(t, 2, 1, 11, 1)
	empty := newOutlineFolder(t, 3, 0, "空章节", 2)
	root := newOutlineArticleNode(t, 4, 0, 12, 3)
	if err := repo.Create(ctx, []*outline.Node{folder, child, empty, root}); err != nil {
		t.Fatalf("插入节点失败: %v", err)
	}

	nodes, err := repo.ListNodes(ctx, outline.OwnerCollection, ctype.NewID(900))
	if err != nil {
		t.Fatalf("读取节点失败: %v", err)
	}
	if len(nodes) != 4 {
		t.Fatalf("应读到 4 个节点，实际 %d", len(nodes))
	}

	tree, err := outline.Build(nodes)
	if err != nil {
		t.Fatalf("组装树失败: %v", err)
	}
	if len(tree) != 2 || len(tree[0].Children) != 1 || len(tree[1].Children) != 0 {
		t.Fatalf("嵌套结构不对: %+v", tree)
	}

	// 唯一约束：同一棵树内同一篇文章不能有第二个节点。
	dup := newOutlineArticleNode(t, 5, 0, 11, 9)
	if err := repo.Create(ctx, []*outline.Node{dup}); err == nil {
		t.Fatal("同树内重复文章应撞唯一键")
	} else if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("应返回唯一键错误，实际 %v", err)
	}

	// 跨树（另一个 owner）允许同一篇文章。
	otherNode, err := outline.New(outline.NewParams{
		ID: ctype.NewID(6), OwnerKind: outline.OwnerProject, OwnerID: ctype.NewID(700),
		Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11), SortOrder: 1, Now: outlineBaseTime,
	})
	if err != nil {
		t.Fatalf("构造跨树节点失败: %v", err)
	}
	if err := repo.Create(ctx, []*outline.Node{otherNode}); err != nil {
		t.Fatalf("跨树重复文章应允许: %v", err)
	}

	// 软删 + 按文章找回（含软删行）+ 恢复原行。
	if err := repo.SoftDelete(ctx, []outline.ID{child.ID()}, outlineBaseTime); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	found, err := repo.FindByArticle(ctx, outline.OwnerCollection, ctype.NewID(900), ctype.NewID(11))
	if err != nil {
		t.Fatalf("软删后应能按文章找回: %v", err)
	}
	if !found.Deleted() {
		t.Fatal("找回的应是软删行")
	}
	found.Restore(outlineBaseTime)
	found.MoveTo("", 9, outlineBaseTime)
	if err := repo.Update(ctx, []*outline.Node{found}); err != nil {
		t.Fatalf("恢复软删行失败: %v", err)
	}
	active, err := repo.ListNodes(ctx, outline.OwnerCollection, ctype.NewID(900))
	if err != nil {
		t.Fatalf("恢复后读取失败: %v", err)
	}
	for _, n := range active {
		if n.ID() == child.ID() && n.Deleted() {
			t.Fatal("恢复后不应再是软删行")
		}
	}
}

// TestOutlineRepoScopesByOwner 断言一张树表不会把合集与项目串在一起。
func TestOutlineRepoScopesByOwner(t *testing.T) {
	t.Parallel()

	db := migrateOutlines(t)
	repo := mysql.NewOutlineRepo(db)
	ctx := context.Background()

	collectionNode := newOutlineFolder(t, 1, 0, "合集章节", 1)
	projectNode, err := outline.New(outline.NewParams{
		ID: ctype.NewID(2), OwnerKind: outline.OwnerProject, OwnerID: ctype.NewID(700),
		Kind: outline.NodeKindFolder, Title: "项目章节", SortOrder: 1, Now: outlineBaseTime,
	})
	if err != nil {
		t.Fatalf("构造项目节点失败: %v", err)
	}
	if err := repo.Create(ctx, []*outline.Node{collectionNode, projectNode}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	cNodes, err := repo.ListNodes(ctx, outline.OwnerCollection, ctype.NewID(900))
	if err != nil {
		t.Fatalf("读合集树失败: %v", err)
	}
	if len(cNodes) != 1 || cNodes[0].Title() != "合集章节" {
		t.Fatalf("合集树串到了项目节点: %+v", cNodes)
	}

	_, err = repo.FindByArticle(ctx, outline.OwnerCollection, ctype.NewID(900), ctype.NewID(11))
	if !errors.Is(err, outline.ErrNodeMissing) {
		t.Fatalf("没有文章节点时应返回 ErrNodeMissing，实际 %v", err)
	}
}

// TestOutlineRepoListsByArticleAcrossTrees 是阅读页"无来源回退到第一棵归属树"的
// SQL 契约：按文章反查所有树，**排除软删行**，顺序固定为 (owner_kind, owner_id, id)。
func TestOutlineRepoListsByArticleAcrossTrees(t *testing.T) {
	t.Parallel()

	db := migrateOutlines(t)
	repo := mysql.NewOutlineRepo(db)
	ctx := context.Background()

	nodeOf := func(t *testing.T, id uint64, ownerKind outline.OwnerKind, ownerID, articleID uint64) *outline.Node {
		t.Helper()
		n, err := outline.New(outline.NewParams{
			ID: ctype.NewID(id), OwnerKind: ownerKind, OwnerID: ctype.NewID(ownerID),
			Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(articleID),
			SortOrder: 1, Now: outlineBaseTime,
		})
		if err != nil {
			t.Fatalf("构造节点失败: %v", err)
		}
		return n
	}

	// 合集 900 / 合集 901 / 项目 700 都挂着文章 11；项目 700 里的另一篇是 12。
	c1 := nodeOf(t, 1, outline.OwnerCollection, 900, 11)
	c2 := nodeOf(t, 2, outline.OwnerCollection, 901, 11)
	p1 := nodeOf(t, 3, outline.OwnerProject, 700, 11)
	p2 := nodeOf(t, 4, outline.OwnerProject, 700, 12)
	if err := repo.Create(ctx, []*outline.Node{c1, c2, p1, p2}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	// 软删同一篇文章的另一个节点：回退时不能让一篇已移除的收录冒充归属树。
	gone, err := outline.New(outline.NewParams{
		ID: ctype.NewID(5), OwnerKind: outline.OwnerCollection, OwnerID: ctype.NewID(902),
		Kind: outline.NodeKindArticle, ArticleID: ctype.NewID(11), SortOrder: 1, Now: outlineBaseTime,
	})
	if err != nil {
		t.Fatalf("构造软删节点失败: %v", err)
	}
	if err := repo.Create(ctx, []*outline.Node{gone}); err != nil {
		t.Fatalf("插入软删节点失败: %v", err)
	}
	if err := repo.SoftDelete(ctx, []outline.ID{gone.ID()}, outlineBaseTime); err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	got, err := repo.ListByArticle(ctx, ctype.NewID(11))
	if err != nil {
		t.Fatalf("按文章反查失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("应在 3 棵活跃树里找到文章 11（软删树不算），实际 %d: %+v", len(got), got)
	}
	want := []struct {
		kind  outline.OwnerKind
		owner uint64
	}{
		{outline.OwnerCollection, 900},
		{outline.OwnerCollection, 901},
		{outline.OwnerProject, 700},
	}
	for i, w := range want {
		if got[i].OwnerKind() != w.kind || got[i].OwnerID() != ctype.NewID(w.owner) {
			t.Fatalf("第 %d 棵树顺序不对: %v/%s", i, got[i].OwnerKind(), got[i].OwnerID())
		}
	}

	none, err := repo.ListByArticle(ctx, ctype.NewID(99))
	if err != nil {
		t.Fatalf("按文章反查失败: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("没有归属的文章应返回空列表，实际 %+v", none)
	}
}
