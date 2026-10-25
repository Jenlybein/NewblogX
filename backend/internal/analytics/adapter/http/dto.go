package http

import "myblogx/internal/kernel/ctype"

// IDRequest 是路径里的数字 ID。
type IDRequest struct {
	ID ctype.ID `uri:"id"`
}
