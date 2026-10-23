package site_api

import (
	"myblogx/common/res"
	"myblogx/conf"
	"myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// 数据结构映射表
var confMap = map[string]any{
	"site": &conf.Site{},
	"ai":   &conf.AI{},
}

// 更新站点运行时配置。
// 这里只允许修改数据库中的运行时配置，不再修改 settings.yaml 和前端 HTML 文件。
func (h SiteApi) SiteUpdateView(c *gin.Context) {
	cr := middleware.GetBindUri[SiteInfoRequest](c)
	runtimeSite := h.App.RuntimeSite
	if runtimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	var auditInput middleware.GinAuditInput

	targetStruct, ok := confMap[cr.Name]
	if !ok {
		res.FailWithMsg("站点信息不存在", c)
		return
	}
	if err := c.ShouldBindJSON(targetStruct); err != nil {
		res.FailWithError(err, c)
		return
	}

	switch s := targetStruct.(type) {
	case *conf.Site:
		if err := runtimeSite.UpdateRuntimeSite(*s); err != nil {
			res.FailWithError(err, c)
			return
		}
		auditInput = middleware.GinAuditInput{
			ActionName:        "site_runtime_update",
			TargetType:        "runtime_site_config",
			TargetID:          "site",
			Success:           true,
			Message:           "更新站点运行时配置成功",
			RequestBody:       *s,
			UseRawRequestBody: true,
			UseRawRequestHead: true,
		}
	case *conf.AI:
		current := runtimeSite.GetRuntimeAI()
		if s.SecretKey == sensitive_place_holder {
			s.SecretKey = current.SecretKey
		}
		if err := runtimeSite.UpdateRuntimeAI(*s); err != nil {
			res.FailWithError(err, c)
			return
		}
		auditInput = middleware.GinAuditInput{
			ActionName: "site_runtime_update",
			TargetType: "runtime_site_config",
			TargetID:   "ai",
			Success:    true,
			Message:    "更新 AI 运行时配置成功",
			RequestBody: map[string]any{
				"enable":     s.Enable,
				"model":      s.ChatModel,
				"base_url":   s.BaseURL,
				"nickname":   s.Nickname,
				"avatar":     s.Avatar,
				"abstract":   s.Abstract,
				"secret_key": "***",
			},
			UseRawRequestBody: true,
			UseRawRequestHead: true,
		}
	default:
		res.FailWithMsg("站点信息不存在", c)
		return
	}

	res.OkWithMsg("站点配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, auditInput)
}
