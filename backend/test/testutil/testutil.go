package testutil

import (
	"fmt"
	"io"
	"myblogx/buildinfo"
	"myblogx/conf"
	blogmodels "myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/db_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var sqliteDSNCounter uint64
var testConfig *conf.Config
var testLogger *logrus.Logger
var testDB *gorm.DB
var testRedis *redis.Client
var testESClient *elasticsearch.Client
var testImageCaptchaStore = base64Captcha.DefaultMemStore

func InitGlobals() {
	if testLogger == nil {
		testLogger = logrus.New()
		testLogger.SetOutput(io.Discard)
	}
	if testConfig == nil {
		testConfig = &conf.Config{}
	}
	applyDefaultConfig(testConfig)
	configureModules()
	if err := db_service.InitSnowflake(testConfig.System.ServerID); err != nil {
		panic(fmt.Errorf("初始化测试雪花 ID 生成器失败: %w", err))
	}
}

func applyDefaultConfig(cfg *conf.Config) {
	if cfg.Log.Dir == "" {
		cfg.Log.Dir = defaultTestLogDir()
	}
	if cfg.Log.App == "" {
		cfg.Log.App = "test"
	}
	if cfg.Log.StdoutFormat == "" {
		cfg.Log.StdoutFormat = "json"
	}
	if cfg.System.ServerID == 0 {
		cfg.System.ServerID = 1
	}
}

func configureModules() {
	if testConfig == nil || testLogger == nil {
		return
	}
}

func Config() *conf.Config {
	InitGlobals()
	return testConfig
}

func SetConfig(cfg *conf.Config) *conf.Config {
	if cfg == nil {
		cfg = &conf.Config{}
	}
	applyDefaultConfig(cfg)
	testConfig = cfg
	configureModules()
	return testConfig
}

func Logger() *logrus.Logger {
	InitGlobals()
	return testLogger
}

func DB() *gorm.DB {
	return testDB
}

func Redis() *redis.Client {
	return testRedis
}

func ESClient() *elasticsearch.Client {
	return testESClient
}

func SetESClient(client *elasticsearch.Client) *elasticsearch.Client {
	testESClient = client
	configureModules()
	return testESClient
}

func ImageCaptchaStore() base64Captcha.Store {
	return testImageCaptchaStore
}

func Version() string {
	return buildinfo.Version
}

func defaultTestLogDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "./logs/test_logs"
	}
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	return filepath.Join(projectRoot, "logs", "test_logs")
}

func SetupMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	InitGlobals()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}

	testRedis = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	configureModules()

	t.Cleanup(func() {
		if testRedis != nil {
			_ = testRedis.Close()
			testRedis = nil
		}
		configureModules()
		mr.Close()
	})

	return mr
}

func SetupSQLite(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	InitGlobals()

	seq := atomic.AddUint64(&sqliteDSNCounter, 1)
	dsn := fmt.Sprintf("file:test_%d_%d?mode=memory&cache=shared", time.Now().UnixNano(), seq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sqlite 连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if len(models) > 0 {
		models = appendDependentModels(models)
		if err = db.AutoMigrate(models...); err != nil {
			t.Fatalf("自动迁移失败: %v", err)
		}
	}

	testDB = db
	configureModules()
	return db
}

func CreateUser(t *testing.T, db *gorm.DB, user *blogmodels.UserModel) {
	t.Helper()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return user_service.InitUserDefaults(tx, user.ID)
	}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
}

func appendDependentModels(list []any) []any {
	hasArticle := false
	hasTag := false
	hasArticleTag := false
	hasUser := false
	hasUserConf := false
	hasUserStat := false
	hasUserSession := false

	for _, item := range list {
		switch reflect.TypeOf(item) {
		case reflect.TypeOf(&blogmodels.ArticleModel{}):
			hasArticle = true
		case reflect.TypeOf(&blogmodels.TagModel{}):
			hasTag = true
		case reflect.TypeOf(&blogmodels.ArticleTagModel{}):
			hasArticleTag = true
		case reflect.TypeOf(&blogmodels.UserModel{}):
			hasUser = true
		case reflect.TypeOf(&blogmodels.UserConfModel{}):
			hasUserConf = true
		case reflect.TypeOf(&blogmodels.UserStatModel{}):
			hasUserStat = true
		case reflect.TypeOf(&blogmodels.UserSessionModel{}):
			hasUserSession = true
		}
	}

	if hasUser {
		if !hasUserConf {
			list = append(list, &blogmodels.UserConfModel{})
		}
		if !hasUserStat {
			list = append(list, &blogmodels.UserStatModel{})
		}
		if !hasUserSession {
			list = append(list, &blogmodels.UserSessionModel{})
		}
	}

	if hasArticle {
		if !hasTag {
			list = append(list, &blogmodels.TagModel{})
		}
		if !hasArticleTag {
			list = append(list, &blogmodels.ArticleTagModel{})
		}
	}
	return list
}

func NewJSONRequest(method, target, body string) *http.Request {
	req, _ := http.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func IssueAccessToken(t *testing.T, user *blogmodels.UserModel) string {
	t.Helper()
	sessionID, err := db_service.NextSnowflakeID()
	if err != nil {
		t.Fatalf("生成测试会话ID失败: %v", err)
	}
	now := time.Now()
	session := blogmodels.UserSessionModel{
		Model: blogmodels.Model{
			ID: sessionID,
		},
		UserID:           user.ID,
		RefreshTokenHash: fmt.Sprintf("test-refresh-%s", sessionID.String()),
		IP:               "127.0.0.1",
		Addr:             "本地测试",
		UA:               "codex-test",
		LastSeenAt:       &now,
		ExpiresAt:        now.Add(24 * time.Hour),
	}
	if testDB == nil {
		t.Fatal("测试数据库未初始化，请先调用 SetupSQLite")
	}
	if err = testDB.Create(&session).Error; err != nil {
		t.Fatalf("创建测试会话失败: %v", err)
	}

	token, err := jwts.GetToken(testConfig.Jwt, jwts.Claims{
		UserID:       user.ID,
		SessionID:    session.ID,
		TokenVersion: effectiveTokenVersion(user),
		Username:     user.Username,
		Role:         user.Role,
	})
	if err != nil {
		t.Fatalf("签发测试访问令牌失败: %v", err)
	}
	return token
}

func effectiveTokenVersion(user *blogmodels.UserModel) uint32 {
	if user == nil || user.TokenVersion == 0 {
		return 1
	}
	return user.TokenVersion
}

func NewClaims(user *blogmodels.UserModel) *jwts.MyClaims {
	if user == nil {
		return &jwts.MyClaims{}
	}
	return &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:       user.ID,
			SessionID:    ctype.ID(0),
			TokenVersion: effectiveTokenVersion(user),
			Username:     user.Username,
			Role:         user.Role,
		},
	}
}
