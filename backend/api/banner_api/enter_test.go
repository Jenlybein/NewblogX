package banner_api_test

import (
	"encoding/json"
	"myblogx/api/banner_api"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return int(body["code"].(float64))
}

func readData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	return data
}

func TestBannerCreateListUpdateRemove(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.BannerModel{}, &models.ImageModel{}, &models.ImageRefModel{})

	api := banner_api.New(banner_api.Deps{DB: testutil.DB()})
	imageA := models.ImageModel{ObjectKey: "banner/a.png", URL: "/a.png", Hash: "hash-a", Status: enum.ImageStatusPass}
	imageB := models.ImageModel{ObjectKey: "banner/b.png", URL: "/b.png", Hash: "hash-b", Status: enum.ImageStatusPass}
	if err := db.Create(&imageA).Error; err != nil {
		t.Fatalf("创建图片 A 失败: %v", err)
	}
	if err := db.Create(&imageB).Error; err != nil {
		t.Fatalf("创建图片 B 失败: %v", err)
	}

	{
		c, w := newCtx()
		c.Set("requestJson", banner_api.BannerCreateRequest{
			CoverImageID: imageA.ID,
			Href:         "/a",
			Show:         true,
		})
		api.BannerCreateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("创建失败, code=%d body=%s", code, w.Body.String())
		}
		if got := readData(t, w)["id"]; got == nil || got == "" {
			t.Fatalf("创建响应应返回字符串 id, body=%s", w.Body.String())
		}
	}

	var created models.BannerModel
	if err := db.First(&created).Error; err != nil {
		t.Fatalf("查询创建数据失败: %v", err)
	}
	if created.Cover != imageA.URL {
		t.Fatalf("创建应按 cover_image_id 写入真实 URL, got=%s want=%s", created.Cover, imageA.URL)
	}

	{
		c, w := newCtx()
		c.Set("requestQuery", banner_api.BannerListRequest{
			Show: true,
		})
		api.BannerListView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("列表失败, code=%d body=%s", code, w.Body.String())
		}
		data := readData(t, w)
		list, _ := data["list"].([]any)
		if len(list) != 1 {
			t.Fatalf("列表数量异常, body=%s", w.Body.String())
		}
		first, _ := list[0].(map[string]any)
		if got := first["cover_image_id"]; got != imageA.ID.String() {
			t.Fatalf("列表应回填 cover_image_id, got=%v want=%s body=%s", got, imageA.ID.String(), w.Body.String())
		}
	}

	{
		c, w := newCtx()
		c.Set("requestUri", models.IDRequest{ID: created.ID})
		c.Set("requestJson", banner_api.BannerCreateRequest{
			CoverImageID: imageB.ID,
			Href:         "/b",
			Show:         false,
		})
		api.BannerUpdateView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("更新失败, code=%d body=%s", code, w.Body.String())
		}
		var updated models.BannerModel
		if err := db.Take(&updated, created.ID).Error; err != nil {
			t.Fatalf("查询更新数据失败: %v", err)
		}
		if updated.Cover != imageB.URL || updated.Show {
			t.Fatalf("更新应写入新图片且允许 show=false, cover=%s show=%v", updated.Cover, updated.Show)
		}
	}

	{
		c, w := newCtx()
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{created.ID}})
		api.BannerRemoveView(c)
		if code := readCode(t, w); code != 0 {
			t.Fatalf("删除失败, code=%d body=%s", code, w.Body.String())
		}
	}

	var cnt int64
	_ = testutil.DB().Model(&models.BannerModel{}).Count(&cnt).Error
	if cnt != 0 {
		t.Fatalf("删除后数量异常: %d", cnt)
	}
}

func TestBannerCreateRejectsUnavailableImage(t *testing.T) {
	testutil.SetupSQLite(t, &models.BannerModel{}, &models.ImageModel{})
	api := banner_api.New(banner_api.Deps{DB: testutil.DB()})

	c, w := newCtx()
	c.Set("requestJson", banner_api.BannerCreateRequest{
		CoverImageID: 999,
		Href:         "/bad",
		Show:         true,
	})
	api.BannerCreateView(c)
	if code := readCode(t, w); code == 0 {
		t.Fatalf("不可用图片不应创建成功, body=%s", w.Body.String())
	}
}
