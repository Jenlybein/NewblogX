package io_util_test

import (
	"io"
	"myblogx/utils/io_util"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetBody(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"a":1}`))
	data, err := io_util.GetBody(&body)
	if err != nil {
		t.Fatalf("GetBody 失败: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("body 不一致: %s", string(data))
	}
	data2, _ := io.ReadAll(body)
	if string(data2) != `{"a":1}` {
		t.Fatalf("body 未恢复: %s", string(data2))
	}
}

func TestShouldBindJSONWithRecover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"ok"}`))

	var obj struct {
		X string `json:"x"`
	}
	if err := io_util.ShouldBindJSONWithRecover(c, &obj); err != nil {
		t.Fatalf("ShouldBindJSONWithRecover 失败: %v", err)
	}
	if obj.X != "ok" {
		t.Fatalf("绑定值错误: %+v", obj)
	}

	var obj2 struct {
		X string `json:"x"`
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x"`))
	if err := io_util.ShouldBindJSONWithRecover(c, &obj2); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
