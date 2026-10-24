package tags

import (
	"myblogx/common/res"
	"myblogx/models"
	"myblogx/models/ctype"

	"github.com/gin-gonic/gin"
)

// ArticleTagOptionsView 查询启用中的标签下拉选项。
// @Summary 标签选项
// @Tags article
// @Produce json
// @Success 200 {object} object{code=int,data=[]object{id=int,title=string,label=string,value=int},msg=string}
// @Router /articles/tags/options [get]
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
