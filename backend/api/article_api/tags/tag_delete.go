package tags

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

// TagDeleteView 批量删除标签。
// @Summary 批量删除标签
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "标签 ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/tags [delete]
func (h TagsApi) TagDeleteView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)
	if len(cr.IDList) == 0 {
		res.FailWithMsg("请输入要删除的标签 id 列表", c)
		return
	}

	var relationCount int64
	if err := h.App.DB.Model(&models.ArticleTagModel{}).
		Where("tag_id IN ?", cr.IDList).
		Count(&relationCount).Error; err != nil {
		res.FailWithError(err, c)
		return
	}
	if relationCount > 0 {
		res.FailWithMsg("标签已被文章使用，无法直接删除", c)
		return
	}

	var list []models.TagModel
	if err := h.App.DB.Where("id IN ?", cr.IDList).Find(&list).Error; err != nil {
		res.FailWithMsg("查询标签失败", c)
		return
	}
	if len(list) == 0 {
		res.FailWithMsg("未找到需要删除的标签", c)
		return
	}

	if err := h.App.DB.Delete(&list).Error; err != nil {
		res.FailWithMsg("删除标签失败", c)
		return
	}
	res.OkWithMsg(fmt.Sprintf("删除标签成功，共删除 %d 条", len(list)), c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "tag_delete",
		TargetType: "tag",
		Success:    true,
		Message:    fmt.Sprintf("删除标签成功，共删除 %d 条", len(list)),
	})
}
