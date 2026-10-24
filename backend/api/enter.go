// API模块入口

package api

import (
	"myblogx/api/ai_api"
	"myblogx/api/banner_api"
	"myblogx/api/captcha_api"
	"myblogx/api/chat_api"
	"myblogx/api/data_api"
	"myblogx/api/follow_api"
	"myblogx/api/global_notif_api"
	"myblogx/api/log_api"
	"myblogx/api/search_api"
	"myblogx/api/site_api"
	"myblogx/api/sitemsg_api"
	"myblogx/api/user_api"
	"myblogx/middleware"
)

type Api struct {
	SiteApi         site_api.SiteApi
	LogApi          log_api.LogApi
	BannerApi       banner_api.BannerApi
	ImageCaptchaApi captcha_api.ImageCaptchaApi
	UserApi         user_api.UserApi
	ChatApi         chat_api.ChatApi
	SitemsgApi      sitemsg_api.SitemsgApi
	GlobalNotifApi  global_notif_api.GlobalNotifApi
	FollowApi       follow_api.FollowApi
	SearchApi       search_api.SearchApi
	AIApi           ai_api.AIApi
	DataApi         data_api.DataApi
}

func New(deps middleware.Deps) Api {
	return Api{
		SiteApi: site_api.New(site_api.Deps{
			Version:     deps.Version,
			Logger:      deps.Logger,
			Redis:       deps.Redis,
			QQ:          deps.QQ,
			RuntimeSite: deps.RuntimeSite,
		}),
		LogApi: log_api.New(log_api.Deps{
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
			DB:               deps.DB,
		}),
		BannerApi: banner_api.New(banner_api.Deps{
			DB: deps.DB,
		}),
		ImageCaptchaApi: captcha_api.New(captcha_api.Deps{
			RuntimeSite:       deps.RuntimeSite,
			ImageCaptchaStore: deps.ImageCaptchaStore,
		}),
		UserApi: user_api.New(user_api.Deps{
			DB:               deps.DB,
			JWT:              deps.JWT,
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			ClickHouse:       deps.ClickHouse,
			QQ:               deps.QQ,
			Email:            deps.Email,
			Logger:           deps.Logger,
			Redis:            deps.Redis,
		}),
		ChatApi: chat_api.New(chat_api.Deps{
			DB:     deps.DB,
			Logger: deps.Logger,
			JWT:    deps.JWT,
			Redis:  deps.Redis,
		}),
		SitemsgApi: sitemsg_api.New(sitemsg_api.Deps{
			DB: deps.DB,
		}),
		GlobalNotifApi: global_notif_api.New(global_notif_api.Deps{
			DB: deps.DB,
		}),
		FollowApi: follow_api.New(follow_api.Deps{
			DB: deps.DB,
		}),
		SearchApi: search_api.New(search_api.Deps{
			DB:       deps.DB,
			Logger:   deps.Logger,
			JWT:      deps.JWT,
			Redis:    deps.Redis,
			ESClient: deps.ESClient,
			ES:       deps.ES,
		}),
		AIApi: ai_api.New(ai_api.Deps{
			DB:          deps.DB,
			Logger:      deps.Logger,
			Redis:       deps.Redis,
			ESClient:    deps.ESClient,
			ES:          deps.ES,
			RuntimeSite: deps.RuntimeSite,
		}),
		DataApi: data_api.New(data_api.Deps{
			System:           deps.System,
			Log:              deps.Log,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			DB:               deps.DB,
			Redis:            deps.Redis,
			ClickHouse:       deps.ClickHouse,
		}),
	}
}
