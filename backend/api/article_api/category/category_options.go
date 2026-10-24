package category

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

// CategoryOptionsView 查询分类下拉选项。
// @Summary 分类选项
// @Tags article
// @Produce json
// @Param user_id query int true "用户 ID"
// @Success 200 {object} object{code=int,data=[]object{id=int,title=string,label=string,value=int},msg=string}
// @Router /articles/category/options [get]
func (h CategoryApi) CategoryOptionsView(c *gin.Context) {
	cr := middleware.GetBindQuery[CategoryOptionsRequest](c)
	var list []models.OptionsResponse[ctype.ID]
	if err := h.App.DB.Model(&models.CategoryModel{}).
		Where("user_id = ?", cr.UserID).
		Order("title asc, id asc").
		Select("id as id", "title as title", "title as label", "id as value").
		Scan(&list).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithData(list, c)
}
