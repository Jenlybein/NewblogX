//go:build integration

// Package escontainer 用 testcontainers 起一个 Elasticsearch 8，供真实索引集成测试使用。
//
// 为什么不复用 mock ES：删除链路的缺陷恰恰在"mock 永远返回成功"时看不见——
// 真实 ES 才会在文档不存在时给 404，也才会验证 bulk 的 delete 动作真的落盘。
//
// 用法：
//
//	addr := escontainer.Addr(t)
//	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{addr}})
//
// 优先级：BLOGX_ES_ADDR 环境变量 > testcontainers 自动启动。
// 没有 Docker 时 t.Skip，不阻断本地普通开发。
package escontainer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// image 固定版本而不是 latest：mapping / 分析器行为随大版本变化，
	// 升级镜像必须是一次显式动作。
	image = "docker.elastic.co/elasticsearch/elasticsearch:8.15.3"
)

// Addr 返回可访问的 ES HTTP 地址（如 http://127.0.0.1:49153）。
//
// 外部实例优先：CI 若已有 ES 服务，设 BLOGX_ES_ADDR 即可省掉容器启动开销。
func Addr(t *testing.T) string {
	t.Helper()

	if shared := strings.TrimSpace(os.Getenv("BLOGX_ES_ADDR")); shared != "" {
		if strings.HasPrefix(shared, "http://") || strings.HasPrefix(shared, "https://") {
			return shared
		}
		return "http://" + shared
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"9200/tcp"},
			Env: map[string]string{
				"discovery.type":         "single-node",
				"xpack.security.enabled": "false",
				// 测试机不需要 1g 堆；小一点起得更快，也不容易把 CI 内存打爆。
				"ES_JAVA_OPTS": "-Xms512m -Xmx512m",
			},
			WaitingFor: wait.ForHTTP("/").WithPort("9200/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("Docker/testcontainers 不可用，跳过真实 ES 集成测试: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Logf("终止 ES 容器失败: %v", err)
		}
	})

	// Endpoint 的第一个参数是协议（"" 返回 host:port，"http" 返回完整 URL），
	// 不是 "9200/tcp"——传端口会把端口当协议，得到 "9200/tcp:xxxxx" 这种地址。
	endpoint, err := ctr.Endpoint(ctx, "http")
	if err != nil {
		t.Fatalf("取 ES 容器地址失败: %v", err)
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	return endpoint
}
