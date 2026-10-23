package log_service

import (
	"flag"
	"path/filepath"
	"runtime"
)

// ResolveLogDir 统一解析日志目录，避免测试在包工作目录下写出散落日志。
func ResolveLogDir(dir string) string {
	if dir != "" {
		return dir
	}
	if runningUnderGoTest() {
		return filepath.Join(projectRoot(), "logs", "test_logs")
	}
	return "./logs"
}

// ResolveLogApp 统一解析日志应用名，避免空应用名生成 .log 文件。
func ResolveLogApp(app string, defaultApp string) string {
	if app != "" {
		return app
	}
	if defaultApp != "" {
		return defaultApp
	}
	if runningUnderGoTest() {
		return "test"
	}
	return "app"
}

// runningUnderGoTest 判断当前进程是否运行在 go test 环境下。
func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}

// projectRoot 返回项目根目录，用于测试场景下推导稳定的日志输出目录。
func projectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}
