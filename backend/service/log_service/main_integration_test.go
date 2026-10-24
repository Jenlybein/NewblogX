//go:build integration

package log_service

import (
	"context"
	"os"
	"testing"

	"myblogx/internal/testkit/chcontainer"
)

// TestMain 在整包测试结束时终止 testcontainers 起的 ClickHouse。
func TestMain(m *testing.M) {
	code := m.Run()
	chcontainer.Terminate(context.Background())
	os.Exit(code)
}
