package router

import (
	"myblogx/api"
	"myblogx/api/ai_api"
	mw "myblogx/middleware"

	"github.com/gin-gonic/gin"
)

func AIRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	app := appContainer.AIApi

	group := r.Group("ai")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	//adminGroup := authGroup.Group("", mw.AdminMiddleware)

	group.POST("scoring/article", runtimeMw.OptionalAuthMiddleware, mw.BindJson[ai_api.AIArticleScoringRequest], app.AIArticleScoringView)
	authGroup.POST("metainfo", mw.BindJson[ai_api.AIBaseRequest], app.AIArticleMetaInfoView)
	authGroup.POST("overwrite", mw.BindJson[ai_api.AIOverwriteRequest], app.AIOverwriteView)
	authGroup.POST("diagnose", mw.BindJson[ai_api.AIDiagnoseRequest], app.AIDiagnoseView)
	authGroup.POST("search/list", mw.BindJson[ai_api.AIBaseRequest], app.AIArticleSearchListView)
	authGroup.POST("search/llm", mw.BindJson[ai_api.AIBaseRequest], app.AIArticleSearchLLMView)
}
