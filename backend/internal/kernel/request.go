// Package kernel 放跨模块共用的值对象与请求结构。
//
// 这里的代码必须是"零业务规则"的：一旦出现"只有文章能用"或"只有用户能用"
// 的判断，它就属于某个模块，不该放在这里。
package kernel

import (
	"myblogx/internal/kernel/ctype"
)

// IDRequest 是"路径里带一个 ID"的通用绑定结构。
//
// 从 models 搬过来（S4.1）。它不含任何用户/文章语义，纯粹是 HTTP 绑定的形状，
// 所以适合放 kernel：否则每个模块都要复制一份完全相同的结构体。
type IDRequest struct {
	ID ctype.ID `json:"id" form:"id" uri:"id" binding:"required"`
}

// IDListRequest 是"请求体里带一批 ID"的通用绑定结构。
type IDListRequest struct {
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

// PagedRequest 是通用分页参数。
type PagedRequest struct {
	Page int `json:"page" form:"page"`
	Size int `json:"size" form:"size"`
}

// Normalize 把分页参数收敛到安全范围（与 domain.Query.Normalize 保持一致的口径）。
func (p PagedRequest) Normalize() PagedRequest {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Size <= 0 {
		p.Size = 20
	}
	if p.Size > 100 {
		p.Size = 100
	}
	return p
}
