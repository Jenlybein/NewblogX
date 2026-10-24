//go:build integration

package persist

import (
	"context"
	"os"
	"testing"

	"myblogx/internal/testkit/chcontainer"
)

// TestMain 在整包测试结束时终止 testcontainers 起的 ClickHouse。
// 未启用容器（如设置了 BLOGX_CH_ADDR）时 Terminate 是 no-op。
func TestMain(m *testing.M) {
	code := m.Run()
	chcontainer.Terminate(context.Background())
	os.Exit(code)
}
