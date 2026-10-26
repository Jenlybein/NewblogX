package river

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/platform/cdc/deadletter"
)

// payload 能原样还原成一条 index 请求。
func TestBuildReplayRequestIndex(t *testing.T) {
	req, err := buildReplayRequest(deadletter.Record{
		Action:      "insert",
		CdcJobID:    "es_river:db.article_models:bin.1:4:0",
		PayloadJSON: `{"index":"article_index","type":"_doc","id":"123","pipeline":"p1","data":{"title":"hi"}}`,
	})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if req.Action != "index" || req.Index != "article_index" || req.ID != "123" || req.Pipeline != "p1" {
		t.Fatalf("还原结果不对: %+v", req)
	}
	if req.Data["title"] != "hi" {
		t.Fatalf("文档内容丢了: %+v", req.Data)
	}
}

// delete 动作要还原成 ES 的 delete，且不要求有 data。
func TestBuildReplayRequestDelete(t *testing.T) {
	req, err := buildReplayRequest(deadletter.Record{
		Action:      "delete",
		CdcJobID:    "es_river:db.article_models:bin.1:4:0",
		PayloadJSON: `{"index":"article_index","id":"123"}`,
	})
	if err != nil {
		t.Fatalf("删除动作不应因为没有 data 报错: %v", err)
	}
	if req.Action != "delete" {
		t.Fatalf("应为 delete，实际 %s", req.Action)
	}
}

// 缺关键字段要明确报错，而不是拼出一个会被 ES 拒绝的请求。
func TestBuildReplayRequestRejectsIncomplete(t *testing.T) {
	cases := map[string]deadletter.Record{
		"空 payload":    {PayloadJSON: ""},
		"{} payload":   {PayloadJSON: "{}"},
		"非法 JSON":      {PayloadJSON: "{oops"},
		"缺 index":      {PayloadJSON: `{"id":"1","data":{"a":1}}`},
		"缺文档 ID":       {PayloadJSON: `{"index":"i","data":{"a":1}}`},
		"index 缺 data": {PayloadJSON: `{"index":"i","id":"1"}`},
	}
	for name, record := range cases {
		if _, err := buildReplayRequest(record); err == nil {
			t.Fatalf("%s: 期望报错，实际通过", name)
		}
	}
}

// 没有 ES 客户端时必须明确失败，不能静默当成功（那会让死信被误标 resolved）。
func TestReplayWithoutClientFails(t *testing.T) {
	r := NewESReplayer(nil)
	if err := r.Replay(context.Background(), deadletter.Record{
		PayloadJSON: `{"index":"i","id":"1","data":{"a":1}}`,
	}); err == nil {
		t.Fatal("未注入 ES 客户端时应报错")
	}
}

// Streams 声明要与死信表里落的值一致（改了这里就查不到对应的执行器）。
func TestESReplayerStreams(t *testing.T) {
	got := NewESReplayer(nil).Streams()
	if len(got) != 1 || !strings.EqualFold(got[0], ESStream) {
		t.Fatalf("stream 声明不对: %v", got)
	}
}

// 改文档 ID 的 update 会拆成"删旧 + 写新"两条请求，两者的 canal 动作都是 update。
// 删除那条没有 data —— 必须靠 payload 里的 action 才能正确重投（回归：这条曾经永远失败）。
func TestBuildReplayRequestUpdateDeleteHalf(t *testing.T) {
	req, err := buildReplayRequest(deadletter.Record{
		Action:      "update", // canal 的动作，看不出这条是删除
		CdcJobID:    "es_river:db.article_models:bin.1:4:0",
		PayloadJSON: `{"action":"delete","index":"article_index","id":"old-id"}`,
	})
	if err != nil {
		t.Fatalf("删除半条不应因为没有 data 报错: %v", err)
	}
	if req.Action != "delete" || req.ID != "old-id" {
		t.Fatalf("应为 delete old-id，实际 %+v", req)
	}
}

// 老记录（payload 里没有 action）仍按 canal 动作兜底，行为与之前一致。
func TestReplayActionFallsBackToRecordAction(t *testing.T) {
	cases := []struct{ payload, record, want string }{
		{"", "delete", "delete"},
		{"", "insert", "index"},
		{"index", "delete", "index"},  // payload 优先
		{"create", "update", "index"}, // create 归一到 index
		{"weird", "delete", "delete"}, // 认不出的值退回记录动作
	}
	for _, c := range cases {
		req, err := buildReplayRequest(deadletter.Record{
			Action:      c.record,
			PayloadJSON: `{"action":"` + c.payload + `","index":"i","id":"1","data":{"a":1}}`,
		})
		if err != nil {
			t.Fatalf("%+v 不应报错: %v", c, err)
		}
		if req.Action != c.want {
			t.Fatalf("%+v: 期望 %s，实际 %s", c, c.want, req.Action)
		}
	}
}
