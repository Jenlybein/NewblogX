package site_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_site"

	"github.com/gin-gonic/gin"
)

// 获取站点基本配置信息-任何用户
func (h SiteApi) SiteInfoView(c *gin.Context) {
	cr := middleware.GetBindUri[SiteInfoRequest](c)
	runtimeSite := h.App.RuntimeSite
	if runtimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}

	var data any

	switch cr.Name {
	// 站点版本
	case "site":
		redis_site.SetFlow(redis_service.NewDeps(h.App.Redis, h.App.Logger))
		rep := runtimeSite.GetRuntimeSite()
		rep.About.Version = h.App.Version
		data = rep
	case "seo":
		site := runtimeSite.GetRuntimeSite()
		data = SiteSEOResponse{
			SiteTitle:    site.SiteInfo.Title,
			ProjectTitle: site.Project.Title,
			Logo:         site.SiteInfo.Logo,
			Icon:         site.Project.Icon,
			Keywords:     site.Seo.Keywords,
			Description:  site.Seo.Description,
		}
	default:
		res.FailWithMsg("站点信息不存在", c)
		return
	}

	res.OkWithData(data, c)
}

// 获取站点配置信息-管理员
func (h SiteApi) SiteInfoAdminView(c *gin.Context) {
	cr := middleware.GetBindUri[SiteInfoRequest](c)
	runtimeSite := h.App.RuntimeSite
	if runtimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}

	var data any

	switch cr.Name {
	case "site":
		rep := runtimeSite.GetRuntimeSite()
		rep.About.Version = h.App.Version
		data = rep
	case "ai":
		rep := runtimeSite.GetRuntimeAI()
		rep.SecretKey = sensitive_place_holder
		data = rep
	default:
		res.FailWithMsg("站点信息不存在", c)
		return
	}

	res.OkWithData(data, c)
}
