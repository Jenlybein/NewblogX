package river

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/es/articlesync"
)

// 每个 (表, 动作) 都要落到正确的重建事件上。
//
// 这张表就是"写入端与服务端的共识"：只要 article_search_projection.go 里换了调用方式，
// 这里必须一起改 —— 两处都写在同一条用例里，改一处漏一处会立刻红。
func TestProjectionReplayEventsMapping(t *testing.T) {
	cases := []struct {
		table  string
		action string
		want   []articlesync.ArticleSearchProjectionEventType
	}{
		{"article_models", "insert", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleUpsert}},
		// update 归一到 upsert：手里没有那份"只含变更列"的增量，全量重算更稳。
		{"article_models", "update", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleUpsert}},
		// delete 必须区分：用 upsert 会把刚删掉的文档又建回来。
		{"article_models", "delete", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleDelete}},
		// 用户行同时影响"作者快照"和"置顶作者名"，原链路就是连发两个，少一个会漏一半。
		{"user_models", "insert", []articlesync.ArticleSearchProjectionEventType{
			articlesync.ArticleSearchProjectionAuthorSnapshot,
			articlesync.ArticleSearchProjectionTopUserChanged,
		}},
		{"category_models", "update", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionCategorySnapshot}},
		{"tag_models", "update", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionTagSnapshot}},
		{"article_tag_models", "insert", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleTagsChanged}},
		{"user_top_article_models", "delete", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleTopChanged}},
		// 大小写/空格容错：payload 来自 JSON，不能假设写入端一定规范。
		{" Article_Models ", "DELETE", []articlesync.ArticleSearchProjectionEventType{articlesync.ArticleSearchProjectionArticleDelete}},
	}
	for _, c := range cases {
		got, err := projectionReplayEvents(c.table, c.action)
		if err != nil {
			t.Fatalf("%s/%s 不应报错: %v", c.table, c.action, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s/%s: 期望 %v，实际 %v", c.table, c.action, c.want, got)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("%s/%s: 第 %d 个事件期望 %s，实际 %s", c.table, c.action, i, c.want[i], got[i])
			}
		}
	}
}

// 不认识的表必须报错（普通 error），不能被当成 unsupported 糊过去：
// 那说明写入端与重投端的表清单对不上，是真故障，需要人看。
func TestProjectionReplayEventsRejectsUnknownTable(t *testing.T) {
	if _, err := projectionReplayEvents("comment_models", "insert"); err == nil {
		t.Fatal("未知表应报错")
	} else if errors.Is(err, deadletter.ErrUnsupported) {
		t.Fatal("未知表是真故障，不应包装成 ErrUnsupported")
	}
}

// 关联表/置顶表的行主键不是文章 ID，重算时必须取 article_id。
func TestProjectionIDColumn(t *testing.T) {
	cases := map[string]string{
		"article_models":          "id",
		"user_models":             "id",
		"category_models":         "id",
		"tag_models":              "id",
		"article_tag_models":      "article_id",
		"user_top_article_models": "article_id",
		"ARTICLE_TAG_MODELS":      "article_id",
	}
	for table, want := range cases {
		if got := projectionIDColumn(table); got != want {
			t.Fatalf("%s: 期望 %s，实际 %s", table, want, got)
		}
	}
}

// 旧记录（没有 ids）应当被明确判成"不支持重投"，并给出可执行的全量重建命令。
func TestProjectionReplayWithoutIDsIsUnsupported(t *testing.T) {
	r := NewProjectionReplayer(nil, nil)
	err := r.Replay(context.Background(), deadletter.Record{
		Stream:      ProjectionStream,
		CdcJobID:    "es_projection:db.article_models:bin.1:4:0",
		Action:      "update",
		PayloadJSON: `{"schema":"db","table":"article_models","action":"update","rowCount":3}`,
	})
	if err == nil {
		t.Fatal("没有 ids 的旧记录应报错")
	}
	if !errors.Is(err, deadletter.ErrUnsupported) {
		t.Fatalf("旧记录应包装 ErrUnsupported（不涨 retry_count），实际 %v", err)
	}
	if !strings.Contains(err.Error(), "-es -s article-sync") {
		t.Fatalf("应给出可执行的全量重建命令，实际 %q", err.Error())
	}
}

// 有 ids 但没有 db：报"未初始化"，而不是默默成功。
func TestProjectionReplayWithoutDBFails(t *testing.T) {
	r := NewProjectionReplayer(nil, nil)
	err := r.Replay(context.Background(), deadletter.Record{
		PayloadJSON: `{"schema":"db","table":"article_models","action":"insert","ids":["1"]}`,
	})
	if err == nil {
		t.Fatal("未注入数据库时应报错")
	}
}

// 空 payload 要报错，不要去猜。
func TestProjectionReplayRejectsEmptyPayload(t *testing.T) {
	r := NewProjectionReplayer(nil, nil)
	for _, raw := range []string{"", "{}", "{oops"} {
		err := r.Replay(context.Background(), deadletter.Record{PayloadJSON: raw})
		if err == nil {
			t.Fatalf("%q 应报错", raw)
		}
	}
}

// 保证 ids 里确实是 ctype.ID（JSON 里是字符串），避免有人把它改成整型切片。
func TestProjectionReplayPayloadIDsAreStrings(t *testing.T) {
	var payload projectionReplayPayload
	raw := `{"schema":"db","table":"article_models","action":"insert","ids":["1019","1011"]}`
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(payload.IDs) != 2 || payload.IDs[0] != ctype.NewID(1019) {
		t.Fatalf("ids 解析不对: %+v", payload.IDs)
	}
}
