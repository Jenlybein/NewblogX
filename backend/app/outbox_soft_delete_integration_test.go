//go:build integration

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/outboxrelay"
	es_service "myblogx/internal/platform/es"
	"myblogx/internal/platform/es/articlesync"
	"myblogx/internal/testkit/escontainer"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"github.com/elastic/go-elasticsearch/v7"
)

// 本用例补上规格 Testing Decisions 3 的"真实数据库 + 真实索引"组合：
//
//	写文章（真 MySQL）→ 建真实索引文档（真 ES）→ 软删（deleted_at UPDATE）
//	→ 显式驱动一次发件箱中继 → 查 ES，文档必须不存在。
//
// 既有 outbox_deleted_test.go 用 sqlite + mock ES，只证明"中继会发出 delete
// 动作"；它不能证明真实 ES 真的把文档删掉，也不能证明 MySQL 的软删 UPDATE
// 与中继读取链路合起来成立。缺的正是这一段。
//
// 运行：BLOGX_MYSQL_ADDR=127.0.0.1:3306 go test -tags integration ./app -run TestSoftDeleteRemovesRealESDoc -count=1
func TestSoftDeleteRemovesRealESDocViaOutboxRelay(t *testing.T) {
	// 不 t.Parallel：articlesync 的索引名是装配期注入的包级配置，
	// 与其它真实索引用例并行会互相改索引名。
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.ArticleCore, schema.Outbox)

	esAddr := escontainer.Addr(t)
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{esAddr}})
	if err != nil {
		t.Fatalf("创建真实 ES 客户端失败: %v", err)
	}

	index := fmt.Sprintf("blogx_it_article_%d", time.Now().UnixNano())
	articlesync.SetIndexName(index)
	// 不套用生产 mapping：生产 mapping 依赖 IK 分析与 ingest pipeline，
	// 测试容器不装这些插件。这里只需要一个能存文档、能按 _id 删除的索引。
	const testMapping = `{
		"settings": {"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": {"properties": {
			"id": {"type": "keyword"},
			"title": {"type": "text"},
			"is_deleted": {"type": "boolean"}
		}}
	}`
	if err := es_service.EnsureIndex(client, index, testMapping); err != nil {
		t.Fatalf("建真实测试索引失败: %v", err)
	}

	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	const (
		authorID  = uint64(9001)
		articleID = uint64(9101)
		outboxID  = uint64(9201)
	)
	schema.InsertUser(t, db, schema.UserSpec{
		ID: authorID, Username: "it_author", Nickname: "作者", Password: "x", Role: schema.RoleUser,
		CreatedAt: at, UpdatedAt: at,
	})
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID: articleID, Title: "真实索引软删用例", Abstract: "摘要", Content: "# 正文\n\n用于验证删除链路。",
		AuthorID: authorID, PublishStatus: schema.ArticleStatusPublished, Visibility: schema.VisibilityVisible,
		CommentsToggle: true, CreatedAt: at, UpdatedAt: at,
	})

	// 1) 写文章后先真的进索引：这是"删除前文档存在"的前提。
	articleCtypeID := ctype.NewID(articleID)
	if err := articlesync.SyncESDocs(db, client, []ctype.ID{articleCtypeID}); err != nil {
		t.Fatalf("真实 ES 建文档失败: %v", err)
	}
	if !esDocExists(t, client, index, articleCtypeID.String()) {
		t.Fatalf("建文档后 ES 里应有 %s", articleCtypeID)
	}

	// 2) 软删：库里的行还在，只有 deleted_at 被 UPDATE。
	//    这正是旧实现漏掉的那条 binlog 形态（不是 DELETE 行）。
	if err := db.Model(&schema.ArticleRow{}).
		Where("id = ?", articleCtypeID).
		Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}
	var softDeleted schema.ArticleRow
	if err := db.Unscoped().Where("id = ?", articleCtypeID).Take(&softDeleted).Error; err != nil {
		t.Fatalf("软删后应仍能读到行: %v", err)
	}
	if !softDeleted.DeletedAt.Valid {
		t.Fatal("软删应写入 deleted_at；行被物理删了，用例前提不成立")
	}

	// 3) 发件箱事件（生产里由删除事务写入，这里显式落一行），再显式驱动一次中继。
	outbox := &schema.OutboxRow{
		BaseRow:       schema.BaseRow{ID: ctype.NewID(outboxID), CreatedAt: at, UpdatedAt: at},
		EventName:     articleapp.EventArticleDeleted,
		AggregateType: "article",
		AggregateID:   articleID,
		Payload:       "{}",
		Status:        outboxrelay.OutboxStatusPending,
		AvailableAt:   time.Now().Add(-time.Second),
	}
	if err := db.Create(outbox).Error; err != nil {
		t.Fatalf("写入删除事件失败: %v", err)
	}

	relay := outboxrelay.New(db, testutil.Logger(), outboxrelay.Options{})
	t.Cleanup(relay.Close)
	registerOutboxSubscribers(relay, &Infra{DB: db, Logger: testutil.Logger(), ESClient: client})
	if err := relay.Dispatch(context.Background(), outbox.ID); err != nil {
		t.Fatalf("中继投递 article.deleted 失败: %v", err)
	}

	var dispatched schema.OutboxRow
	if err := db.Where("id = ?", outbox.ID).Take(&dispatched).Error; err != nil {
		t.Fatalf("读取投递后的事件行失败: %v", err)
	}
	if dispatched.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("删除事件投递后应为 done，实际 %s（last_error=%q）", dispatched.Status, dispatched.LastError)
	}

	// 4) 真实 ES：文档必须已经不存在。
	if esDocExists(t, client, index, articleCtypeID.String()) {
		t.Fatalf("软删并驱动中继后，ES 里仍能查到文档 %s", articleCtypeID)
	}
}

// esDocExists 用真实 ES 的 GET 判断文档是否存在；404 是"已删除"的正常形态。
func esDocExists(t *testing.T, client *elasticsearch.Client, index, id string) bool {
	t.Helper()

	res, err := client.Get(index, id)
	if err != nil {
		t.Fatalf("查询 ES 文档失败: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == 404 {
		return false
	}
	if res.IsError() {
		t.Fatalf("查询 ES 文档返回错误 %s: %s", res.Status(), res.String())
	}
	return true
}
