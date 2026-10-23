package ai_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/service/ai_service/ai_metainfo"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

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
