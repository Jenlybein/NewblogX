package router_test

import (
	"encoding/json"
	"fmt"
	api2 "myblogx/api"
	"myblogx/apideps"
	"myblogx/conf"
	mw "myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/models/enum/global_notif_enum"
	"myblogx/models/enum/message_enum"
	"myblogx/router"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func readBizCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}

	return int(body["code"].(float64))
}

func setupSitemsgRouterEnv(t *testing.T) (*models.UserModel, string) {
	t.Helper()
	testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ChatSessionModel{},
		&models.ArticleMessageModel{},
		&models.GlobalNotifModel{},
		&models.UserGlobalNotifModel{},
	)
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "router-test-secret",
			Issuer: "blogx-test",
		},
	})

	user := &models.UserModel{
		Username: "msg_user",
		Password: "x",
		Nickname: "msg",
		Role:     enum.RoleUser,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	token := testutil.IssueAccessToken(t, user)
	return user, token
}

func newSitemsgRouterEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	apiGroup := r.Group("/api")
	app := api2.New(api2.Deps{
		DB:     testutil.DB(),
		Redis:  testutil.Redis(),
		JWT:    testutil.Config().Jwt,
		Logger: testutil.Logger(),
	})
	runtimeMw := mw.NewRuntime(apideps.Deps{
		DB:               testutil.DB(),
		Redis:            testutil.Redis(),
		JWT:              testutil.Config().Jwt,
		Log:              testutil.Config().Log,
		Logger:           testutil.Logger(),
		System:           testutil.Config().System,
		ClickHouseConfig: testutil.Config().ClickHouse,
	})
	router.SitemsgRouter(apiGroup, app, runtimeMw)
	return r
}

func TestSitemsgRouterPutConfBindsJSON(t *testing.T) {
	testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{})
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "router-test-secret",
			Issuer: "blogx-test",
		},
	})

	user := models.UserModel{
		Username: "msg_user",
		Password: "x",
		Nickname: "msg",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	token := testutil.IssueAccessToken(t, &user)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	apiGroup := r.Group("/api")
	app := api2.New(api2.Deps{
		DB:     testutil.DB(),
		Redis:  testutil.Redis(),
		JWT:    testutil.Config().Jwt,
		Logger: testutil.Logger(),
	})
	runtimeMw := mw.NewRuntime(apideps.Deps{
		DB:               testutil.DB(),
		Redis:            testutil.Redis(),
		JWT:              testutil.Config().Jwt,
		Log:              testutil.Config().Log,
		Logger:           testutil.Logger(),
		System:           testutil.Config().System,
		ClickHouseConfig: testutil.Config().ClickHouse,
	})
	router.SitemsgRouter(apiGroup, app, runtimeMw)

	req := testutil.NewJSONRequest(http.MethodPut, "/api/sitemsg/conf", `{
		"digg_notice_enabled": false,
		"comment_notice_enabled": false,
		"favor_notice_enabled": false,
		"private_chat_notice_enabled": false
	}`)
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码异常: %d, body=%s", w.Code, w.Body.String())
	}
	if code := readBizCode(t, w); code != 0 {
		t.Fatalf("业务码异常: %d, body=%s", code, w.Body.String())
	}

	var confModel models.UserConfModel
	if err := db.Take(&confModel, user.ID).Error; err != nil {
		t.Fatalf("查询用户消息配置失败: %v", err)
	}

	if confModel.DiggNoticeEnabled || confModel.CommentNoticeEnabled ||
		confModel.FavorNoticeEnabled || confModel.PrivateChatNoticeEnabled {
		t.Fatalf("消息配置未按请求更新: %+v", confModel)
	}
}

func TestSitemsgRouterPostSupportsReadByID(t *testing.T) {
	user, token := setupSitemsgRouterEnv(t)
	db := testutil.DB()

	msg := models.ArticleMessageModel{
		ReceiverID: user.ID,
		Type:       message_enum.SystemType,
		Content:    "system",
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	req := testutil.NewJSONRequest(http.MethodPost, "/api/sitemsg", fmt.Sprintf(`{"id":"%s"}`, msg.ID.String()))
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	newSitemsgRouterEngine().ServeHTTP(w, req)

	if w.Code != http.StatusOK || readBizCode(t, w) != 0 {
		t.Fatalf("按 id 标记已读失败, body=%s", w.Body.String())
	}

	var check models.ArticleMessageModel
	if err := db.Take(&check, msg.ID).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if !check.IsRead {
		t.Fatalf("消息未标记已读: %+v", check)
	}
}

func TestSitemsgRouterPostSupportsBatchReadByType(t *testing.T) {
	user, token := setupSitemsgRouterEnv(t)
	db := testutil.DB()

	msgs := []models.ArticleMessageModel{
		{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "d1"},
		{ReceiverID: user.ID, Type: message_enum.FavorArticleType, Content: "f1"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "s1"},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	req := testutil.NewJSONRequest(http.MethodPost, "/api/sitemsg", `{"t":2}`)
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	newSitemsgRouterEngine().ServeHTTP(w, req)

	if w.Code != http.StatusOK || readBizCode(t, w) != 0 {
		t.Fatalf("按类型批量已读失败, body=%s", w.Body.String())
	}

	var checks []models.ArticleMessageModel
	if err := db.Order("id asc").Find(&checks).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if !checks[0].IsRead || !checks[1].IsRead {
		t.Fatalf("点赞/收藏消息未全部标记已读: %+v", checks)
	}
	if checks[2].IsRead {
		t.Fatalf("系统消息不应被批量已读: %+v", checks[2])
	}
}

func TestSitemsgRouterDeleteSupportsRemoveByID(t *testing.T) {
	user, token := setupSitemsgRouterEnv(t)
	db := testutil.DB()

	msg := models.ArticleMessageModel{
		ReceiverID: user.ID,
		Type:       message_enum.SystemType,
		Content:    "system",
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	req := testutil.NewJSONRequest(http.MethodDelete, "/api/sitemsg", fmt.Sprintf(`{"id":"%s"}`, msg.ID.String()))
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	newSitemsgRouterEngine().ServeHTTP(w, req)

	if w.Code != http.StatusOK || readBizCode(t, w) != 0 {
		t.Fatalf("按 id 删除消息失败, body=%s", w.Body.String())
	}

	var count int64
	if err := db.Model(&models.ArticleMessageModel{}).Where("id = ?", msg.ID).Count(&count).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("消息未删除, count=%d", count)
	}
}

func TestSitemsgRouterDeleteSupportsBatchRemoveByType(t *testing.T) {
	user, token := setupSitemsgRouterEnv(t)
	db := testutil.DB()

	msgs := []models.ArticleMessageModel{
		{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "d1"},
		{ReceiverID: user.ID, Type: message_enum.FavorArticleType, Content: "f1"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "s1"},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	req := testutil.NewJSONRequest(http.MethodDelete, "/api/sitemsg", `{"t":2}`)
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	newSitemsgRouterEngine().ServeHTTP(w, req)

	if w.Code != http.StatusOK || readBizCode(t, w) != 0 {
		t.Fatalf("按类型批量删除消息失败, body=%s", w.Body.String())
	}

	var remain []models.ArticleMessageModel
	if err := db.Order("id asc").Find(&remain).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	if len(remain) != 1 || remain[0].Type != message_enum.SystemType {
		t.Fatalf("批量删除范围异常: %+v", remain)
	}
}

func TestSitemsgRouterGetUserSummary(t *testing.T) {
	user, token := setupSitemsgRouterEnv(t)
	db := testutil.DB()

	registerAt := time.Now().Add(-24 * time.Hour).Round(time.Second)
	if err := db.Model(user).Update("created_at", registerAt).Error; err != nil {
		t.Fatalf("更新用户注册时间失败: %v", err)
	}
	if err := db.Take(user, user.ID).Error; err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}

	msgs := []models.ArticleMessageModel{
		{ReceiverID: user.ID, Type: message_enum.CommentArticleType, Content: "comment-unread"},
		{ReceiverID: user.ID, Type: message_enum.DiggArticleType, Content: "digg-unread"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "system-unread"},
		{ReceiverID: user.ID, Type: message_enum.SystemType, Content: "system-read", IsRead: true},
	}
	if err := db.Create(&msgs).Error; err != nil {
		t.Fatalf("创建站内消息失败: %v", err)
	}

	notifs := []models.GlobalNotifModel{
		{
			Title:           "global-unread",
			Content:         "global-unread",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
		{
			Title:           "global-read",
			Content:         "global-read",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
	}
	if err := db.Create(&notifs).Error; err != nil {
		t.Fatalf("创建全局通知失败: %v", err)
	}

	now := time.Now()
	if err := db.Create(&models.UserGlobalNotifModel{
		MsgID:  notifs[1].ID,
		UserID: user.ID,
		IsRead: true,
		ReadAt: &now,
	}).Error; err != nil {
		t.Fatalf("创建全局通知用户态失败: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/sitemsg/user", nil)
	req.Header.Set("token", token)

	w := httptest.NewRecorder()
	newSitemsgRouterEngine().ServeHTTP(w, req)

	if w.Code != http.StatusOK || readBizCode(t, w) != 0 {
		t.Fatalf("查询用户消息统计失败, body=%s", w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	data := body["data"].(map[string]any)
	if int(data["comment_msg_count"].(float64)) != 1 {
		t.Fatalf("评论未读数异常, body=%s", w.Body.String())
	}
	if int(data["digg_favor_msg_count"].(float64)) != 1 {
		t.Fatalf("点赞/收藏未读数异常, body=%s", w.Body.String())
	}
	if int(data["system_msg_count"].(float64)) != 1 {
		t.Fatalf("系统未读数应只包含站内系统消息, body=%s", w.Body.String())
	}
	if int(data["global_msg_count"].(float64)) != 1 {
		t.Fatalf("全局未读数异常, body=%s", w.Body.String())
	}
}
