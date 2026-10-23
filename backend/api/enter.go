// API模块入口

package api

import (
	"database/sql"
	"myblogx/api/ai_api"
	"myblogx/api/article_api"
	"myblogx/api/banner_api"
	"myblogx/api/captcha_api"
	"myblogx/api/chat_api"
	"myblogx/api/comment_api"
	"myblogx/api/data_api"
	"myblogx/api/follow_api"
	"myblogx/api/global_notif_api"
	"myblogx/api/image_api"
	"myblogx/api/log_api"
	"myblogx/api/search_api"
	"myblogx/api/site_api"
	"myblogx/api/sitemsg_api"
	"myblogx/api/user_api"
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	Version           string
	System            conf.System
	JWT               conf.Jwt
	Log               conf.Logrus
	ClickHouseConfig  conf.ClickHouse
	ES                conf.ES
	QQ                conf.QQ
	Email             conf.Email
	QiNiu             conf.QiNiu
	Upload            conf.Upload
	Logger            *logrus.Logger
	DB                *gorm.DB
	Redis             *redis.Client
	ClickHouse        *sql.DB
	ESClient          *elasticsearch.Client
	RuntimeSite       *site_service.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
}

type Api struct {
	SiteApi         site_api.SiteApi
	LogApi          log_api.LogApi
	ImageApi        image_api.ImageApi
	BannerApi       banner_api.BannerApi
	ImageCaptchaApi captcha_api.ImageCaptchaApi
	UserApi         user_api.UserApi
	ArticleApi      article_api.ArticleApi
	CommentApi      comment_api.CommentApi
	ChatApi         chat_api.ChatApi
	SitemsgApi      sitemsg_api.SitemsgApi
	GlobalNotifApi  global_notif_api.GlobalNotifApi
	FollowApi       follow_api.FollowApi
	SearchApi       search_api.SearchApi
	AIApi           ai_api.AIApi
	DataApi         data_api.DataApi
}

func New(deps Deps) Api {
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
		ImageApi: image_api.New(image_api.Deps{
			DB:     deps.DB,
			Logger: deps.Logger,
			QiNiu:  deps.QiNiu,
			Upload: deps.Upload,
			Redis:  deps.Redis,
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
			RuntimeSite:      deps.RuntimeSite,
		}),
		ArticleApi: article_api.New(article_api.Deps{
			DB:          deps.DB,
			JWT:         deps.JWT,
			Logger:      deps.Logger,
			Redis:       deps.Redis,
			RuntimeSite: deps.RuntimeSite,
		}),
		CommentApi: comment_api.New(comment_api.Deps{
			DB:          deps.DB,
			Logger:      deps.Logger,
			Redis:       deps.Redis,
			RuntimeSite: deps.RuntimeSite,
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
