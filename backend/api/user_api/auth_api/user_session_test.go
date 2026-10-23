package auth_api_test

import (
	"encoding/json"
	"fmt"
	"myblogx/api/user_api/auth_api"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/db_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustNextID(t *testing.T) ctype.ID {
	t.Helper()
	id, err := db_service.NextSnowflakeID()
	if err != nil {
		t.Fatalf("生成雪花 ID 失败: %v", err)
	}
	return id
}

func TestUserSessionListView(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserSessionModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	_ = testutil.SetupMiniRedis(t)
	api := newAuthAPI(t)

	user := models.UserModel{Username: "session_user", Role: enum.RoleUser}
	otherUser := models.UserModel{Username: "session_other", Role: enum.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	now := time.Now()
	currentID := mustNextID(t)
	otherID := mustNextID(t)
	expiredID := mustNextID(t)
	revokedID := mustNextID(t)
	foreignID := mustNextID(t)
	revokedAt := now.Add(-10 * time.Minute)
	lastSeen := now.Add(-5 * time.Minute)

	sessions := []models.UserSessionModel{
		{
			Model:  models.Model{ID: currentID, CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)},
			UserID: user.ID, RefreshTokenHash: "hash-current", IP: "1.1.1.1", Addr: "北京", UA: "Chrome A", LastSeenAt: &lastSeen, ExpiresAt: now.Add(24 * time.Hour),
		},
		{
			Model:  models.Model{ID: otherID, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
			UserID: user.ID, RefreshTokenHash: "hash-other", IP: "2.2.2.2", Addr: "上海", UA: "Chrome B", ExpiresAt: now.Add(12 * time.Hour),
		},
		{
			Model:  models.Model{ID: expiredID, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour)},
			UserID: user.ID, RefreshTokenHash: "hash-expired", IP: "3.3.3.3", Addr: "广州", UA: "Chrome Expired", ExpiresAt: now.Add(-1 * time.Hour),
		},
		{
			Model:  models.Model{ID: revokedID, CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour)},
			UserID: user.ID, RefreshTokenHash: "hash-revoked", IP: "4.4.4.4", Addr: "深圳", UA: "Chrome Revoked", ExpiresAt: now.Add(24 * time.Hour), RevokedAt: &revokedAt,
		},
		{
			Model:  models.Model{ID: foreignID, CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute)},
			UserID: otherUser.ID, RefreshTokenHash: "hash-foreign", IP: "5.5.5.5", Addr: "杭州", UA: "Chrome Foreign", ExpiresAt: now.Add(24 * time.Hour),
		},
	}
	if err := db.Create(&sessions).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	c, w := newCtx()
	c.Set("requestQuery", auth_api.UserSessionListRequest{})
	c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
		UserID:    user.ID,
		SessionID: currentID,
		Username:  user.Username,
		Role:      user.Role,
	}})
	api.UserSessionListView(c)

	if code := readCode(t, w); code != 0 {
		t.Fatalf("查询会话列表失败, body=%s", w.Body.String())
	}

	var resp struct {
		Data struct {
			List  []auth_api.UserSessionItem `json:"list"`
			Count int                        `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.Count != 2 {
		t.Fatalf("只应返回 2 条有效会话, got=%d body=%s", resp.Data.Count, w.Body.String())
	}
	if len(resp.Data.List) != 2 {
		t.Fatalf("返回列表长度错误, got=%d", len(resp.Data.List))
	}
	if resp.Data.List[0].ID != currentID {
		t.Fatalf("会话列表应按 created_at desc 排序, first=%s", resp.Data.List[0].ID.String())
	}
	if !resp.Data.List[0].IsCurrent {
		t.Fatal("当前会话应标记 is_current=true")
	}
	if resp.Data.List[1].IsCurrent {
		t.Fatal("非当前会话不应标记为当前")
	}
}

func TestUserSessionDeleteView(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserSessionModel{}, &models.RuntimeSiteConfigModel{})
	setupAuthEnv(t)
	_ = testutil.SetupMiniRedis(t)
	api := newAuthAPI(t)

	user := models.UserModel{Username: "delete_session_user", Role: enum.RoleUser}
	otherUser := models.UserModel{Username: "delete_session_other", Role: enum.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("创建其他用户失败: %v", err)
	}

	currentToken := testutil.IssueAccessToken(t, &user)
	claims, err := jwts.ParseToken(testutil.Config().Jwt, currentToken)
	if err != nil {
		t.Fatalf("解析 access token 失败: %v", err)
	}

	now := time.Now()
	otherSessionID := mustNextID(t)
	foreignSessionID := mustNextID(t)
	if err := db.Create(&models.UserSessionModel{
		Model:            models.Model{ID: otherSessionID, CreatedAt: now, UpdatedAt: now},
		UserID:           user.ID,
		RefreshTokenHash: fmt.Sprintf("refresh-%s", otherSessionID.String()),
		IP:               "9.9.9.9",
		Addr:             "成都",
		UA:               "Safari",
		ExpiresAt:        now.Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("创建目标会话失败: %v", err)
	}
	if err := db.Create(&models.UserSessionModel{
		Model:            models.Model{ID: foreignSessionID, CreatedAt: now, UpdatedAt: now},
		UserID:           otherUser.ID,
		RefreshTokenHash: fmt.Sprintf("refresh-%s", foreignSessionID.String()),
		IP:               "8.8.8.8",
		Addr:             "武汉",
		UA:               "Firefox",
		ExpiresAt:        now.Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("创建外部会话失败: %v", err)
	}

	t.Run("吊销其他设备成功", func(t *testing.T) {
		c, w := newCtx()
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/users/sessions/"+otherSessionID.String(), nil)
		c.Set("requestUri", models.IDRequest{ID: otherSessionID})
		c.Set("claims", claims)

		api.UserSessionDeleteView(c)

		if code := readCode(t, w); code != 0 {
			t.Fatalf("吊销其他设备失败, body=%s", w.Body.String())
		}

		var session models.UserSessionModel
		if err := db.Unscoped().Take(&session, "id = ?", otherSessionID).Error; err != nil {
			t.Fatalf("查询被吊销会话失败: %v", err)
		}
		if session.RevokedAt == nil {
			t.Fatal("目标会话应被写入 revoked_at")
		}
		if session.RefreshTokenHash == "" {
			t.Fatal("吊销会话后仍应保留哈希，revoked_at 才是失效判定依据")
		}
	})

	t.Run("不能吊销别人的会话", func(t *testing.T) {
		c, w := newCtx()
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/users/sessions/"+foreignSessionID.String(), nil)
		c.Set("requestUri", models.IDRequest{ID: foreignSessionID})
		c.Set("claims", claims)

		api.UserSessionDeleteView(c)

		if code := readCode(t, w); code == 0 {
			t.Fatalf("不应允许吊销他人会话, body=%s", w.Body.String())
		}

		var session models.UserSessionModel
		if err := db.Take(&session, "id = ?", foreignSessionID).Error; err != nil {
			t.Fatalf("查询外部会话失败: %v", err)
		}
		if session.RevokedAt != nil {
			t.Fatal("外部会话不应被吊销")
		}
	})

	t.Run("吊销当前设备会清理 cookie", func(t *testing.T) {
		c, w := newCtx()
		req := httptest.NewRequest(http.MethodDelete, "/api/users/sessions/"+claims.SessionID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+currentToken)
		c.Request = req
		c.Set("requestUri", models.IDRequest{ID: claims.SessionID})
		c.Set("claims", claims)

		api.UserSessionDeleteView(c)

		if code := readCode(t, w); code != 0 {
			t.Fatalf("吊销当前设备失败, body=%s", w.Body.String())
		}

		var session models.UserSessionModel
		if err := db.Unscoped().Take(&session, "id = ?", claims.SessionID).Error; err != nil {
			t.Fatalf("查询当前会话失败: %v", err)
		}
		if session.RevokedAt == nil {
			t.Fatal("当前会话应被吊销")
		}
		cookies := w.Result().Cookies()
		if len(cookies) == 0 || cookies[0].MaxAge != -1 {
			t.Fatalf("吊销当前设备后应清理 refresh cookie, cookies=%v", cookies)
		}
	})
}
