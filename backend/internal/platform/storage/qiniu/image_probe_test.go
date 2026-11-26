package qiniu

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/conf"
)

// 七牛在 fop 不可用（或私有空间没把 fop 签进 token）时，会把**原图**当 200 回给
// 客户端。旧实现直接把它当 JSON 解，于是上传被判失败、提示是
// `invalid character '\u0089' looking for beginning of value`（0x89 = PNG 头）。
//
// 这条用例钉住"fop 拿回原图时必须靠文件头兜底"，即上传不该被七牛侧的可选能力绑死。
func TestImageInfoFallsBackToSniffWhenFopReturnsRawImage(t *testing.T) {
	t.Parallel()

	// 造一张 8×6 的 PNG，尺寸刻意不是 1×1，免得"碰巧对了"。
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatalf("造测试图失败: %v", err)
	}

	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		// 无论带不带 fop，都回原图字节：正是七牛在 fop 不可用时的表现。
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw.Bytes())
	}))
	defer server.Close()

	storage := New(conf.QiNiu{Enable: true, Bucket: "demo", AccessKey: "ak-for-test", SecretKey: "sk-for-test"})
	storage.imageInfoBase = server.URL

	info, err := storage.ImageInfo("images/a.png")
	if err != nil {
		t.Fatalf("fop 不可用时必须靠文件头兜底成功，实际失败: %v", err)
	}
	if info.Format != "png" || info.Width != 8 || info.Height != 6 {
		t.Fatalf("兜底探测结果不对: %+v", info)
	}
	if hits != 2 {
		t.Fatalf("应先后请求 fop 与对象各一次，实际 %d 次", hits)
	}
}

// fop 正常时不该为了兜底多下一次对象（那是白花的流量）。
func TestImageInfoUsesFopWhenItReturnsJSON(t *testing.T) {
	t.Parallel()

	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.Contains(r.URL.RawQuery, "imageInfo") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"format":"jpeg","width":1600,"height":900}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	storage := New(conf.QiNiu{Enable: true, Bucket: "demo", AccessKey: "ak-for-test", SecretKey: "sk-for-test"})
	storage.imageInfoBase = server.URL

	info, err := storage.ImageInfo("images/b.jpg")
	if err != nil {
		t.Fatalf("fop 正常时不该失败: %v", err)
	}
	if info.Format != "jpeg" || info.Width != 1600 || info.Height != 900 {
		t.Fatalf("fop 结果不对: %+v", info)
	}
	if hits != 1 {
		t.Fatalf("fop 成功时不应再取对象，实际请求 %d 次", hits)
	}
}

// 两条通道都失败时要给中文提示：编码器内部错误（invalid character ...）不能
// 直接甩给用户 —— 2026-11-26 线上头像上传弹的就是那句毫无意义的英文。
func TestImageInfoBothChannelsFailGivesFriendlyMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not an image at all"))
	}))
	defer server.Close()

	storage := New(conf.QiNiu{Enable: true, Bucket: "demo", AccessKey: "ak-for-test", SecretKey: "sk-for-test"})
	storage.imageInfoBase = server.URL

	_, err := storage.ImageInfo("images/c.png")
	if err == nil {
		t.Fatal("两条通道都失败时必须报错，不能默默放行")
	}
	msg := err.Error()
	if !strings.Contains(msg, "读取图片信息失败") {
		t.Fatalf("提示应是给用户看的中文，实际: %q", msg)
	}
	for _, leak := range []string{"invalid character", "looking for beginning of value"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("不该把编码器内部错误透给用户（%q），实际: %q", leak, msg)
		}
	}
}
