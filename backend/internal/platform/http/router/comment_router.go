package router

import (
	commenthttp "myblogx/internal/comment/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// commentHandler 由 app.WireComment 在启动时注入。
var commentHandler *commenthttp.Handler

// SetCommentHandler 注入评论处理器。
func SetCommentHandler(h *commenthttp.Handler) { commentHandler = h }

// CommentRouter 注册评论路由。
//
// 管理端「评论管理」（后台「文章管理 → 评论管理」第一期）单独挂在 adminGroup 上：
// 列表能看到已删除评论与"谁说了什么"，删除 / 禁言 / 封禁都是账号级处置，
// 全段只给管理员。写接口挂 CaptureLog(mw.ReqBody)：请求体只在失败请求时进审计，
// 是排查"后台点了删除却报错"时唯一能对上的原始输入。
func CommentRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if commentHandler == nil {
		panic("router: 评论处理器未注入（应在 app.WireComment 中调用 SetCommentHandler）")
	}
	app := commentHandler

	Group := r.Group("comments")
	authGroup := Group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)

	Group.GET("", app.ListRoot)
	Group.GET("replies", app.ListReplies)
	authGroup.POST("", app.Create)
	authGroup.POST(":id/digg", app.Digg)
	authGroup.DELETE(":id", app.Remove)

	// 静态段 admin/* 与参数段 :id/* 在同一位置共存：gin 优先匹配静态段，
	// 所以 admin/mute 不会被当成 :id = "admin" 的路径参数。
	adminGroup.GET("admin/list", app.AdminList)
	adminGroup.DELETE("admin", mw.CaptureLog(mw.ReqBody), app.AdminRemoveMany)
	adminGroup.POST("admin/mute", mw.CaptureLog(mw.ReqBody), app.AdminMute)
	adminGroup.POST("admin/ban", mw.CaptureLog(mw.ReqBody), app.AdminBan)
}
