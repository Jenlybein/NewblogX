package qiniu

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"context"
	"time"

	"myblogx/conf"
)

// 审核请求必须用 `Qiniu` 前缀的令牌，不能是 `QBox`。
//
// 这条用例钉住一个只会在线上暴露的坑：SDK 的 SignRequest() 生成 QBox 令牌，
// 而 ai.qiniuapi.com 只认 Qiniu 令牌。签名本身对、前缀不对，七牛回
// `401 bad token`；调用方又按"失败就保持待审核"处理，于是所有新图永远卡在
// 待审核（2026-11-25 线上表现）。这里直接跑一遍真实签名，断言头里的前缀。
func TestCensorImageSignsWithQiniuToken(t *testing.T) {
	t.Parallel()

	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"result":{"suggestion":"pass"}}`))
	}))
	defer server.Close()

	storage := New(conf.QiNiu{
		Enable:    true,
		Bucket:    "demo",
		AccessKey: "ak-for-test",
		SecretKey: "sk-for-test",
	})
	// 把审核地址指向测试服务器：只验证签名，不碰真七牛。
	storage.censorEndpoint = server.URL

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := storage.CensorImage(ctx, "demo", "images/a.png"); err != nil {
		t.Fatalf("审核调用本身应成功: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "Qiniu ") {
		t.Fatalf("审核令牌前缀必须是 Qiniu（QBox 会被七牛拒成 bad token），实际: %q", gotAuth)
	}
	// uri 必须是 qiniu 协议：写成 `bucket:key` 会被拒成 400 not support。
	if want := `"uri":"qiniu:///demo/images/a.png"`; !strings.Contains(gotBody, want) {
		t.Fatalf("审核请求体里的 uri 形状不对，期望包含 %s，实际: %s", want, gotBody)
	}
}
