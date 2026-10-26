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
