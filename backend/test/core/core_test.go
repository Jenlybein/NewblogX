package core_test

import (
	"myblogx/conf"
	"myblogx/core"
	"myblogx/test/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadAndSetCfg(t *testing.T) {
	testutil.InitGlobals()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "settings.yaml")
	orig := `system:
  ip: "127.0.0.1"
  port: 8080
jwt:
  expire: 2
  secret: "abc"
  issuer: "blogx"
`
	if err := os.WriteFile(cfgFile, []byte(orig), 0644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	cfg := core.ReadCfg(&cfgFile)
	if cfg.System.Port != 8080 {
		t.Fatalf("ReadCfg 读取错误: %d", cfg.System.Port)
	}

	cfg.System.Port = 9090
	core.SetCfg(cfg, &cfgFile, testutil.Logger())
	cfg2 := core.ReadCfg(&cfgFile)
	if cfg2.System.Port != 9090 {
		t.Fatalf("SetCfg 写回失败: %d", cfg2.System.Port)
	}
}

func TestReadCfgExpandEnv(t *testing.T) {
	testutil.InitGlobals()
	t.Setenv("BLOGX_TEST_IP", "127.0.0.1")
	t.Setenv("BLOGX_TEST_PORT", "9091")
	t.Setenv("BLOGX_TEST_EXPIRE", "2")
	t.Setenv("BLOGX_TEST_SECRET", "env-secret")
	t.Setenv("BLOGX_TEST_ISSUER", "blogx")

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "settings.yaml")
	orig := `system:
  ip: "${BLOGX_TEST_IP}"
  port: ${BLOGX_TEST_PORT}
jwt:
  expire: ${BLOGX_TEST_EXPIRE}
  secret: "${BLOGX_TEST_SECRET}"
  issuer: "${BLOGX_TEST_ISSUER}"
`
	if err := os.WriteFile(cfgFile, []byte(orig), 0644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	cfg := core.ReadCfg(&cfgFile)
	if cfg.System.IP != "127.0.0.1" {
		t.Fatalf("字符串环境变量展开失败: %q", cfg.System.IP)
	}
	if cfg.System.Port != 9091 {
		t.Fatalf("环境变量端口展开失败: %d", cfg.System.Port)
	}
	if cfg.Jwt.Expire != 2 {
		t.Fatalf("数值环境变量展开失败: %d", cfg.Jwt.Expire)
	}
	if cfg.Jwt.Secret != "env-secret" {
		t.Fatalf("字符串环境变量展开失败: %q", cfg.Jwt.Secret)
	}
	if cfg.Jwt.Issuer != "blogx" {
		t.Fatalf("字符串环境变量展开失败: %q", cfg.Jwt.Issuer)
	}
}

func TestReadCfgPanicOnMissingEnv(t *testing.T) {
	testutil.InitGlobals()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "settings.yaml")
	orig := `system:
  port: ${BLOGX_TEST_MISSING_PORT}
`
	if err := os.WriteFile(cfgFile, []byte(orig), 0644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("ReadCfg 缺少环境变量时应 panic")
		}
	}()
	_ = core.ReadCfg(&cfgFile)
}

func TestReadCfgPanicOnMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	defer func() {
		if recover() == nil {
			t.Fatal("ReadCfg 读取不存在文件应 panic")
		}
	}()
	_ = core.ReadCfg(&missing)
}

func TestInitLogrusCreatesDailyFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "blogx-log-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	logger := core.InitLogrus(&conf.Logrus{
		App:   "blogx-test",
		Dir:   dir,
		Level: "info",
	}, &conf.System{ServerID: 1})
	logger.Info("hello-log")

	logFile := filepath.Join(dir, "runtime_logs", time.Now().Format("2006-01-02"), "blogx-test.log")
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(b), "hello-log") {
		t.Fatalf("日志内容未写入: %s", string(b))
	}
}
