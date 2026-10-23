package core_test

import (
	"myblogx/core"
	"myblogx/test/testutil"
	"testing"
)

func TestGetIpAddrBranches(t *testing.T) {
	testutil.InitGlobals()

	if got := core.GetIpAddr("bad-ip"); got == "" {
		t.Fatalf("非法 IP 应返回兜底字符串, got=%q", got)
	}

	if got := core.GetIpAddr("127.0.0.1"); got == "" {
		t.Fatalf("本地 IP 应返回内网地址提示, got=%q", got)
	}
}
