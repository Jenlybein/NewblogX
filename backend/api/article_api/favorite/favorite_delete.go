package favorite

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// 删除收藏夹
func (h FavoriteApi) FavoriteDeleteView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	if len(cr.IDList) == 0 {
		res.FailWithMsg("请填入要删除的 id 列表", c)
		return
	}

	query := h.App.DB.Where("id IN ?", cr.IDList)

	claim := jwts.GetClaimsByGin(c)
	if claim.IsAdmin() == false {
		query = query.Where("user_id = ?", claim.UserID)
	}

	var list []models.FavoriteModel
	if err := h.App.DB.Where(query).Find(&list).Error; err != nil {
		h.App.Logger.Errorf("查找对应收藏夹失败: 错误=%v", err)
		res.FailWithMsg("寻找对应的收藏夹失败", c)
		return
	}

	if len(list) > 0 {
		if err := h.App.DB.Delete(&list).Error; err != nil {
			h.App.Logger.Errorf("删除对应收藏夹失败: 错误=%v", err)
			res.FailWithMsg("删除收藏夹失败", c)
			return
		}
	} else {
		res.FailWithMsg("未找到需删除的收藏夹", c)
		return
	}

	res.OkWithMsg(fmt.Sprintf("删除收藏夹成功，共删除 %d 条", len(list)), c)
}
