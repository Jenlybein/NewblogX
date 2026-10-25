// API模块入口

package api

import (
	"myblogx/api/ai_api"
	"myblogx/api/captcha_api"
	"myblogx/api/data_api"
	"myblogx/api/log_api"
	"myblogx/api/user_api"
	"myblogx/middleware"
)

type Api struct {
	LogApi          log_api.LogApi
	ImageCaptchaApi captcha_api.ImageCaptchaApi
	UserApi         user_api.UserApi
	AIApi           ai_api.AIApi
	DataApi         data_api.DataApi
}

func New(deps middleware.Deps) Api {
	return Api{
		LogApi: log_api.New(log_api.Deps{
			Log:              deps.Log,
			System:           deps.System,
			ClickHouseConfig: deps.ClickHouseConfig,
			Logger:           deps.Logger,
			ClickHouse:       deps.ClickHouse,
			DB:               deps.DB,
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
