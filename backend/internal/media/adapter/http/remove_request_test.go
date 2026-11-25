package http_test

import (
	"encoding/json"
	"testing"

	mediahttp "myblogx/internal/media/adapter/http"
)

// 删除请求的 ID 是**字符串**（雪花 ID 3.8e17 超过 JS 2^53，前端只能按字符串传）。
//
// 这条用例钉住协议形状：DTO 若写回 []uint64，JSON 绑定会失败，而前端把业务失败
// 显示成"参数错误"——2026-11-25 线上"删除按下去没反应"的真正原因。
func TestRemoveRequestAcceptsStringIDs(t *testing.T) {
	t.Parallel()

	var req mediahttp.RemoveRequest
	if err := json.Unmarshal([]byte(`{"id_list":["383532196775858176"]}`), &req); err != nil {
		t.Fatalf("字符串 ID 必须能绑定: %v", err)
	}
	if len(req.IDList) != 1 || req.IDList[0].String() != "383532196775858176" {
		t.Fatalf("ID 解析不对: %+v", req.IDList)
	}
}
