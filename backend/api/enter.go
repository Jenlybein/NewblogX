// API模块入口

package api

import (
	"myblogx/api/captcha_api"
	"myblogx/middleware"
)

type Api struct {
	ImageCaptchaApi captcha_api.ImageCaptchaApi
}

func New(deps middleware.Deps) Api {
	return Api{
		ImageCaptchaApi: captcha_api.New(captcha_api.Deps{
			RuntimeSite:       deps.RuntimeSite,
			ImageCaptchaStore: deps.ImageCaptchaStore,
		}),
	}
}
