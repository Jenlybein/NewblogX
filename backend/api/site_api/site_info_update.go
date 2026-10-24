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
// @Summary 更新站点运行时配置（管理员）
// @Tags site
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param name path string true "配置名称：site 站点配置、ai AI 配置"
// @Param body body object true "请求体，name=site 时为 conf.Site，name=ai 时为 conf.AI"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /site/{name} [put]
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
			ActionName: "site_runtime_update",
			TargetType: "runtime_site_config",
			TargetID:   "site",
			Success:    true,
			Message:    "更新站点运行时配置成功",
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
		}
	default:
		res.FailWithMsg("站点信息不存在", c)
		return
	}

	res.OkWithMsg("站点配置更新成功", c)
	middleware.EmitActionAuditFromGin(c, auditInput)
}
