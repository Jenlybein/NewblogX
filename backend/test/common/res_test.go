package common_test

import (
	"encoding/json"
	"errors"
	"myblogx/common/res"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newGinCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	return body
}

func TestCodeString(t *testing.T) {
	if res.SuccessCode.String() == "" || res.FailValidCode.String() == "" || res.FailServiceCode.String() == "" {
		t.Fatal("Code.String() 不应为空")
	}
}

func TestOkWithData(t *testing.T) {
	c, w := newGinCtx()
	res.OkWithData(map[string]any{"a": 1}, c)
	body := decodeBody(t, w)

	if body["code"].(float64) != float64(res.SuccessCode) {
		t.Fatalf("code 错误: %v", body["code"])
	}
	if body["msg"] == "" {
		t.Fatal("msg 不应为空")
	}
}

func TestFailWithMsg(t *testing.T) {
	c, w := newGinCtx()
	res.FailWithMsg("bad request", c)
	body := decodeBody(t, w)

	if body["code"].(float64) != float64(res.FailValidCode) {
		t.Fatalf("code 错误: %v", body["code"])
	}
	if body["msg"].(string) != "bad request" {
		t.Fatalf("msg 错误: %v", body["msg"])
	}
}

func TestFailWithError(t *testing.T) {
	c, w := newGinCtx()
	res.FailWithError(errors.New("boom"), c)
	body := decodeBody(t, w)

	if body["code"].(float64) != float64(res.FailServiceCode) {
		t.Fatalf("code 错误: %v", body["code"])
	}
	if body["msg"].(string) != "boom" {
		t.Fatalf("msg 错误: %v", body["msg"])
	}
}
