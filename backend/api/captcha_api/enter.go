package captcha_api

import (
	"myblogx/service/site_service"

	"github.com/mojocn/base64Captcha"
)

type Deps struct {
	RuntimeSite       *site_service.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
}

func New(deps Deps) ImageCaptchaApi {
	return ImageCaptchaApi{App: deps}
}
