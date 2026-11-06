package http_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	notifhttp "myblogx/internal/notification/adapter/http"
	"myblogx/internal/notification/apptest"
	"myblogx/internal/notification/domain"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/testkit"
)

// notification golden 逐字节快照（#285 AC 5）。
//
// 口径与 governance §3.4 一致：写死种子（ID、时间戳）、逐字节比对、
// **不做归一化**，`UPDATE_GOLDEN=1` 重写；不把 ID / 时间抹成占位符。
// 清单仍是五个只读模块，notification 只是补上本就属于清单的欠账。
// 落点：internal/notification/adapter/http/testdata/golden/。
//
// 种子时刻 2026-01-01 04:00:00Z 对应北京时间 12:00:00 +08:00；
// 用固定 UTC 时刻而非运行时区，换机器不会让 golden 变红。

const notificationGoldenDir = "testdata/golden"

func notificationGoldenBaseTime() time.Time {
	return time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC)
}

// newGoldenFixture 写死种子：用户 1 的注册时间、3 条公告、3 条站内信、
// 通知开关与私信未读数。ID 与时间戳全部是字面量，不做归一化。
func newGoldenFixture(t *testing.T) (*apptest.Env, time.Time) {
	t.Helper()
	now := notificationGoldenBaseTime()
	created := now.Add(-time.Hour)
	messageAt := now.Add(-time.Minute)

	env := apptest.NewEnv()
	env.Globals = apptest.NewGlobals(func() time.Time { return now })
	env.Users.Put(1, apptest.User{
		CreatedAt: now.Add(-48 * time.Hour),
		Nickname:  "小明",
		Avatar:    "avatar.png",
	})
	env.Confs.Put(1, domain.Conf{
		DiggNoticeEnabled:        true,
		CommentNoticeEnabled:     false,
		FavorNoticeEnabled:       true,
		PrivateChatNoticeEnabled: false,
	})
	env.Chat.Set(1, 7)

	for _, g := range []struct {
		id      uint64
		title   string
		content string
		rule    domain.VisibleRule
	}{
		{101, "面向所有人", "所有用户可见", domain.VisibleAll},
		{102, "面向老用户", "发布前注册可见", domain.VisibleRegistered},
		{103, "面向新用户", "发布后注册可见", domain.VisibleNew},
	} {
		env.Globals.Seed(domain.GlobalNotif{
			ID:          ctype.NewID(g.id),
			VisibleRule: g.rule,
			ExpireAt:    now.Add(24 * time.Hour),
			ActionUser:  9,
			Title:       g.title,
			Content:     g.content,
			Icon:        "icon.png",
			Href:        "/a/" + g.title,
			CreatedAt:   created,
		})
	}

	for _, m := range []struct {
		id       uint64
		typ      domain.MessageType
		receiver uint64
		content  string
		isRead   bool
	}{
		{201, domain.TypeCommentArticle, 1, "有人评论了你的文章", false},
		{202, domain.TypeSystem, 1, "系统通知", true},
		{203, domain.TypeDiggArticle, 2, "别人的消息", false},
	} {
		readAt := messageAt
		env.Messages.Seed(domain.Message{
			ID:                 ctype.NewID(m.id),
			Type:               m.typ,
			Receiver:           m.receiver,
			ActionUser:         9,
			ActionUserNickname: "小华",
			ActionUserAvatar:   "h.png",
			Content:            m.content,
			ArticleID:          ctype.NewID(301),
			ArticleTitle:       "文章标题",
			LinkTitle:          "链接标题",
			LinkHref:           "/link",
			IsRead:             m.isRead,
			ReadAt:             &readAt,
			CreatedAt:          messageAt,
			UpdatedAt:          messageAt,
		})
	}
	return env, now
}

type goldenRequest struct {
	name   string
	method string
	path   string
	role   enum.RoleType
}

// goldenCases 是只读接口的固定请求清单；这些接口都是五个只读 golden
// 模块里 notification 的读面，写接口不进逐字节快照。
func goldenCases() []goldenRequest {
	return []goldenRequest{
		{"global_list_user.json", http.MethodGet, "/api/global_notif?type=1", enum.RoleUser},
		{"global_list_admin.json", http.MethodGet, "/api/global_notif?type=2", enum.RoleAdmin},
		{"message_comment_list.json", http.MethodGet, "/api/sitemsg?t=1", enum.RoleUser},
		{"message_system_list.json", http.MethodGet, "/api/sitemsg?t=3", enum.RoleUser},
		{"conf_get.json", http.MethodGet, "/api/sitemsg/conf", enum.RoleUser},
		{"unread_counts.json", http.MethodGet, "/api/sitemsg/user", enum.RoleUser},
	}
}

// TestNotificationGolden 逐字节比对通知模块只读接口的完整响应体。
func TestNotificationGolden(t *testing.T) {
	update := os.Getenv(testkit.UpdateGoldenEnv) == "1"
	env, now := newGoldenFixture(t)
	svc := newNotificationService(env, now, env.Messages)

	for _, c := range goldenCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			engine := newNotificationEngine(t, svc, c.role)
			_, body := callNotification(t, engine, c.method, c.path, "")
			testkit.CompareGolden(t, notificationGoldenDir, c.name, body, update)
		})
	}
}

// TestNotificationErrorContract 接口级验错误码 / 协议码 / 文案 / 载荷（#285 AC 4）。
//
// 每个用例都从 HTTP 响应观察：失败协议码由 apperr 的协议分类给出，中文文案
// 由错误码包给出，data 恒为空对象；内部报错原文绝不进响应。
func TestNotificationErrorContract(t *testing.T) {
	now := notificationGoldenBaseTime()

	t.Run("领域错误-过期时间过短", func(t *testing.T) {
		env := apptest.NewEnv()
		env.Globals = apptest.NewGlobals(func() time.Time { return now })
		expire := now.Add(time.Hour)
		engine := newNotificationEngine(t, newNotificationService(env, now, env.Messages), enum.RoleAdmin)
		got, body := callNotification(t, engine, http.MethodPost, "/api/global_notif",
			`{"title":"t","content":"c","expire_time":"`+expire.Format(time.RFC3339)+`"}`)
		assertFailure(t, got, body, res.FailValidCode, apperr.Message(apperr.CodeNotifExpireTooSoon))
	})

	t.Run("领域错误-标题重复", func(t *testing.T) {
		env := apptest.NewEnv()
		env.Globals = apptest.NewGlobals(func() time.Time { return now })
		engine := newNotificationEngine(t, newNotificationService(env, now, env.Messages), enum.RoleAdmin)
		first, _ := callNotification(t, engine, http.MethodPost, "/api/global_notif", `{"title":"重复","content":"c"}`)
		if first.Code != res.SuccessCode {
			t.Fatalf("首次创建应成功，实际 %+v", first)
		}
		got, body := callNotification(t, engine, http.MethodPost, "/api/global_notif", `{"title":"重复","content":"c"}`)
		assertFailure(t, got, body, res.FailValidCode, apperr.Message(apperr.CodeNotifTitleDuplicated))
	})

	t.Run("适配器参数门禁-排序字段", func(t *testing.T) {
		env := apptest.NewEnv()
		engine := newNotificationEngine(t, newNotificationService(env, now, env.Messages), enum.RoleUser)
		got, body := callNotification(t, engine, http.MethodGet, "/api/global_notif?type=1&order=x", "")
		assertFailure(t, got, body, res.FailValidCode, apperr.Message(apperr.CodeCommonInvalidOrder))
	})

	t.Run("未知错误不吞且不泄漏原文", func(t *testing.T) {
		env := apptest.NewEnv()
		secret := "mysql://user:password@127.0.0.1:3306/db"
		injected := failingMessages{Messages: env.Messages, err: errors.New(secret)}
		engine := newNotificationEngine(t, newNotificationService(env, now, injected), enum.RoleUser)
		got, body := callNotification(t, engine, http.MethodGet, "/api/sitemsg?t=1", "")
		assertFailure(t, got, body, res.FailServiceCode, apperr.Message(apperr.CodeCommonServiceFailed))
		if strings.Contains(string(body), secret) || strings.Contains(string(body), "mysql://") {
			t.Fatalf("内部报错原文不得进响应: %s", body)
		}
	})

	t.Run("登录态错误用专属协议码", func(t *testing.T) {
		env := apptest.NewEnv()
		gin.SetMode(gin.TestMode)
		bare := gin.New()
		h := notifhttp.NewHandler(newNotificationService(env, now, env.Messages))
		bare.GET("/api/global_notif", mw.BindQuery[notifhttp.GlobalListRequest], h.GlobalList)
		req := httptest.NewRequest(http.MethodGet, "/api/global_notif?type=1", nil)
		rec := httptest.NewRecorder()
		bare.ServeHTTP(rec, req)
		var got notificationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("响应不是合法 JSON: %v", err)
		}
		assertFailure(t, got, rec.Body.Bytes(), res.FailAuthCode, apperr.Message(apperr.CodeAuthRequired))
	})
}

// assertFailure 验完整的失败响应：协议码、中文文案与空对象载荷。
func assertFailure(t *testing.T, got notificationResponse, body []byte, wantCode res.Code, wantMsg string) {
	t.Helper()
	if got.Code != wantCode {
		t.Fatalf("协议码错误：期望 %d，实际 %d（body=%s）", wantCode, got.Code, body)
	}
	if got.Msg != wantMsg {
		t.Fatalf("中文文案错误：期望 %q，实际 %q（body=%s）", wantMsg, got.Msg, body)
	}
	if string(got.Data) != "{}" {
		t.Fatalf("失败载荷应为空对象，实际 %s（body=%s）", got.Data, body)
	}
}
