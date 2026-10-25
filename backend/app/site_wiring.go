package app

import (
	"context"

	"myblogx/conf"
	"myblogx/internal/apperr"
	mediamysql "myblogx/internal/media/adapter/mysql"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/router"
	sitehttp "myblogx/internal/site/adapter/http"
	sitemysql "myblogx/internal/site/adapter/mysql"
	siteapp "myblogx/internal/site/app"

	"gorm.io/gorm"
)

// WireSite 装配站点模块，并注入路由处理器。
func WireSite(infra *Infra) {
	if infra == nil {
		return
	}
	svc := siteapp.New(
		sitemysql.NewBanner(infra.DB),
		&siteImageResolver{db: infra.DB},
		siteRuntimeBridge{inner: infra.RuntimeSite},
		infra.Version,
		infra.Config.QQ.Url(),
		infra.Logger,
	)
	router.SetSiteHandler(sitehttp.NewHandler(svc))
}

// siteImageResolver 把 media 模块的图片查询适配成站点模块的端口。
type siteImageResolver struct {
	db *gorm.DB
}

// URLByID 由图片 ID 取访问地址。
func (r *siteImageResolver) URLByID(_ context.Context, id uint64) (string, error) {
	return mediamysql.ResolveURLByID(r.db, id)
}

// IDsByURLs 批量由 URL 反查图片 ID。
func (r *siteImageResolver) IDsByURLs(_ context.Context, urls []string) (map[string]uint64, error) {
	out := make(map[string]uint64, len(urls))
	for _, url := range urls {
		id, ok, err := mediamysql.FindIDByURL(r.db, url)
		if err != nil {
			return nil, err
		}
		if ok {
			out[url] = id
		}
	}
	return out, nil
}

// siteRuntimeBridge 把 platform/config 的服务适配成站点模块的端口。
//
// 平台包不依赖业务 domain，转换放在装配层。
type siteRuntimeBridge struct {
	inner *sitecfg.RuntimeConfigService
}

// Site 返回当前站点配置。
func (b siteRuntimeBridge) Site() conf.Site {
	if b.inner == nil {
		return conf.Site{}
	}
	return b.inner.GetRuntimeSite()
}

// AI 返回当前 AI 配置。
func (b siteRuntimeBridge) AI() conf.AI {
	if b.inner == nil {
		return conf.AI{}
	}
	return b.inner.GetRuntimeAI()
}

// UpdateSite 覆盖站点配置。
func (b siteRuntimeBridge) UpdateSite(site conf.Site) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateRuntimeSite(site)
}

// UpdateAI 覆盖 AI 配置。
func (b siteRuntimeBridge) UpdateAI(ai conf.AI) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateRuntimeAI(ai)
}

// errSiteRuntimeMissing 表示运行时配置服务没装配。
func errSiteRuntimeMissing() error { return apperr.New(apperr.CodeSiteConfigMissing) }
