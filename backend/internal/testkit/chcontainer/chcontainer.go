//go:build integration

// Package chcontainer 用 testcontainers 起一个 ClickHouse，供集成测试使用。
//
// 用法：
//
//	addr := chcontainer.Addr(t)          // 拿到 host:9000
//	chcontainer.Terminate(ctx)           // 可选：TestMain 里统一收尾
//
// 优先级：BLOGX_CH_ADDR 环境变量 > testcontainers 自动启动。
// 没有 Docker 时 t.Skip，不阻断本地普通开发。
package chcontainer

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcch "github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

var (
	once      sync.Once
	addr      string
	container *tcch.ClickHouseContainer
	startErr  error
)

// Addr 返回 ClickHouse 的 native 地址（host:9000）。
//
// 同一个测试进程内只起一个容器（sync.Once）；不同测试包是不同进程，各自独立。
func Addr(t *testing.T) string {
	t.Helper()

	if a := os.Getenv("BLOGX_CH_ADDR"); a != "" {
		return a
	}

	once.Do(func() {
		addr, container, startErr = start()
	})
	if startErr != nil {
		t.Skipf("Docker/testcontainers 不可用，跳过集成测试: %v", startErr)
	}
	return addr
}

// Terminate 终止容器；建议在 TestMain 里 defer 调用。
// 未启动容器（例如用了 BLOGX_CH_ADDR）时是 no-op。
func Terminate(ctx context.Context) {
	if container == nil {
		return
	}
	_ = testcontainers.TerminateContainer(container)
}

func start() (string, *tcch.ClickHouseContainer, error) {
	ctx := context.Background()

	initSQL, err := findInitSQL()
	if err != nil {
		return "", nil, err
	}

	// 显式置空密码：模块默认把 default 用户的密码设成 "default"，
	// 而项目里的连接（含本地手搓容器）一直是空密码，这里保持一致。
	ctr, err := tcch.Run(ctx, "clickhouse/clickhouse-server:25.3",
		tcch.WithDatabase("blogx_logs"),
		tcch.WithPassword(""),
		tcch.WithInitScripts(initSQL),
	)
	if err != nil {
		return "", nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", ctr, err
	}
	port, err := ctr.MappedPort(ctx, "9000/tcp")
	if err != nil {
		return "", ctr, err
	}
	return net.JoinHostPort(host, port.Port()), ctr, nil
}

// findInitSQL 从本文件位置向上找仓库根的建表脚本。
//
// 不依赖 cwd：go test 的工作目录是包目录，直接拼相对路径会找不到仓库根。
func findInitSQL() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位 chcontainer 源文件")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "deploy", "docker", "clickhouse", "init", "01-logs.sql")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("找不到 deploy/docker/clickhouse/init/01-logs.sql")
}
