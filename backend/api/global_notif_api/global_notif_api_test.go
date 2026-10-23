package global_notif_api_test

import (
	"encoding/json"
	"myblogx/api/global_notif_api"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/models/enum/global_notif_enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newGlobalNotifCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readGlobalNotifBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	return body
}

func readGlobalNotifCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	return int(readGlobalNotifBody(t, w)["code"].(float64))
}

func setupGlobalNotifEnv(t *testing.T) (*models.UserModel, *models.UserModel) {
	t.Helper()
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.GlobalNotifModel{},
		&models.UserGlobalNotifModel{},
	)

	admin := &models.UserModel{
		Username: "notif_admin",
		Password: "x",
		Role:     enum.RoleAdmin,
	}
	user := &models.UserModel{
		Username: "notif_user",
		Password: "x",
		Role:     enum.RoleUser,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return admin, user
}

func setupGlobalNotifAPI() global_notif_api.GlobalNotifApi {
	return global_notif_api.New(global_notif_api.Deps{
		DB: testutil.DB(),
	})
}

func setClaims(c *gin.Context, user *models.UserModel) {
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     user.Role,
			Username: user.Username,
		},
	})
}

func TestGlobalNotifCreateViewDefaultsAndPermission(t *testing.T) {
	admin, user := setupGlobalNotifEnv(t)
	api := setupGlobalNotifAPI()

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestJson", global_notif_api.GlobalNotifCreateRequest{
			Title:   "user-create",
			Content: "denied",
		})
		api.GlobalNotifCreateView(c)
		if code := readGlobalNotifCode(t, w); code == 0 {
			t.Fatalf("普通用户创建通知应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, admin)
		c.Set("requestJson", global_notif_api.GlobalNotifCreateRequest{
			Title:   "hello",
			Content: "world",
		})
		api.GlobalNotifCreateView(c)
		if code := readGlobalNotifCode(t, w); code != 0 {
			t.Fatalf("管理员创建通知失败, body=%s", w.Body.String())
		}
		data := readGlobalNotifBody(t, w)["data"].(map[string]any)
		if _, ok := data["id"].(string); !ok {
			t.Fatalf("创建通知返回的 id 应为字符串, body=%s", w.Body.String())
		}
	}

	var notif models.GlobalNotifModel
	if err := testutil.DB().Take(&notif, "title = ?", "hello").Error; err != nil {
		t.Fatalf("查询通知失败: %v", err)
	}
	if notif.ActionUser != admin.ID {
		t.Fatalf("通知操作人错误: got=%d want=%d", notif.ActionUser, admin.ID)
	}
	if notif.UserVisibleRule != global_notif_enum.UserVisibleRegisteredUsers {
		t.Fatalf("默认可见规则错误: %v", notif.UserVisibleRule)
	}
	if notif.ExpireTime.Before(time.Now().Add(6 * 24 * time.Hour)) {
		t.Fatalf("默认过期时间过短: %v", notif.ExpireTime)
	}
}

func TestGlobalNotifListViewAndUserRemove(t *testing.T) {
	_, user := setupGlobalNotifEnv(t)
	api := setupGlobalNotifAPI()
	db := testutil.DB()

	registerAt := time.Now().Add(-24 * time.Hour).Round(time.Second)
	if err := db.Model(user).Update("created_at", registerAt).Error; err != nil {
		t.Fatalf("更新用户注册时间失败: %v", err)
	}
	if err := db.Take(user, user.ID).Error; err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}

	future := registerAt.Add(48 * time.Hour)
	past := registerAt.Add(-time.Hour)
	expired := registerAt.Add(-time.Minute)

	notifs := []models.GlobalNotifModel{
		{Title: "all", Content: "all", UserVisibleRule: global_notif_enum.UserVisibleAllUsers, ExpireTime: future},
		{Title: "registered", Content: "registered", UserVisibleRule: global_notif_enum.UserVisibleRegisteredUsers, ExpireTime: future},
		{Title: "new", Content: "new", UserVisibleRule: global_notif_enum.UserVisibleNewUsers, ExpireTime: future},
		{Title: "expired", Content: "expired", UserVisibleRule: global_notif_enum.UserVisibleAllUsers, ExpireTime: expired},
		{Title: "deleted", Content: "deleted", UserVisibleRule: global_notif_enum.UserVisibleAllUsers, ExpireTime: future},
	}
	if err := db.Create(&notifs).Error; err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	if err := db.Model(&notifs[1]).Update("created_at", registerAt.Add(time.Hour)).Error; err != nil {
		t.Fatalf("更新老用户通知时间失败: %v", err)
	}
	if err := db.Model(&notifs[2]).Update("created_at", past).Error; err != nil {
		t.Fatalf("更新新用户通知时间失败: %v", err)
	}

	readState := models.UserGlobalNotifModel{
		MsgID:  notifs[0].ID,
		UserID: user.ID,
		IsRead: true,
	}
	if err := db.Create(&readState).Error; err != nil {
		t.Fatalf("创建已读状态失败: %v", err)
	}

	deletedState := models.UserGlobalNotifModel{
		Model: models.Model{
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		},
		MsgID:  notifs[4].ID,
		UserID: user.ID,
	}
	if err := db.Create(&deletedState).Error; err != nil {
		t.Fatalf("创建删除状态失败: %v", err)
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestQuery", global_notif_api.GlobalNotifListRequest{Type: 1})
		api.GlobalNotifListView(c)
		if code := readGlobalNotifCode(t, w); code != 0 {
			t.Fatalf("查询用户通知失败, body=%s", w.Body.String())
		}

		data := readGlobalNotifBody(t, w)["data"].(map[string]any)
		if data["has_more"].(bool) {
			t.Fatalf("用户通知 has_more 应为 false, body=%s", w.Body.String())
		}
		if len(data["list"].([]any)) != 3 {
			t.Fatalf("用户通知数量异常, body=%s", w.Body.String())
		}

		list := data["list"].([]any)
		got := make(map[string]bool, len(list))
		for _, item := range list {
			row := item.(map[string]any)
			got[row["title"].(string)] = row["is_read"].(bool)
		}
		if !got["all"] {
			t.Fatalf("已读状态未返回, body=%s", w.Body.String())
		}
		if _, ok := got["registered"]; !ok {
			t.Fatalf("老用户通知未返回, body=%s", w.Body.String())
		}
		if _, ok := got["new"]; !ok {
			t.Fatalf("新用户通知未返回, body=%s", w.Body.String())
		}
		if _, ok := got["expired"]; ok {
			t.Fatalf("过期通知不应返回, body=%s", w.Body.String())
		}
		if _, ok := got["deleted"]; ok {
			t.Fatalf("已删除通知不应返回, body=%s", w.Body.String())
		}
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{notifs[1].ID}})
		api.GlobalNotifUserRemoveView(c)
		if code := readGlobalNotifCode(t, w); code != 0 {
			t.Fatalf("用户删除通知失败, body=%s", w.Body.String())
		}
	}

	var removed models.UserGlobalNotifModel
	if err := db.Unscoped().Take(&removed, "user_id = ? and msg_id = ?", user.ID, notifs[1].ID).Error; err != nil {
		t.Fatalf("查询用户删除状态失败: %v", err)
	}
	if !removed.DeletedAt.Valid {
		t.Fatalf("用户删除通知后应保留软删除标记: %+v", removed)
	}
}

func TestGlobalNotifReadView(t *testing.T) {
	_, user := setupGlobalNotifEnv(t)
	api := setupGlobalNotifAPI()
	db := testutil.DB()

	registerAt := time.Now().Add(-24 * time.Hour).Round(time.Second)
	if err := db.Model(user).Update("created_at", registerAt).Error; err != nil {
		t.Fatalf("更新用户注册时间失败: %v", err)
	}
	if err := db.Take(user, user.ID).Error; err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}

	notifs := []models.GlobalNotifModel{
		{
			Title:           "read-me",
			Content:         "read-me",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
		{
			Title:           "deleted-read",
			Content:         "deleted-read",
			UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
			ExpireTime:      time.Now().Add(24 * time.Hour),
		},
	}
	if err := db.Create(&notifs).Error; err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	deletedState := models.UserGlobalNotifModel{
		Model: models.Model{
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		},
		MsgID:  notifs[1].ID,
		UserID: user.ID,
	}
	if err := db.Create(&deletedState).Error; err != nil {
		t.Fatalf("创建已删除状态失败: %v", err)
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{notifs[0].ID}})
		api.GlobalNotifReadView(c)
		if code := readGlobalNotifCode(t, w); code != 0 {
			t.Fatalf("首次标记已读失败, body=%s", w.Body.String())
		}
	}

	var readState models.UserGlobalNotifModel
	if err := db.Take(&readState, "user_id = ? and msg_id = ?", user.ID, notifs[0].ID).Error; err != nil {
		t.Fatalf("查询已读状态失败: %v", err)
	}
	if !readState.IsRead || readState.ReadAt == nil {
		t.Fatalf("消息未被正确标记已读: %+v", readState)
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{notifs[0].ID}})
		api.GlobalNotifReadView(c)
		if code := readGlobalNotifCode(t, w); code == 0 {
			t.Fatalf("重复读取应失败, body=%s", w.Body.String())
		}
	}

	{
		c, w := newGlobalNotifCtx()
		setClaims(c, user)
		c.Set("requestJson", models.IDListRequest{IDList: []ctype.ID{notifs[1].ID}})
		api.GlobalNotifReadView(c)
		if code := readGlobalNotifCode(t, w); code == 0 {
			t.Fatalf("读取已删除通知应失败, body=%s", w.Body.String())
		}
	}
}

func TestUserGlobalNotifStateUniqueIndex(t *testing.T) {
	_, user := setupGlobalNotifEnv(t)
	db := testutil.DB()

	notif := models.GlobalNotifModel{
		Title:           "unique-state",
		Content:         "unique-state",
		UserVisibleRule: global_notif_enum.UserVisibleAllUsers,
		ExpireTime:      time.Now().Add(24 * time.Hour),
	}
	if err := db.Create(&notif).Error; err != nil {
		t.Fatalf("创建通知失败: %v", err)
	}

	first := models.UserGlobalNotifModel{
		MsgID:  notif.ID,
		UserID: user.ID,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("创建首条用户通知状态失败: %v", err)
	}

	second := models.UserGlobalNotifModel{
		MsgID:  notif.ID,
		UserID: user.ID,
	}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("同一用户同一通知不应创建出第二条状态记录")
	}
}
