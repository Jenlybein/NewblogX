// Package page 是分页请求的共享参数。
//
// 从 common/list_query.go 搬来（M6）。原来它和一套 gorm 查询构造器（ListQuery /
// CountQuery / PageIDQuery）挤在一个包里，但那套构造器**全仓零引用**
// ——它们是被新的模块化仓储取代之后留下的死代码，已删除；只留下 PageInfo，
// 因为 analytics 的 ClickHouse 查询还在用它做分页参数。
//
// 放在 kernel 而不是 platform/http：用它的不只是 HTTP 层（ClickHouse 查询也嵌它），
// 而 kernel 就是"各层都可以引用的共享类型"这一层。
package page

// PageInfo 是分页参数。
//
// 字段带 form tag，因为它直接作为 query 参数结构被 binding 绑定。
type PageInfo struct {
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
	Key   string `form:"key"`
	Order string `form:"order"` // 允许前端覆盖排序字段
}

// NormalizePage 把非法页码收敛到 1。
func (p PageInfo) NormalizePage() int {
	pg := p.Page
	if pg <= 0 {
		return 1
	}
	return pg
}

// GetPage 返回夹在 [1, 最大页] 内的页码。
//
// 为什么需要夹：前端可能传 page=9999 而总数只有 3 页，
// 不夹的话算出的 offset 越界，SQL 白跑一趟还返回空页。
func (p PageInfo) GetPage(count int) int {
	pg := p.NormalizePage()

	if count <= 0 {
		return 1
	}

	limit := p.GetLimit()
	max := (count + limit - 1) / limit
	if pg > max {
		return max
	}
	return pg
}

// GetLimit 返回每页条数，上限 100、非法值回落到 10。
//
// 上限是硬约束：前端传 limit=100000 等于让一次查询拉全表。
func (p PageInfo) GetLimit() int {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	return p.Limit
}

// GetOffset 在已知总数时算偏移（页码会被 GetPage 夹住）。
func (p PageInfo) GetOffset(count int) int {
	return (p.GetPage(count) - 1) * p.GetLimit()
}

// GetOffsetNoCount 在不知道总数时算偏移（页码只做下界收敛，不夹上界）。
func (p PageInfo) GetOffsetNoCount() int {
	return (p.NormalizePage() - 1) * p.GetLimit()
}
