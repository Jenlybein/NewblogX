package image_service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func imageTestDeps() Deps {
	cfg := testutil.Config()
	if cfg == nil {
		cfg = &conf.Config{}
	}
	return Deps{
		QiNiu:  cfg.QiNiu,
		Upload: cfg.Upload,
		DB:     testutil.DB(),
		Redis:  testutil.Redis(),
		Logger: testutil.Logger(),
	}
}

func TestCreateUploadToken(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QiNiu: conf.QiNiu{
			AccessKey: "ak",
			SecretKey: "sk",
			Bucket:    "bucket",
		},
	})

	ret, err := CreateUploadToken(imageTestDeps(), UploadPolicy{
		Bucket:      "bucket",
		ObjectKey:   "blogx/images/test.png",
		CallbackURL: "https://api.example.com/api/images/qiniu/callback",
		ExpireAt:    time.Now().Add(time.Hour),
		MaxSize:     5 * 1024 * 1024,
		EndUser:     "1",
	})
	if err != nil {
		t.Fatalf("CreateUploadToken 失败: %v", err)
	}
	if ret == nil || ret.Token == "" {
		t.Fatal("上传 token 不应为空")
	}
	if !strings.Contains(ret.Token, ":") {
		t.Fatalf("token 格式异常: %s", ret.Token)
	}
}

func TestCreateUploadTokenWithoutCallback(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QiNiu: conf.QiNiu{
			AccessKey: "ak",
			SecretKey: "sk",
			Bucket:    "bucket",
		},
	})

	ret, err := CreateUploadToken(imageTestDeps(), UploadPolicy{
		Bucket:    "bucket",
		ObjectKey: "blogx/images/test-no-callback.png",
		ExpireAt:  time.Now().Add(time.Hour),
		MaxSize:   5 * 1024 * 1024,
		EndUser:   "1",
	})
	if err != nil {
		t.Fatalf("未配置回调地址时也应允许签发上传 token: %v", err)
	}
	if ret == nil || ret.Token == "" {
		t.Fatal("上传 token 不应为空")
	}
}

func TestCreateUploadTokenInvalidPolicy(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QiNiu: conf.QiNiu{
			AccessKey: "ak",
			SecretKey: "sk",
			Bucket:    "bucket",
		},
	})

	ret, err := CreateUploadToken(imageTestDeps(), UploadPolicy{
		Bucket:      "",
		ObjectKey:   "blogx/images/test.png",
		CallbackURL: "https://api.example.com/api/images/qiniu/callback",
		ExpireAt:    time.Now().Add(time.Hour),
		MaxSize:     5 * 1024 * 1024,
	})
	if err == nil || ret != nil {
		t.Fatalf("非法策略应失败, ret=%+v err=%v", ret, err)
	}
}

func TestImageSuffixAndVerifyFormat(t *testing.T) {
	testutil.InitGlobals()

	if s := GetImageSuffix("a.JPG"); s != "jpg" {
		t.Fatalf("GetImageSuffix 错误: %s", s)
	}
	if s := GetImageSuffix("noext"); s != "" {
		t.Fatalf("无后缀应返回空: %s", s)
	}

	pngData, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7+f2UAAAAASUVORK5CYII=",
	)
	h := makeMultipartImageHeader(t, "x.png", pngData)

	err := VerifyImageFormat([]string{"png", "jpg"}, h)
	if err != nil {
		t.Fatalf("合法图片校验失败: %v", err)
	}

	bad := makeMultipartImageHeader(t, "x.jpg", pngData)
	if err = VerifyImageFormat([]string{"jpg"}, bad); err == nil {
		t.Fatal("后缀与内容不匹配应报错")
	}
}

func makeMultipartImageHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile 失败: %v", err)
	}
	if _, err = part.Write(content); err != nil {
		t.Fatalf("写入 multipart 内容失败: %v", err)
	}
	if err = w.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err = req.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("ParseMultipartForm 失败: %v", err)
	}

	return req.MultipartForm.File["file"][0]
}

func TestHandleQiniuAuditCallbackUpdateExistingImage(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.ImageModel{})
	testutil.SetupMiniRedis(t)

	image := models.ImageModel{
		UserID:    1,
		Provider:  enum.ImageProviderQiNiu,
		Bucket:    "bucket",
		ObjectKey: "blogx/images/20260327/etag-audit",
		FileName:  "audit.png",
		URL:       "https://cdn.example.com/blogx/images/20260327/etag-audit",
		MimeType:  "image/png",
		Size:      1,
		Hash:      "etag-audit",
		Status:    enum.ImageStatusPass,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatalf("创建图片记录失败: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"inputBucket": "bucket",
		"inputKey":    "blogx/images/20260327/etag-audit",
		"items": []map[string]any{
			{
				"result": map[string]any{
					"result": map[string]any{
						"suggestion": "block",
					},
				},
			},
		},
	})
	if err := HandleQiniuAuditCallback(imageTestDeps(), body); err != nil {
		t.Fatalf("处理审核回调失败: %v", err)
	}

	var updated models.ImageModel
	if err := db.Take(&updated, "id = ?", image.ID).Error; err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if updated.Status != enum.ImageStatusBlocked {
		t.Fatalf("审核回调后状态应为 blocked, got=%v", updated.Status)
	}
}

func TestHandleQiniuAuditCallbackCacheAndApplyLater(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.ImageModel{})
	testutil.SetupMiniRedis(t)

	body, _ := json.Marshal(map[string]any{
		"inputBucket": "bucket",
		"inputKey":    "blogx/images/20260327/etag-audit-later",
		"items": []map[string]any{
			{
				"result": map[string]any{
					"result": map[string]any{
						"suggestion": "review",
					},
				},
			},
		},
	})
	if err := HandleQiniuAuditCallback(imageTestDeps(), body); err != nil {
		t.Fatalf("处理审核回调失败: %v", err)
	}

	image := models.ImageModel{
		UserID:    1,
		Provider:  enum.ImageProviderQiNiu,
		Bucket:    "bucket",
		ObjectKey: "blogx/images/20260327/etag-audit-later",
		FileName:  "audit-later.png",
		URL:       "https://cdn.example.com/blogx/images/20260327/etag-audit-later",
		MimeType:  "image/png",
		Size:      1,
		Hash:      "etag-audit-later",
		Status:    enum.ImageStatusPass,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatalf("创建图片记录失败: %v", err)
	}

	if err := applyPendingAuditStatusIfAny(imageTestDeps(), &image); err != nil {
		t.Fatalf("应用缓存审核结果失败: %v", err)
	}
	if image.Status != enum.ImageStatusReviewing {
		t.Fatalf("review 应落为 reviewing, got=%v", image.Status)
	}

	var updated models.ImageModel
	if err := db.Take(&updated, "id = ?", image.ID).Error; err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if updated.Status != enum.ImageStatusReviewing {
		t.Fatalf("数据库中的图片状态应为 reviewing, got=%v", updated.Status)
	}
}
