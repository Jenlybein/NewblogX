package image_api_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"myblogx/api/image_api"
	"myblogx/common"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return body
}

func readCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	return int(readBody(t, w)["code"].(float64))
}

func issueClaims(userID ctype.ID) *jwts.MyClaims {
	return &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   userID,
			Username: "tester",
		},
	}
}

func newImageAPI() image_api.ImageApi {
	return image_api.New(image_api.Deps{
		DB:     testutil.DB(),
		Logger: testutil.Logger(),
		QiNiu:  testutil.Config().QiNiu,
		Upload: testutil.Config().Upload,
		Redis:  testutil.Redis(),
	})
}

func TestImageListView(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.ImageModel{}, &models.ImageRefModel{})

	image := models.ImageModel{
		UserID:    1,
		Provider:  enum.ImageProviderQiNiu,
		Bucket:    "bucket",
		ObjectKey: "blogx/images/20260327/etag-test",
		FileName:  "img.png",
		URL:       "https://cdn.example.com/blogx/images/20260327/etag-test",
		MimeType:  "image/png",
		Size:      1,
		Hash:      "etag-test",
		Status:    enum.ImageStatusPass,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatalf("创建图片记录失败: %v", err)
	}

	api := newImageAPI()
	c, w := newCtx()
	c.Set("requestQuery", common.PageInfo{Page: 1, Limit: 10})
	api.ImageListView(c)

	if code := readCode(t, w); code != 0 {
		t.Fatalf("图片列表失败, code=%d body=%s", code, w.Body.String())
	}
}

func TestCreateUploadTaskViewInvalidConfig(t *testing.T) {
	testutil.SetupMiniRedis(t)
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		Upload: conf.Upload{
			Whitelist: []string{"png", "jpg", "jpeg", "webp"},
		},
		QiNiu: conf.QiNiu{
			Enable: false,
		},
	})

	api := newImageAPI()
	c, w := newCtx()
	c.Set("requestJson", image_api.CreateImageUploadTaskRequest{
		FileName: "avatar.png",
		Size:     123,
		MimeType: "image/png",
		Hash:     "etag-create",
	})
	c.Set("claims", issueClaims(1))

	api.CreateUploadTaskView(c)
	if code := readCode(t, w); code == 0 {
		t.Fatalf("七牛未启用时应返回失败, body=%s", w.Body.String())
	}
}

func TestUploadTaskStatusViewNotFound(t *testing.T) {
	testutil.SetupMiniRedis(t)
	api := newImageAPI()
	c, w := newCtx()
	c.Set("requestUri", models.IDRequest{ID: 999})
	c.Set("claims", issueClaims(1))

	api.UploadTaskStatusView(c)
	if code := readCode(t, w); code == 0 {
		t.Fatalf("不存在的上传任务应返回失败, body=%s", w.Body.String())
	}
}
