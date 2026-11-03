package articlesync

// 索引侧软删兜底（#199）：文档上的软删标记。
//
// #196 让删除本身"生效"（增量路径直接删文档、outbox 订阅者也会删）。
// 这一层兜底是在文档里记一个标记，查询侧用它把"本该被删、却因为中继位点
// 落后或投影进了死信而还留在索引里"的文档挡在读者面前。
//
// 标记由两个入口维护：
//  1. 全量重建的文档构造（快照路径）：已软删的行重建时必须带 is_deleted=true，
//     否则重建会把删掉的文章重新带进索引；
//  2. 增量路径：deleted_at 从 NULL 变成时间（软删）由 #196 直接删文档；
//     从时间变回 NULL（恢复）必须把标记清回 false —— 这一条最容易被漏掉，
//     因为恢复在旧实现里是"空 data → continue"，对索引零写入。
//
// 另外，标记缺失必须视为"未删除"（查询侧用 must_not，见 internal/search/app）：
// 字段引入之前写入的旧文档没有它，重建回填之前它们必须继续可见。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/test/testutil"
)

// TestArticleMappingHasSoftDeleteMarker 索引 mapping 必须声明软删标记字段。
//
// 不声明也不会报错：ES 会动态映射成 boolean，但那就是"mapping 与投影各说各话"，
// 而且旧索引重建时不会被回填。字段类型必须显式是 boolean。
func TestArticleMappingHasSoftDeleteMarker(t *testing.T) {
	var mapping struct {
		Mappings struct {
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal([]byte((ArticleRow{}).Mapping()), &mapping); err != nil {
		t.Fatalf("解析索引 mapping 失败: %v", err)
	}

	field, ok := mapping.Mappings.Properties[SoftDeleteMarkerField]
	if !ok {
		t.Fatalf("索引 mapping 缺少软删标记字段 %q", SoftDeleteMarkerField)
	}
	if field.Type != "boolean" {
		t.Fatalf("软删标记 %q 应为 boolean，实际 %q", SoftDeleteMarkerField, field.Type)
	}
}

// TestArticleDocumentCarriesSoftDeleteMarker 文档构造必须写这个标记。
//
// 全量重建走的是 buildArticleESDocumentFromSnapshot，所以这里直接断言文档形状。
// 未删除的行必须显式写 false（而不是省略）：读侧虽然容忍缺失，但显式 false
// 让"重建之后的文档全都带标记"这件事可观测。
func TestArticleDocumentCarriesSoftDeleteMarker(t *testing.T) {
	authorID := ctype.NewID(9)
	base := ArticleRowSnapshot{
		ID:               ctype.NewID(7),
		Title:            "标题",
		Content:          "正文",
		AuthorID:         authorID,
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityVisible,
	}

	live := base
	deletedAt := time.Now()
	base.DeletedAt = &deletedAt

	liveDoc := buildArticleESDocumentFromSnapshot(live, articleProjectionOptions{})
	if got, ok := liveDoc[SoftDeleteMarkerField].(bool); !ok || got {
		t.Fatalf("未删除的文章应写 %s=false, got=%#v", SoftDeleteMarkerField, liveDoc[SoftDeleteMarkerField])
	}

	deletedDoc := buildArticleESDocumentFromSnapshot(base, articleProjectionOptions{})
	if got, ok := deletedDoc[SoftDeleteMarkerField].(bool); !ok || !got {
		t.Fatalf("已软删的文章应写 %s=true, got=%#v", SoftDeleteMarkerField, deletedDoc[SoftDeleteMarkerField])
	}
}

// TestSyncESDocsIndexesSoftDeletedArticle 全量重建必须能索引已软删的行。
//
// 这条同时钉住两个回归：
//  1. 重建取的是"只选 id"的投影（不带 GORM 的软删作用域），已删文章也会进来；
//     若回源装载仍按软删作用域过滤，整批会报"部分文章未加载到"，重建直接失败。
//  2. 这些行既然被索引，就必须带 is_deleted=true —— 否则重建等于把删掉的文章
//     又塞回读者列表，兜底过滤形同虚设。
func TestSyncESDocsIndexesSoftDeletedArticle(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
		&cropRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	author := UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}

	live := ArticleRow{Title: "活着的文章", Content: "正文", AuthorID: author.ID, PublishStatus: enum.ArticleStatusPublished}
	deleted := ArticleRow{Title: "已删的文章", Content: "正文", AuthorID: author.ID, PublishStatus: enum.ArticleStatusPublished}
	if err := db.Create(&live).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	// GORM 软删：行还在，deleted_at 有值（这就是 binlog 里那次 UPDATE）。
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	docs := setupMockESCollectingBulkDocs(t)

	if err := SyncESDocs(db, testutil.ESClient(), []ctype.ID{live.ID, deleted.ID}); err != nil {
		t.Fatalf("全量重建不应因软删行而失败: %v", err)
	}
	if len(*docs) != 2 {
		t.Fatalf("活着的与已删的文章都应被索引, got=%d: %v", len(*docs), *docs)
	}

	markerByID := make(map[string]any, len(*docs))
	for _, doc := range *docs {
		markerByID[fmt.Sprint(doc["id"])] = doc[SoftDeleteMarkerField]
	}
	if got, ok := markerByID[live.ID.String()].(bool); !ok || got {
		t.Fatalf("活着的文章 %s 应写 %s=false, got=%#v", live.ID, SoftDeleteMarkerField, got)
	}
	if got, ok := markerByID[deleted.ID.String()].(bool); !ok || !got {
		t.Fatalf("已删的文章 %s 应写 %s=true, got=%#v", deleted.ID, SoftDeleteMarkerField, got)
	}
}

// TestUpdateESDocsByArticleDeltasClearsMarkerOnRestore 恢复（deleted_at→NULL）要把标记清回。
//
// 恢复在旧实现里是"deleted_at 不在可投影列里 → data 为空 → continue"，
// 也就是对索引零写入：标记会永远停在 true，这篇文章从此在任何列表里都出不来。
//
// 这里模拟真实次序：删除已经把文档从索引里删掉，于是恢复先是一次
// update（404 → 文档缺失）→ 走既有的"缺失文档回源重建"；最终索引里的文档
// 必须是 is_deleted=false 且标题仍在。
func TestUpdateESDocsByArticleDeltasClearsMarkerOnRestore(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
		&cropRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	author := UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := ArticleRow{Title: "恢复的文章", Content: "正文", AuthorID: author.ID, PublishStatus: enum.ArticleStatusPublished}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Delete(&article).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}
	// 恢复：deleted_at 改回 NULL（binlog 里就是这次 UPDATE）。
	if err := db.Unscoped().Model(&ArticleRow{}).
		Where("id = ?", article.ID).
		Update("deleted_at", nil).Error; err != nil {
		t.Fatalf("恢复文章失败: %v", err)
	}

	bulkBodies := make([]string, 0, 2)
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			payload := string(body)
			bulkBodies = append(bulkBodies, payload)
			if strings.Contains(payload, "\"update\"") {
				// 删除已经生效：文档在索引里不存在。
				writeJSON(w, 200, fmt.Sprintf(`{"took":1,"errors":true,"items":[{"update":{"_id":"%s","status":404,"error":{"type":"document_missing_exception","reason":"missing"}}}]}`, article.ID.String()))
				return
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[{"index":{"_id":"ok","status":201}}]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	if err := UpdateESDocsByArticleDeltas(db, testutil.ESClient(), []ArticleModelDelta{{
		ArticleID: article.ID,
		Changed:   map[string]any{"deleted_at": nil},
	}}); err != nil {
		t.Fatalf("恢复增量不应报错: %v", err)
	}
	if len(bulkBodies) != 2 {
		t.Fatalf("恢复应先清标记、再回源重建, got=%d: %v", len(bulkBodies), bulkBodies)
	}
	if !strings.Contains(bulkBodies[0], "\"update\"") || !strings.Contains(bulkBodies[0], "\"is_deleted\":false") {
		t.Fatalf("恢复的第一轮应把标记清回 false: %s", bulkBodies[0])
	}
	if !strings.Contains(bulkBodies[1], "\"index\"") {
		t.Fatalf("文档缺失后应回源重建: %s", bulkBodies[1])
	}
	// 回源重建的完整文档同样不能带"已删"标记。
	var rebuilt map[string]any
	if err := json.Unmarshal([]byte(bulkBodies[1][strings.Index(bulkBodies[1], "\n")+1:]), &rebuilt); err != nil {
		t.Fatalf("解析重建文档失败: %v (%s)", err, bulkBodies[1])
	}
	if got, ok := rebuilt[SoftDeleteMarkerField].(bool); !ok || got {
		t.Fatalf("恢复后重建的文档应写 %s=false, got=%#v", SoftDeleteMarkerField, rebuilt[SoftDeleteMarkerField])
	}
}
