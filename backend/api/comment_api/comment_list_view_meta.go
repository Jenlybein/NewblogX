package comment_api

import (
	"myblogx/models/ctype"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// commentViewerIDFromGin 提取当前可选登录用户。
// 评论列表接口允许匿名访问，所以这里不能强制要求鉴权成功。
func commentViewerIDFromGin(c *gin.Context) ctype.ID {
	if claims, err := jwts.ParseTokenByGin(c); err == nil && claims != nil {
		return claims.UserID
	}
	return 0
}
