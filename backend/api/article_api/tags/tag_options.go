package tags

import (
	"myblogx/common/res"
	"myblogx/models"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

func (h TagsApi) ArticleTagOptionsView(c *gin.Context) {
	var list []models.OptionsResponse[ctype.ID]
	if err := h.App.DB.Model(&models.TagModel{}).
		Where("is_enabled = ?", true).
		Order("sort desc, id asc").
		Select("id as id", "title as title", "title as label", "id as value").
		Scan(&list).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithData(list, c)
}
