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

// TestInitIPDBLoadsRealLibrary 锁住一个曾经真实存在过的缺陷。
//
// 历史：`InitIPDB` 从未被调用，searcher 一直是 nil，于是 `GetAddr` 对任何
// 真实 IP 都返回「未知地址」——登录日志和用户列表的 addr 字段全是这四个字，
// 而仓库里的两个 xdb 库从没被加载过。
//
// 这条测试的判据是「公网 IP 能解析出真实归属地」，而不只是「返回非空」：
// 后者在库没加载时也会因为兜底分支而通过，正是它让缺陷藏了这么久。
func TestInitIPDBLoadsRealLibrary(t *testing.T) {
	testutil.InitGlobals()

	if err := core.InitIPDB(); err != nil {
		t.Fatalf("加载 IP 库失败: %v", err)
	}

	// 114.114.114.114 是国内公共 DNS，归属地稳定为江苏·南京一带。
	// 只断言「不是兜底值」，避免依赖具体城市名而变脆。
	if got := core.GetIpAddr("114.114.114.114"); got == "未知地址" || got == "" {
		t.Fatalf("库已加载但公网 IP 仍解析不出归属地, got=%q", got)
	}
}
