// 站点API模块

package site_api

import (
	"myblogx/common/res"
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Deps struct {
	Version     string
	Logger      *logrus.Logger
	Redis       *redis.Client
	QQ          conf.QQ
	RuntimeSite *site_service.RuntimeConfigService
}

type SiteApi struct {
	App Deps
}

func New(deps Deps) SiteApi {
	return SiteApi{App: deps}
}

// 敏感信息占位符
var sensitive_place_holder = "******"

// 站点 qq 登录地址
// @Summary 获取 QQ 登录地址
// @Tags site
// @Produce json
// @Success 200 {object} object{code=int,data=string,msg=string}
// @Router /site/qq_url [get]
func (h SiteApi) SiteInfoQQView(c *gin.Context) {
	res.OkWithData(h.App.QQ.Url(), c)
}

// AI 信息获取
// @Summary 获取站点 AI 信息
// @Tags site
// @Produce json
// @Success 200 {object} object{code=int,data=SiteAIResponse,msg=string}
// @Router /site/ai_info [get]
func (h SiteApi) SiteInfoAIView(c *gin.Context) {
	if h.App.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	ai := h.App.RuntimeSite.GetRuntimeAI()
	res.OkWithData(SiteAIResponse{
		Enable:   ai.Enable,
		Nickname: ai.Nickname,
		Avatar:   ai.Avatar,
		Abstract: ai.Abstract,
	}, c)
}

// SEO 信息获取，适合 Nuxt 等前端在服务端渲染或页面切换时拉取。
// @Summary 获取站点 SEO 信息
// @Tags site
// @Produce json
// @Success 200 {object} object{code=int,data=SiteSEOResponse,msg=string}
// @Router /site/seo [get]
func (h SiteApi) SiteSEOView(c *gin.Context) {
	if h.App.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	site := h.App.RuntimeSite.GetRuntimeSite()
	res.OkWithData(SiteSEOResponse{
		SiteTitle:    site.SiteInfo.Title,
		ProjectTitle: site.Project.Title,
		Logo:         site.SiteInfo.Logo,
		Icon:         site.Project.Icon,
		Keywords:     site.Seo.Keywords,
		Description:  site.Seo.Description,
	}, c)
}
