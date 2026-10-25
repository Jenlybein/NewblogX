package http

import "myblogx/models/ctype"

// IDRequest 是路径里的数字 ID。
type IDRequest struct {
	ID ctype.ID `uri:"id"`
}
