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
	sitehttp "myblogx/internal/site/adapter/http"
	siteapp "myblogx/internal/site/app"
	"myblogx/internal/testkit/schema"
)

// 「此刻生效值」的往返测试（#284）。唯一测试接缝是后端 HTTP 接口 + 契约：
// 后台 PUT 保存 → 深度合并 → 现读取值闭包 → 读者页 GET → 下一次发送按新值判。
// 这里不读源码文本、不逐字段解析 Go tag / TS 键；九段登记与漏登记失败复用
// #283 registry_test.go 的外部行为（RegisteredSections / LookupSection），不重做。
//
// 消费点只持 NewEffectiveConfig 给出的一个现读闭包，不重启、不重建服务。

type effectiveFixture struct {
	db        *gorm.DB
	store     *config.RuntimeConfigService
	effective *config.EffectiveConfig
	handler   *sitehttp.Handler
	logger    *logrus.Logger
}

func newEffectiveFixture(t *testing.T) *effectiveFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := idgen.Init(1); err != nil {
		t.Fatalf("初始化 idgen 失败: %v", err)
	}
	dsn := "file:effective_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	if err := db.AutoMigrate(&schema.RuntimeConfigRow{}); err != nil {
		t.Fatalf("建配置表失败: %v", err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	defaults := conf.RuntimeDefaults{Site: conf.Site{
		Article: siteconf.Article{ShowNonAdminArticles: true},
		Chat:    siteconf.Chat{RateWindowSec: 10, RateDailyMax: 99},
	}}
	store := config.NewRuntimeConfigService(defaults, logger, db)
	if err := store.InitRuntimeConfig(); err != nil {
		t.Fatalf("配置补种失败: %v", err)
	}
	svc := siteapp.New(nil, nil, siteRuntimeBridge{inner: store}, "test", func() string { return "" }, logger)
	return &effectiveFixture{
		db:        db,
		store:     store,
		effective: config.NewEffectiveConfig(store),
		handler:   sitehttp.NewHandler(svc, nil),
		logger:    logger,
	}
}

// call 走具名 HTTP 出入口：方法 + 路径 + 请求体 → 响应体。
func (f *effectiveFixture) call(t *testing.T, fn func(*gin.Context), method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	fn(c)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("%s %s 应成功，实际 status=%d body=%s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

// readSection 从契约响应里取出 data 段的某个键（不读源码、不查库）。
func readSection(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是合法契约 JSON: %v, body=%s", err, rec.Body.String())
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("解析 data 段失败: %v, data=%s", err, envelope.Data)
	}
}

func TestEffectiveConfig_后台改每天99条为7_保存深度合并现读_下一次发送按新值判不用重启(t *testing.T) {
	f := newEffectiveFixture(t)

	if rate := f.effective.ChatRate(); rate.WindowSec != 10 || rate.DailyMax != 99 {
		t.Fatalf("初始生效值应为 10 秒 / 99 条，实际 %+v", rate)
	}

	// 1) 后台 PUT 保存：只提交 rate_daily_max，其余字段靠深度合并补齐。
	f.call(t, f.handler.UpdateSiteSection, "PUT", "/site/site", `{"chat":{"rate_daily_max":7}}`)

	// 2) 现读闭包：不重启、不重建服务，下一次读取就拿到 7。
	rate := f.effective.ChatRate()
	if rate.DailyMax != 7 || rate.WindowSec != 10 {
		t.Fatalf("保存后现读应为 10 秒 / 7 条（深度合并保留 10 秒），实际 %+v", rate)
	}
	windowSec, dailyMax := f.effective.ChatRateFunc()()
	if windowSec != 10 || dailyMax != 7 {
		t.Fatalf("消费点闭包应现读到 10 秒 / 7 条，实际 %d 秒 / %d 条", windowSec, dailyMax)
	}

	// 3) 读者页 GET 拿到新值（HTTP 出入口 + 契约 JSON）。
	var reader struct {
		Chat struct {
			RateDailyMax int `json:"rate_daily_max"`
		} `json:"chat"`
	}
	readSection(t, f.call(t, f.handler.PublicSiteSection, "GET", "/site/site", ""), &reader)
	if reader.Chat.RateDailyMax != 7 {
		t.Fatalf("读者页应拿到新的每天 7 条，实际 %d", reader.Chat.RateDailyMax)
	}

	// 4) 下一次发送：消费点只持这一个现读闭包，判定按 7 而不是 99。
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	deps := redis_service.Options{Client: client, Logger: f.logger}

	limiter := adapterredis.NewLimiter(deps, f.effective.ChatRateFunc())
	probe, err := limiter.Reserve(context.Background(), ctype.NewID(8), ctype.NewID(9), 1)
	if err != nil || probe == nil {
		t.Fatalf("限流器按现读闭包应放行首条: err=%v", err)
	}
	if err := probe.Rollback(); err != nil {
		t.Fatalf("回滚探针预占失败: %v", err)
	}

	// 真实限流判定（限流器委托的同一个 Lua 脚本）：滑过 10 秒窗口逐条推进，
	// 第 8 条必须被「日额度」挡住 —— 按新值 7，而不是旧的 99。
	sender, receiver := ctype.NewID(1), ctype.NewID(2)
	now := time.Date(2026, 11, 6, 0, 30, 0, 0, time.Local)
	window, daily := f.effective.ChatRateFunc()()
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
	if strings.Contains(apperr.Message(apperr.CodeChatDailyLimit), "99") {
		t.Fatalf("日额度文案不应再抄死 99 条，实际: %s", apperr.Message(apperr.CodeChatDailyLimit))
	}
}

// 占位符不写回星号、空串是真清除、其它字符串是覆盖 —— 三种语义只由生效值模块
// 的 ResolveSensitive 判定，这里从 HTTP 具名出入口往返验证。
func TestEffectiveConfig_敏感字段占位符不写回星号_空串真清除_其它字符串覆盖(t *testing.T) {
	f := newEffectiveFixture(t)

	f.call(t, f.handler.UpdateMailSection, "PUT", "/site/mail",
		`{"secret_id":"id-1","secret_key":"real-key","region":"ap-guangzhou"}`)
	var admin struct {
		SecretKey string `json:"secret_key"`
	}
	readSection(t, f.call(t, f.handler.AdminMailSection, "GET", "/site/admin/mail", ""), &admin)
	if admin.SecretKey != config.SensitivePlaceholder {
		t.Fatalf("管理端回显应遮蔽成占位符，实际 %q", admin.SecretKey)
	}

	// 前端把回显的占位符原样提交：其余字段应更新，密钥不能变成星号。
	f.call(t, f.handler.UpdateMailSection, "PUT", "/site/mail",
		`{"secret_id":"id-1","secret_key":"`+config.SensitivePlaceholder+`","region":"ap-hongkong"}`)
	if got := f.store.GetRuntimeMail(); got.SecretKey != "real-key" || got.Region != "ap-hongkong" {
		t.Fatalf("占位符应保留原密钥、其余字段应更新，实际 %+v", got)
	}
	// 换一份重新加载（走模块自己的现读入口），确认库里也没被写成星号。
	reloaded := config.NewRuntimeConfigService(conf.RuntimeDefaults{}, f.logger, f.db)
	if err := reloaded.InitRuntimeConfig(); err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if got := reloaded.GetRuntimeMail(); got.SecretKey != "real-key" || strings.Contains(got.SecretKey, "*") {
		t.Fatalf("重新加载后密钥不应是星号，实际 %q", got.SecretKey)
	}

	// 空串 = 真清除。
	f.call(t, f.handler.UpdateMailSection, "PUT", "/site/mail", `{"secret_key":""}`)
	if got := f.store.GetRuntimeMail().SecretKey; got != "" {
		t.Fatalf("空字符串应能真清除密钥，实际 %q", got)
	}

	// 其它字符串 = 覆盖。
	f.call(t, f.handler.UpdateMailSection, "PUT", "/site/mail", `{"secret_key":"rotated-key"}`)
	if got := f.store.GetRuntimeMail().SecretKey; got != "rotated-key" {
		t.Fatalf("其它字符串应覆盖密钥，实际 %q", got)
	}
}

// 九段全部登记：复用 #283 RegisteredSections() 的外部行为枚举段名，
// 再逐段走具名 HTTP 出入口往返（写成功 + 管理读能拿回新值）。
// 漏登记即失败由 #283 的 registry_test.go 外部行为覆盖，本票不重做注册表。
func TestEffectiveConfig_九段具名出入口全部登记且往返可读(t *testing.T) {
	f := newEffectiveFixture(t)

	names := f.effective.SectionNames()
	if len(names) != 9 {
		t.Fatalf("九段应全部登记，实际 %d 段: %v", len(names), names)
	}
	for _, name := range names {
		if !f.effective.HasSection(string(name)) {
			t.Fatalf("段 %q 未登记", name)
		}
	}
	if f.effective.HasSection("unregistered_section") {
		t.Fatal("注册表之外的段不应被登记（漏登记写库前失败由 #283 registry_test.go 覆盖）")
	}

	// 每段一个具名 PUT + 管理读：从 HTTP 接缝验证登记而非读源码。
	cases := []struct {
		name   string
		update func(*gin.Context)
		admin  func(*gin.Context)
		body   string
		read   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{"site", f.handler.UpdateSiteSection, f.handler.AdminSiteSection,
			`{"site_info":{"beian":"京ICP备00000001号"}}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					SiteInfo struct {
						Beian string `json:"beian"`
					} `json:"site_info"`
				}
				readSection(t, rec, &v)
				if v.SiteInfo.Beian != "京ICP备00000001号" {
					t.Fatalf("site 段往返失败: %+v", v)
				}
			}},
		{"ai", f.handler.UpdateAiSection, f.handler.AdminAiSection,
			`{"nickname":"新助手"}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					Nickname string `json:"nickname"`
				}
				readSection(t, rec, &v)
				if v.Nickname != "新助手" {
					t.Fatalf("ai 段往返失败: %+v", v)
				}
			}},
		{"mail", f.handler.UpdateMailSection, f.handler.AdminMailSection,
			`{"from_name":"新发件人"}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					FromName string `json:"from_name"`
				}
				readSection(t, rec, &v)
				if v.FromName != "新发件人" {
					t.Fatalf("mail 段往返失败: %+v", v)
				}
			}},
		{"storage", f.handler.UpdateStorageSection, f.handler.AdminStorageSection,
			`{"bucket":"b-284"}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					Bucket string `json:"bucket"`
				}
				readSection(t, rec, &v)
				if v.Bucket != "b-284" {
					t.Fatalf("storage 段往返失败: %+v", v)
				}
			}},
		{"oauth", f.handler.UpdateOauthSection, f.handler.AdminOauthSection,
			`{"app_id":"app-284"}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					AppID string `json:"app_id"`
				}
				readSection(t, rec, &v)
				if v.AppID != "app-284" {
					t.Fatalf("oauth 段往返失败: %+v", v)
				}
			}},
		{"log", f.handler.UpdateLogSection, f.handler.AdminLogSection,
			`{"level":"warn"}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					Level string `json:"level"`
				}
				readSection(t, rec, &v)
				if v.Level != "warn" {
					t.Fatalf("log 段往返失败: %+v", v)
				}
			}},
		{"upload", f.handler.UpdateUploadSection, f.handler.AdminUploadSection,
			`{"whitelist":["webp"]}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					Whitelist []string `json:"whitelist"`
				}
				readSection(t, rec, &v)
				if len(v.Whitelist) != 1 || v.Whitelist[0] != "webp" {
					t.Fatalf("upload 段往返失败: %+v", v)
				}
			}},
		{"deco", f.handler.UpdateDecoSection, f.handler.AdminDecoSection,
			`{"charm":{"enable":false}}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					Charm struct {
						Enable bool `json:"enable"`
					} `json:"charm"`
				}
				readSection(t, rec, &v)
				if v.Charm.Enable {
					t.Fatalf("deco 段往返失败: %+v", v)
				}
			}},
		{"image_gc", f.handler.UpdateImageGcSection, f.handler.AdminImageGcSection,
			`{"grace_days":7}`,
			func(t *testing.T, rec *httptest.ResponseRecorder) {
				var v struct {
					GraceDays int `json:"grace_days"`
				}
				readSection(t, rec, &v)
				if v.GraceDays != 7 {
					t.Fatalf("image_gc 段往返失败: %+v", v)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f.call(t, c.update, "PUT", "/site/"+c.name, c.body)
			c.read(t, f.call(t, c.admin, "GET", "/site/admin/"+c.name, ""))
		})
	}
}

// 展示开关四处消费点收口后的往返：后台改一次，下一次现读就翻转。
func TestEffectiveConfig_展示开关改一次_四处消费点同一下一次现读即生效(t *testing.T) {
	f := newEffectiveFixture(t)

	closures := map[string]func() bool{
		"app/visibility_wiring": onlyAdminAuthors(f.effective),
		"search_wiring":         f.effective.OnlyAdminAuthorsFunc(),
		"sidebar":               func() bool { return f.effective.OnlyAdminAuthors() },
	}
	for name, read := range closures {
		if read() {
			t.Fatalf("%s 初始应展示非管理员文章（不过滤），实际被过滤", name)
		}
	}

	f.call(t, f.handler.UpdateSiteSection, "PUT", "/site/site",
		`{"article":{"show_non_admin_articles":false}}`)
	var reader struct {
		Article struct {
			ShowNonAdminArticles bool `json:"show_non_admin_articles"`
		} `json:"article"`
	}
	readSection(t, f.call(t, f.handler.PublicSiteSection, "GET", "/site/site", ""), &reader)
	if reader.Article.ShowNonAdminArticles {
		t.Fatal("读者页应拿到关掉的展示开关")
	}
	for name, read := range closures {
		if !read() {
			t.Fatalf("%s 下一次现读应只出管理员作者，实际仍不过滤", name)
		}
	}

	// 再改回来：不用重启、不重建闭包。
	f.call(t, f.handler.UpdateSiteSection, "PUT", "/site/site",
		`{"article":{"show_non_admin_articles":true}}`)
	for name, read := range closures {
		if read() {
			t.Fatalf("%s 改回后下一次现读应恢复展示，实际仍过滤", name)
		}
	}
}
