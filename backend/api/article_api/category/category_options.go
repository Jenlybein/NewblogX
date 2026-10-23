package category

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

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
