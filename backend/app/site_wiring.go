package app

import (
	"context"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_site"

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
		func() string { return infra.RuntimeSite.GetRuntimeOAuth().Url() },
		infra.Logger,
	)
	// 右栏卡片：配置表 + 三种内置卡的数据源。
	// 数据源是站点模块自己的只读查询（文章 / 评论 / 标签的投影读），
	// 不 import 别的模块 adapter（R11）。
	sidebar := siteapp.NewSidebar(
		sitemysql.NewSidebar(infra.DB),
		sitemysql.NewSidebarData(infra.DB),
		infra.Logger,
	)
	// 站点访问量：每次读取公开配置记一次（口径与迁移前一致）。
	redisOpts := redis_service.Options{Client: infra.Redis, Logger: infra.Logger}
	router.SetSiteHandler(sitehttp.NewHandler(svc, func() {
		redis_site.SetFlow(redisOpts)
	}, sitehttp.WithSidebar(sidebar)))
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
	return b.inner.UpdateSite(site)
}

// UpdateAI 覆盖 AI 配置。
func (b siteRuntimeBridge) UpdateAI(ai conf.AI) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateAI(ai)
}

// Mail 返回当前邮件配置。
func (b siteRuntimeBridge) Mail() conf.Mail {
	if b.inner == nil {
		return conf.Mail{}
	}
	return b.inner.GetRuntimeMail()
}

// Storage 返回当前对象存储配置。
func (b siteRuntimeBridge) Storage() conf.QiNiu {
	if b.inner == nil {
		return conf.QiNiu{}
	}
	return b.inner.GetRuntimeStorage()
}

// OAuth 返回当前第三方登录配置。
func (b siteRuntimeBridge) OAuth() conf.QQ {
	if b.inner == nil {
		return conf.QQ{}
	}
	return b.inner.GetRuntimeOAuth()
}

// Log 返回当前日志配置。
func (b siteRuntimeBridge) Log() conf.Logrus {
	if b.inner == nil {
		return conf.Logrus{}
	}
	return b.inner.GetRuntimeLog()
}

// Upload 返回当前上传策略。
func (b siteRuntimeBridge) Upload() conf.Upload {
	if b.inner == nil {
		return conf.Upload{}
	}
	return b.inner.GetRuntimeUpload()
}

// Deco 返回当前装饰配置。
func (b siteRuntimeBridge) Deco() conf.Deco {
	if b.inner == nil {
		return conf.Deco{}
	}
	return b.inner.GetRuntimeDeco()
}

// UpdateMail 覆盖邮件配置。
func (b siteRuntimeBridge) UpdateMail(mail conf.Mail) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateMail(mail)
}

// UpdateStorage 覆盖对象存储配置。
func (b siteRuntimeBridge) UpdateStorage(storage conf.QiNiu) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateStorage(storage)
}

// UpdateOAuth 覆盖第三方登录配置。
func (b siteRuntimeBridge) UpdateOAuth(oauth conf.QQ) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateOAuth(oauth)
}

// UpdateLog 覆盖日志配置。
func (b siteRuntimeBridge) UpdateLog(log conf.Logrus) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateLog(log)
}

// UpdateUpload 覆盖上传策略。
func (b siteRuntimeBridge) UpdateUpload(upload conf.Upload) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateUpload(upload)
}

// UpdateDeco 覆盖装饰配置。
func (b siteRuntimeBridge) UpdateDeco(deco conf.Deco) error {
	if b.inner == nil {
		return errSiteRuntimeMissing()
	}
	return b.inner.UpdateDeco(deco)
}

// errSiteRuntimeMissing 表示运行时配置服务没装配。
func errSiteRuntimeMissing() error { return apperr.New(apperr.CodeSiteConfigMissing) }
