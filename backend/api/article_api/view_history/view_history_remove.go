package view_history

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func (h ViewHistoryApi) ArticleViewHistoryRemoveView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)
	claims := jwts.GetClaimsByGin(c)

	var list []models.UserArticleViewHistoryModel
	if err := h.App.DB.Find(&list, "user_id = ? and article_id IN ?", claims.UserID, cr.IDList).Error; err != nil {
		res.FailWithError(err, c)
		return
	}
	if len(list) > 0 {
		if err := h.App.DB.Delete(&list).Error; err != nil {
			res.FailWithMsg(fmt.Sprintf("删除访问历史失败:%v", err), c)
			return
		}
	}

	res.OkWithMsg(fmt.Sprintf("访问历史删除成功，共%d条", len(list)), c)
}
