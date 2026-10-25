// API模块入口

package api

import (
	"myblogx/api/captcha_api"
	sitecfg "myblogx/internal/platform/config"

	"github.com/mojocn/base64Captcha"
)

type Api struct {
	ImageCaptchaApi captcha_api.ImageCaptchaApi
}

func New(runtimeSite *sitecfg.RuntimeConfigService, captchaStore base64Captcha.Store) Api {
	return Api{
		ImageCaptchaApi: captcha_api.New(runtimeSite, captchaStore),
	}
}
