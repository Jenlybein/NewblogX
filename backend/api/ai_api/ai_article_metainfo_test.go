package ai_api_test

import (
	"encoding/json"
	"myblogx/api/ai_api"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/ai_service"
	"myblogx/service/site_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func newAICtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readAICode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	return int(body["code"].(float64))
}

func newAIApi(t *testing.T) ai_api.AIApi {
	t.Helper()
	db := testutil.DB()
	if db == nil {
		db = testutil.SetupSQLite(t, &models.RuntimeSiteConfigModel{})
	} else if err := db.AutoMigrate(&models.RuntimeSiteConfigModel{}); err != nil {
		t.Fatalf("迁移运行时配置表失败: %v", err)
	}
	runtimeSvc := site_service.NewRuntimeConfigService(testutil.Config().Site, testutil.Config().AI, testutil.Logger(), db, "")
	if err := runtimeSvc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化 AI 运行时配置失败: %v", err)
	}
	if err := runtimeSvc.UpdateRuntimeSite(testutil.Config().Site); err != nil {
		t.Fatalf("更新运行时站点配置失败: %v", err)
	}
	if err := runtimeSvc.UpdateRuntimeAI(testutil.Config().AI); err != nil {
		t.Fatalf("更新运行时 AI 配置失败: %v", err)
	}
	return ai_api.New(ai_api.Deps{
		DB:          db,
		Logger:      testutil.Logger(),
		Redis:       testutil.Redis(),
		ESClient:    testutil.ESClient(),
		ES:          testutil.Config().ES,
		RuntimeSite: runtimeSvc,
	})
}

func TestAIArticleMetaInfoView(t *testing.T) {
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.TagModel{},
	)

	user := models.UserModel{
		Username: "ai_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	category := models.CategoryModel{
		Title:  "Go 后端",
		UserID: user.ID,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}

	tagGo := models.TagModel{Title: "Go", IsEnabled: true}
	if err := db.Create(&tagGo).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	tagGin := models.TagModel{Title: "Gin", IsEnabled: true}
	if err := db.Create(&tagGin).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("AI 请求消息数量错误: %+v", req.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": `{
							"title":"Go 中间件实践",
							"abstract":"围绕 Gin 中间件设计、日志链路与鉴权流程展开。",
							"category":{"id":` + strconv.FormatUint(uint64(category.ID), 10) + `,"title":"Go 后端"},
							"tags":[
								{"id":` + strconv.FormatUint(uint64(tagGo.ID), 10) + `,"title":"Go"},
								{"id":` + strconv.FormatUint(uint64(tagGin.ID), 10) + `,"title":"Gin"}
							]
						}`,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer server.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:        true,
			SecretKey:     "test-key",
			BaseURL:       server.URL,
			ChatModel:     "test-model",
			MaxInputChars: 2048,
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     enum.RoleUser,
			Username: user.Username,
		},
	})
	c.Set("requestJson", ai_api.AIBaseRequest{
		Content: "# Go 中间件\n\n这是一篇讲 Gin 中间件设计的文章。",
	})

	api.AIArticleMetaInfoView(c)

	if code := readAICode(t, w); code != 0 {
		t.Fatalf("文章元信息接口应成功, body=%s", w.Body.String())
	}

	var body struct {
		Code int                              `json:"code"`
		Data ai_api.AIArticleMetaInfoResponse `json:"data"`
		Msg  string                           `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	if body.Data.Title != "Go 中间件实践" {
		t.Fatalf("标题错误: %+v", body.Data)
	}
	if body.Data.Category == nil || body.Data.Category.ID != category.ID {
		t.Fatalf("分类错误: %+v", body.Data.Category)
	}
	if len(body.Data.Tags) != 2 {
		t.Fatalf("标签数量错误: %+v", body.Data.Tags)
	}
}
