package mysql

import (
	"testing"

	"myblogx/internal/article/app"
)

// 本文件是**装配层契约**：生产仓储必须满足 app 层定义的每一个端口。
//
// 为什么单独一条编译期断言：分组端口（GroupRepositoryFor）第一次落地时，
// 只有 apptest 的 fake 实现了它，生产 repos 漏了 —— 单测全绿，线上每个分组
// 端点都返回 not_found。端口由使用方定义、实现方在别的包，这种"只差一个方法"
// 的缺口不会有任何编译错误提醒，只能靠断言钉住。
func TestReposSatisfyAppPorts(t *testing.T) {
	t.Parallel()

	// 直接对类型断言（不需要真数据库：这里验的是"方法集齐全"，不是行为）。
	var (
		_ app.Repos                  = (*repos)(nil)
		_ app.NamedOutlineRepository = (*repos)(nil)
		_ app.GroupRepositoryFor     = (*repos)(nil)
	)
}
