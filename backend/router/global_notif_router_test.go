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
	"myblogx/router"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupGlobalNotifRouterEnv(t *testing.T) (*models.UserModel, string, *models.UserModel, string) {
	t.Helper()
	testutil.SetupMiniRedis(t)
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
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

	admin := &models.UserModel{
		Username: "router_admin",
		Password: "x",
		Role:     enum.RoleAdmin,
	}
	user := &models.UserModel{
		Username: "router_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	adminToken := testutil.IssueAccessToken(t, admin)
	userToken := testutil.IssueAccessToken(t, user)
	return admin, adminToken, user, userToken
}

func newGlobalNotifRouterEngine() *gin.Engine {
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
	router.GlobalNotifRouter(apiGroup, app, runtimeMw)
	return r
}

func readGlobalNotifRouteCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	return int(body["code"].(float64))
}

func TestGlobalNotifRouterAdminCreateAndUserList(t *testing.T) {
	_, adminToken, _, userToken := setupGlobalNotifRouterEnv(t)
	engine := newGlobalNotifRouterEngine()

	req := testutil.NewJSONRequest(http.MethodPost, "/api/global_notif", `{
		"title":"router-notif",
		"content":"hello router",
		"user_visible_rule":3
	}`)
	req.Header.Set("token", adminToken)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK || readGlobalNotifRouteCode(t, w) != 0 {
		t.Fatalf("管理员创建通知失败, body=%s", w.Body.String())
	}

	listReq, _ := http.NewRequest(http.MethodGet, "/api/global_notif?type=1", nil)
	listReq.Header.Set("token", userToken)

	listW := httptest.NewRecorder()
	engine.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK || readGlobalNotifRouteCode(t, listW) != 0 {
		t.Fatalf("用户查询通知失败, body=%s", listW.Body.String())
	}
}

func TestGlobalNotifRouterDeletePaths(t *testing.T) {
	admin, adminToken, user, userToken := setupGlobalNotifRouterEnv(t)
	db := testutil.DB()
	engine := newGlobalNotifRouterEngine()

	registerAt := time.Now().Add(-2 * time.Hour).Round(time.Second)
	if err := db.Model(user).Update("created_at", registerAt).Error; err != nil {
		t.Fatalf("更新用户注册时间失败: %v", err)
	}

	userNotif := models.GlobalNotifModel{
		Title:           "user-visible",
		Content:         "user-visible",
		ActionUser:      admin.ID,
		UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
		ExpireTime:      time.Now().Add(2 * time.Hour),
	}
	adminNotif := models.GlobalNotifModel{
		Title:           "admin-delete",
		Content:         "admin-delete",
		ActionUser:      admin.ID,
		UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
		ExpireTime:      time.Now().Add(2 * time.Hour),
	}
	if err := db.Create(&[]models.GlobalNotifModel{userNotif, adminNotif}).Error; err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	var list []models.GlobalNotifModel
	if err := db.Order("id asc").Find(&list).Error; err != nil {
		t.Fatalf("查询通知失败: %v", err)
	}
	userNotif = list[0]
	adminNotif = list[1]

	userReq := testutil.NewJSONRequest(http.MethodDelete, "/api/global_notif/user", fmt.Sprintf(`{"id_list":[%d]}`, userNotif.ID))
	userReq.Header.Set("token", userToken)

	userW := httptest.NewRecorder()
	engine.ServeHTTP(userW, userReq)
	if userW.Code != http.StatusOK || readGlobalNotifRouteCode(t, userW) != 0 {
		t.Fatalf("用户删除通知失败, body=%s", userW.Body.String())
	}

	var userState models.UserGlobalNotifModel
	if err := db.Unscoped().Take(&userState, "user_id = ? and msg_id = ?", user.ID, userNotif.ID).Error; err != nil {
		t.Fatalf("查询用户删除状态失败: %v", err)
	}
	if !userState.DeletedAt.Valid {
		t.Fatalf("用户删除通知后应存在软删除标记: %+v", userState)
	}

	adminReq := testutil.NewJSONRequest(http.MethodDelete, "/api/global_notif", fmt.Sprintf(`{"id_list":[%d]}`, adminNotif.ID))
	adminReq.Header.Set("token", adminToken)

	adminW := httptest.NewRecorder()
	engine.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK || readGlobalNotifRouteCode(t, adminW) != 0 {
		t.Fatalf("管理员删除通知失败, body=%s", adminW.Body.String())
	}

	var count int64
	if err := db.Model(&models.GlobalNotifModel{}).Where("id = ?", adminNotif.ID).Count(&count).Error; err != nil {
		t.Fatalf("查询管理员删除结果失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("管理员删除通知未生效, count=%d", count)
	}
}

func TestGlobalNotifRouterReadPath(t *testing.T) {
	admin, _, user, userToken := setupGlobalNotifRouterEnv(t)
	db := testutil.DB()
	engine := newGlobalNotifRouterEngine()

	notif := models.GlobalNotifModel{
		Title:           "router-read",
		Content:         "router-read",
		ActionUser:      admin.ID,
		UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
		ExpireTime:      time.Now().Add(2 * time.Hour),
	}
	if err := db.Create(&notif).Error; err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	req := testutil.NewJSONRequest(http.MethodPost, "/api/global_notif/read", fmt.Sprintf(`{"id_list":[%d]}`, notif.ID))
	req.Header.Set("token", userToken)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK || readGlobalNotifRouteCode(t, w) != 0 {
		t.Fatalf("读取通知失败, body=%s", w.Body.String())
	}

	var state models.UserGlobalNotifModel
	if err := db.Take(&state, "user_id = ? and msg_id = ?", user.ID, notif.ID).Error; err != nil {
		t.Fatalf("查询读取状态失败: %v", err)
	}
	if !state.IsRead || state.ReadAt == nil {
		t.Fatalf("读取状态未正确写入: %+v", state)
	}
}
