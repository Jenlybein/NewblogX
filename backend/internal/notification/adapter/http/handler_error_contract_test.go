package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/notification/app"
	"myblogx/internal/notification/apptest"
	"myblogx/internal/notification/domain"
	notifhttp "myblogx/internal/notification/adapter/http"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/jwts"
)

// 这一组用例只从**后端 HTTP 接口 + 契约（开放接口契约）**这一条接缝观察通知模块：
// 装的是含绑定中间件的真路由、登录态按 claims 注入，断言只看响应壳的
// 协议码 / 中文文案 / data 载荷，不看处理器里有没有某个函数名或错误码分支。

// notificationResponse 是 Envelope[T] 里用例关心的三个字段。
// Data 保留原始字节，便于逐字节比对失败载荷（不做归一化）。
type notificationResponse struct {
	Code res.Code        `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

// newNotificationEngine 挂上通知模块的全部只读/写入入口（与路由表同形），
// 并把登录态按 claims 注入；不经过鉴权中间件，用例只关心适配器之外的可见行为。
func newNotificationEngine(t *testing.T, svc *app.Service, role enum.RoleType) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{
			UserID: ctype.NewID(1), SessionID: ctype.NewID(1), Role: role, Username: "reader",
		}})
	})
	h := notifhttp.NewHandler(svc)

	r.GET("/api/global_notif", mw.BindQuery[notifhttp.GlobalListRequest], h.GlobalList)
	r.POST("/api/global_notif/read", mw.BindJson[notifhttp.IDListRequest], h.GlobalRead)
	r.DELETE("/api/global_notif/user", mw.BindJson[notifhttp.IDListRequest], h.GlobalUserRemove)
	r.POST("/api/global_notif", mw.BindJson[notifhttp.GlobalCreateRequest], h.GlobalCreate)
	r.DELETE("/api/global_notif", mw.BindJson[notifhttp.IDListRequest], h.GlobalAdminRemove)
	r.GET("/api/sitemsg", mw.BindQuery[notifhttp.MessageListRequest], h.Messages)
	r.POST("/api/sitemsg", mw.BindJson[notifhttp.MessageSelectRequest], h.MessageRead)
	r.DELETE("/api/sitemsg", mw.BindJson[notifhttp.MessageSelectRequest], h.MessageRemove)
	r.GET("/api/sitemsg/conf", h.Conf)
	r.PUT("/api/sitemsg/conf", mw.BindJson[notifhttp.ConfRequest], h.ConfUpdate)
	r.GET("/api/sitemsg/user", h.Unread)
	return r
}

// callNotification 发一次真实 HTTP 请求，返回解出的响应壳与完整响应体。
func callNotification(t *testing.T, r *gin.Engine, method, path, body string) (notificationResponse, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var out notificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s 的响应不是合法 JSON: %v（body=%s）", method, path, err, rec.Body.String())
	}
	return out, rec.Body.Bytes()
}

// fixedNotificationClock 固定"此刻"，让错误与可见性边界可复现。
type fixedNotificationClock struct{ at time.Time }

func (c fixedNotificationClock) Now() time.Time { return c.at }

// newNotificationEnv 构造写死时间的有状态假仓储环境。
func newNotificationEnv(at time.Time) *apptest.Env {
	env := apptest.NewEnv()
	env.Globals = apptest.NewGlobals(func() time.Time { return at })
	return env
}

// newNotificationService 允许替换站内信仓储，让同一个领域错误从另一条分支返回。
func newNotificationService(env *apptest.Env, at time.Time, messages domain.MessageRepository) *app.Service {
	return app.New(messages, env.Globals, env.Confs, env.Users, env.Chat, fixedNotificationClock{at: at}, nil)
}

// failingMessages 是有状态假仓储的一个失败变体：只让 List 返回注入的错误值，
// 其余状态仍由 apptest.Messages 持有。
type failingMessages struct {
	*apptest.Messages
	err error
}

func (f failingMessages) List(ctx context.Context, receiver uint64, types []domain.MessageType, page domain.Page) ([]domain.Message, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return f.Messages.List(ctx, receiver, types, page)
}

// TestNotificationErrorProtocol_同一领域错误只有一种协议码 钉住 #285 AC 1/2：
// 同一个领域错误值无论从"用户目录缺失"的分支，还是从"站内信仓储失败"的分支
// 返回，适配器都必须原样转发，得到逐字节一致的协议码、中文文案与 data 载荷。
func TestNotificationErrorProtocol_同一领域错误只有一种协议码(t *testing.T) {
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	env := newNotificationEnv(at)

	// 分支一：GET /global_notif?type=1 走用户目录，用户未登记 -> notif.user_missing。
	naturalEngine := newNotificationEngine(t, newNotificationService(env, at, env.Messages), enum.RoleUser)
	natural, naturalBody := callNotification(t, naturalEngine, http.MethodGet, "/api/global_notif?type=1", "")

	if natural.Code != res.FailValidCode {
		t.Fatalf("领域错误应落在共享响应层的固定协议码，实际 code=%d msg=%q", natural.Code, natural.Msg)
	}
	if natural.Msg != apperr.Message(apperr.CodeNotifUserMissing) {
		t.Fatalf("中文文案应只来自错误码包，实际 %q", natural.Msg)
	}
	if string(natural.Data) != "{}" {
		t.Fatalf("失败载荷应为干净的空对象，实际 %s", natural.Data)
	}

	// 分支二：同一错误值从站内信列表返回。适配器若按错误码挑分支重写响应，
	// 这里就会给出另一种协议码。
	injected := failingMessages{Messages: env.Messages, err: apperr.New(apperr.CodeNotifUserMissing)}
	otherEngine := newNotificationEngine(t, newNotificationService(env, at, injected), enum.RoleUser)
	other, otherBody := callNotification(t, otherEngine, http.MethodGet, "/api/sitemsg?t=1", "")

	if other.Code != natural.Code || other.Msg != natural.Msg || string(other.Data) != string(natural.Data) {
		t.Fatalf("同一个领域错误出现了两种响应：\n分支一 code=%d data=%s msg=%q\n分支二 code=%d data=%s msg=%q",
			natural.Code, natural.Data, natural.Msg, other.Code, other.Data, other.Msg)
	}
	if !bytes.Equal(naturalBody, otherBody) {
		t.Fatalf("同一个领域错误的响应应逐字节一致：\n--- 分支一 ---\n%s\n--- 分支二 ---\n%s", naturalBody, otherBody)
	}
}
