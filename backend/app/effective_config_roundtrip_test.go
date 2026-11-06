package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/apperr"
	adapterredis "myblogx/internal/chat/adapter/redis"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/config"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_chat"
	"myblogx/internal/testkit/schema"
	sitehttp "myblogx/internal/site/adapter/http"
	siteapp "myblogx/internal/site/app"
)

// 「此刻生效值」模块的往返用例。唯一测试接缝是后端 HTTP 接口 + 契约：
// 后台 PUT 保存 → 深度合并 → 现读取值闭包 → 读者页 GET → 下一次发送按新值判。
// 消费点只持一个闭包，不再各自捕获配置服务；不重启、不重建服务。
func TestEffectiveConfig_后台改每天99条为7_保存深度合并现读_下一次发送按新值判不用重启(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = idgen.Init(1)

	db, err := gorm.Open(sqlite.Open("file:effective-config?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	if err := db.AutoMigrate(&schema.RuntimeConfigRow{}); err != nil {
		t.Fatalf("建配置表失败: %v", err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	defaults := conf.RuntimeDefaults{Site: conf.Site{Chat: siteconf.Chat{
		RateWindowSec: 10, RateDailyMax: 99,
	}}}
	store := config.NewRuntimeConfigService(defaults, logger, db)
	if err := store.InitRuntimeConfig(); err != nil {
		t.Fatalf("配置补种失败: %v", err)
	}

	effective := config.NewEffectiveConfig(store)
	if rate := effective.ChatRate(); rate.WindowSec != 10 || rate.DailyMax != 99 {
		t.Fatalf("初始生效值应为 10 秒 / 99 条，实际 %+v", rate)
	}

	svc := siteapp.New(nil, nil, siteRuntimeBridge{inner: store}, "test", func() string { return "" }, logger)
	handler := sitehttp.NewHandler(svc, nil)

	// 1) 后台 PUT 保存：只提交 rate_daily_max，其余字段靠深度合并补齐。
	put := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(put)
	putCtx.Request = httptest.NewRequest("PUT", "/site/site", strings.NewReader(`{"chat":{"rate_daily_max":7}}`))
	putCtx.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateSiteSection(putCtx)
	if put.Code != 200 || !strings.Contains(put.Body.String(), `"code":0`) {
		t.Fatalf("后台保存应成功，实际 status=%d body=%s", put.Code, put.Body.String())
	}

	// 2) 现读闭包：不重启、不重建服务，下一次读取就拿到 7。
	rate := effective.ChatRate()
	if rate.DailyMax != 7 || rate.WindowSec != 10 {
		t.Fatalf("保存后现读应为 10 秒 / 7 条（深度合并保留 10 秒），实际 %+v", rate)
	}
	windowSec, dailyMax := effective.ChatRateFunc()()
	if windowSec != 10 || dailyMax != 7 {
		t.Fatalf("消费点闭包应现读到 10 秒 / 7 条，实际 %d 秒 / %d 条", windowSec, dailyMax)
	}

	// 3) 读者页 GET 拿到新值（HTTP 出入口 + 契约 JSON）。
	get := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(get)
	getCtx.Request = httptest.NewRequest("GET", "/site/site", nil)
	handler.PublicSiteSection(getCtx)
	var reader struct {
		Data struct {
			Chat struct {
				RateDailyMax int `json:"rate_daily_max"`
			} `json:"chat"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &reader); err != nil {
		t.Fatalf("读者页响应不是合法契约 JSON: %v, body=%s", err, get.Body.String())
	}
	if reader.Data.Chat.RateDailyMax != 7 {
		t.Fatalf("读者页应拿到新的每天 7 条，实际 %d", reader.Data.Chat.RateDailyMax)
	}

	// 4) 下一次发送：消费点只持这一个现读闭包，判定按 7 而不是 99。
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	deps := redis_service.Options{Client: client, Logger: logger}

	limiter := adapterredis.NewLimiter(deps, effective.ChatRateFunc())
	probe, err := limiter.Reserve(context.Background(), ctype.NewID(8), ctype.NewID(9), 1)
	if err != nil || probe == nil {
		t.Fatalf("限流器按现读闭包应放行首条: err=%v", err)
	}
	if err := probe.Rollback(); err != nil {
		t.Fatalf("回滚探针预占失败: %v", err)
	}

	// 真实限流判定：消费点只持这一个现读闭包（限流器内部委托同一 Lua 脚本）。
	// 滑过 10 秒窗口逐条推进，第 8 条必须被「日额度」挡住 —— 按新值 7，而不是 99。
	sender, receiver := ctype.NewID(1), ctype.NewID(2)
	now := time.Date(2026, 11, 6, 0, 30, 0, 0, time.Local)
	window, daily := effective.ChatRateFunc()()
	for i := 0; i < daily; i++ {
		res, limitedBy, err := redis_chat.ReserveChatPairRate(
			deps, sender, receiver, now.Add(time.Duration(i*11)*time.Second), window, daily)
		if err != nil || res == nil || limitedBy != "" {
			t.Fatalf("第 %d 条应放行: limitedBy=%s err=%v", i+1, limitedBy, err)
		}
	}
	_, limitedBy, err := redis_chat.ReserveChatPairRate(
		deps, sender, receiver, now.Add(time.Duration((daily+1)*11)*time.Second), window, daily)
	if err != nil || limitedBy != "daily" {
		t.Fatalf("第 %d 条应按新的每天 %d 条被判日额度上限，实际 limitedBy=%s err=%v", daily+1, daily, limitedBy, err)
	}
	if !apperr.IsCode(apperr.New(apperr.CodeChatDailyLimit), apperr.CodeChatDailyLimit) {
		t.Fatal("日额度上限应有稳定的错误码")
	}
	if strings.Contains(apperr.Message(apperr.CodeChatDailyLimit), "99") {
		t.Fatalf("日额度文案不应再抄死 99 条，实际: %s", apperr.Message(apperr.CodeChatDailyLimit))
	}
}
