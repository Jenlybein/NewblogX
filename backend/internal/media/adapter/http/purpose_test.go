package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	mediahttp "myblogx/internal/media/adapter/http"
	mediaapptest "myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"
)

// #216 适配器接缝：创建上传任务的请求体里的 purpose 必须真的传给用例层，
// 且契约注解把它标成 cover/avatar 两种取值。非法值必须被 binding 拒绝，
// 不能静默落到用例层当成"未声明用途"。

func purposeEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID:    ctype.NewID(1),
			SessionID: ctype.NewID(1),
			Role:      enum.RoleUser,
		}})
	})
	cfg := domain.Config{
		Enabled: true, Bucket: "bucket", Prefix: "myblogx", Region: "z0",
		SizeMB: 5, ExpirySeconds: 3600,
		CallbackURL: "https://example.com/api/images/qiniu/callback",
		Whitelist:   []string{"png"},
	}
	handler := mediahttp.NewHandler(mediaapptest.NewService(cfg))
	engine.POST("/images/upload-tasks", handler.CreateUploadTask)
	return engine
}

func postCreateTask(t *testing.T, body map[string]any) res.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/images/upload-tasks", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	purposeEngine().ServeHTTP(rec, req)

	var envelope res.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是 JSON 壳: %v，原文 %s", err, rec.Body.String())
	}
	return envelope
}

func taskBody() map[string]any {
	return map[string]any{
		"file_name": "cover.png",
		"size":      1,
		"mime_type": "image/png",
		"hash":      "hash-1",
	}
}

func TestCreateUploadTaskAcceptsDeclaredPurpose(t *testing.T) {
	t.Parallel()

	for _, purpose := range []string{"cover", "avatar"} {
		body := taskBody()
		body["purpose"] = purpose
		got := postCreateTask(t, body)
		if got.Code != res.SuccessCode {
			t.Fatalf("purpose=%q 应创建任务成功，实际 code=%d msg=%q", purpose, got.Code, got.Msg)
		}
	}
}

func TestCreateUploadTaskAcceptsOmittedPurpose(t *testing.T) {
	t.Parallel()

	got := postCreateTask(t, taskBody()) // 私信发图：不带 purpose
	if got.Code != res.SuccessCode {
		t.Fatalf("省略 purpose 应兼容旧客户端，实际 code=%d msg=%q", got.Code, got.Msg)
	}
}

func TestCreateUploadTaskRejectsUnknownPurpose(t *testing.T) {
	t.Parallel()

	for _, purpose := range []string{"poster", "", " "} {
		body := taskBody()
		body["purpose"] = purpose
		got := postCreateTask(t, body)
		if got.Code == res.SuccessCode {
			t.Fatalf("非法用途 %q 不应创建任务成功", purpose)
		}
		if got.Msg != apperr.Message(apperr.CodeImagePurposeInvalid) {
			t.Fatalf("非法用途 %q 应返回明确错误码对应文案 %q，实际 %q",
				purpose, apperr.Message(apperr.CodeImagePurposeInvalid), got.Msg)
		}
	}
}

// 用例层的错误码口径也在这里钉一次：非法用途有明确错误码，
// 不是退化成"参数错误"或静默当 cover。
func TestCreateTaskPurposeErrorCodeMessage(t *testing.T) {
	t.Parallel()

	if apperr.Message(apperr.CodeImagePurposeInvalid) == "" ||
		apperr.Message(apperr.CodeImagePurposeInvalid) == string(apperr.CodeImagePurposeInvalid) {
		t.Fatal("用途非法错误码缺少中文文案")
	}
}
