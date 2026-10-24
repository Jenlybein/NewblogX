package http

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"myblogx/common/res"
	"myblogx/internal/apperr"
	"myblogx/utils/jwts"
)

// userIDFrom 取当前登录用户；未登录直接写响应并返回 false。
func userIDFrom(c *gin.Context) (uint64, bool) {
	cl := jwts.GetClaimsByGin(c)
	if cl == nil {
		res.FailWithMsg("请先登录", c)
		return 0, false
	}
	return uint64(cl.UserID), true
}

// pathID 解析路径里的数字 ID。
func pathID(c *gin.Context) (uint64, bool) {
	raw := c.Param("id")
	var id uint64
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch < '0' || ch > '9' {
			res.FailWithMsg("参数错误", c)
			return 0, false
		}
		id = id*10 + uint64(ch-'0')
	}
	if id == 0 {
		res.FailWithMsg("参数错误", c)
		return 0, false
	}
	return id, true
}

// apperrObjectKeyMissing 回调缺少对象 key。
func apperrObjectKeyMissing() error { return apperr.New(apperr.CodeImageObjectKeyMissing) }

// sprintfDelete 删除成功的提示文案。
func sprintfDelete(count int) string {
	return fmt.Sprintf("操作成功，删除了 %d 张图片", count)
}
