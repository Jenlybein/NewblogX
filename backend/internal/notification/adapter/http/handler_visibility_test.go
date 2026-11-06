package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/kernel/enum"
	"myblogx/internal/notification/apptest"
	"myblogx/internal/notification/domain"
)

// 公告可见性的四个边界全部从后端 HTTP 接口观察（#285 AC 3/4）：
// 刚过期、注册同秒、新用户、老用户。第五个用例钉住「以线上查询逐字对齐」：
// 线上 SQL 的三段 OR 只认 1/2/3 三种 user_visible_rule，未知规则不可见。

type announcement struct {
	title    string
	created  time.Time
	expire   time.Time
	rule     domain.VisibleRule
	wantSeen bool
}

type visibilityCase struct {
	name          string
	userCreatedAt time.Time
	announcements []announcement
}

// visibilityEngine 用可移动的时间播种公告：Globals.Create 把 now() 写成
// created_at，查询时再把 now() 设回写死的此刻。ID 与时间全部确定，不做归一化。
func visibilityEngine(t *testing.T, now time.Time, tc visibilityCase) *gin.Engine {
	t.Helper()
	clockAt := new(time.Time)
	*clockAt = now

	env := apptest.NewEnv()
	env.Globals = apptest.NewGlobals(func() time.Time { return *clockAt })
	env.Users.Put(1, apptest.User{CreatedAt: tc.userCreatedAt})

	ctx := context.Background()
	for _, a := range tc.announcements {
		*clockAt = a.created
		n := domain.GlobalNotif{
			VisibleRule: a.rule,
			ExpireAt:    a.expire,
			Title:       a.title,
			Content:     "内容-" + a.title,
			Icon:        "icon.png",
			Href:        "/a/" + a.title,
		}
		if err := env.Globals.Create(ctx, &n); err != nil {
			t.Fatalf("播种公告 %q 失败: %v", a.title, err)
		}
	}
	*clockAt = now

	svc := newNotificationService(env, now, env.Messages)
	return newNotificationEngine(t, svc, enum.RoleUser)
}

// TestNotificationVisibility_四个边界以线上查询为准 只走 GET /global_notif?type=1，
// 断言「哪些公告出现在 data.list 里」；不碰领域纯函数、不读源码文本。
func TestNotificationVisibility_四个边界以线上查询为准(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	publish := now.Add(-time.Hour)
	oldUser := now.Add(-48 * time.Hour)
	newUser := now.Add(-30 * time.Minute)
	expire := now.Add(24 * time.Hour)

	cases := []visibilityCase{
		{
			name:          "刚过期",
			userCreatedAt: oldUser,
			announcements: []announcement{
				{"刚过期-所有用户", publish, now, domain.VisibleAll, false},
				{"未过期-所有用户", publish, expire, domain.VisibleAll, true},
			},
		},
		{
			name:          "注册同秒",
			userCreatedAt: publish,
			announcements: []announcement{
				{"同秒-老用户规则", publish, expire, domain.VisibleRegistered, true},
				{"同秒-新用户规则", publish, expire, domain.VisibleNew, false},
				{"同秒-所有用户", publish, expire, domain.VisibleAll, true},
			},
		},
		{
			name:          "新用户",
			userCreatedAt: newUser,
			announcements: []announcement{
				{"新用户-新用户规则", publish, expire, domain.VisibleNew, true},
				{"新用户-老用户规则", publish, expire, domain.VisibleRegistered, false},
				{"新用户-所有用户", publish, expire, domain.VisibleAll, true},
			},
		},
		{
			name:          "老用户",
			userCreatedAt: oldUser,
			announcements: []announcement{
				{"老用户-老用户规则", publish, expire, domain.VisibleRegistered, true},
				{"老用户-新用户规则", publish, expire, domain.VisibleNew, false},
				{"老用户-所有用户", publish, expire, domain.VisibleAll, true},
			},
		},
		{
			name:          "未知可见规则按线上查询不可见",
			userCreatedAt: oldUser,
			announcements: []announcement{
				{"未知规则", publish, expire, domain.VisibleRule(0), false},
				{"正常老用户规则", publish, expire, domain.VisibleRegistered, true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := visibilityEngine(t, now, tc)
			got, _ := callNotification(t, engine, http.MethodGet, "/api/global_notif?type=1", "")

			var page struct {
				List []struct {
					Title string `json:"title"`
				} `json:"list"`
			}
			if err := json.Unmarshal(got.Data, &page); err != nil {
				t.Fatalf("data 不是合法列表载荷: %v（raw=%s）", err, got.Data)
			}
			seen := make(map[string]bool, len(page.List))
			for _, item := range page.List {
				seen[item.Title] = true
			}
			for _, a := range tc.announcements {
				if seen[a.title] != a.wantSeen {
					t.Fatalf("公告 %q 可见性错误：期望 seen=%v，响应里的标题=%v（body=%s）",
						a.title, a.wantSeen, seen, got.Data)
				}
			}
		})
	}
}
