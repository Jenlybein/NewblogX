package image_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

// ImageListView 图片列表（管理员）。
// @Summary 图片列表
// @Tags image
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Success 200 {object} object{code=int,data=object{list=[]models.ImageModel,count=int},msg=string}
// @Router /images [get]
func (h ImageApi) ImageListView(c *gin.Context) {
	cr := middleware.GetBindQuery[common.PageInfo](c)

	list, count, err := common.ListQuery(models.ImageModel{}, common.Options{
		DB:       h.App.DB,
		PageInfo: cr,
		Likes:    []string{"file_name", "object_key"},
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, count, c)
}
