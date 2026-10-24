package ai_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/service/ai_service/ai_metainfo"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// AIArticleMetaInfoView 根据正文生成文章标题、摘要、分类与标签。
// @Summary 生成文章元信息
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body AIBaseRequest true "请求体"
// @Success 200 {object} object{code=int,data=ai_metainfo.MetainfoResponse,msg=string}
// @Router /ai/metainfo [post]
func (h AIApi) AIArticleMetaInfoView(c *gin.Context) {
	app := h.App
	if app.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	aiConf := app.RuntimeSite.GetRuntimeAI()

	cr := middleware.GetBindJson[AIBaseRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	data, err := ai_metainfo.GenerateArticleMetainfo(app.DB, app.Logger, aiConf, claims.UserID, cr.Content)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithData(data, c)
}
