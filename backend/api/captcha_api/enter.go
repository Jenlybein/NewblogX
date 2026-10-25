package captcha_api

import (
	sitecfg "myblogx/internal/platform/config"

	"github.com/mojocn/base64Captcha"
)

type Deps struct {
	RuntimeSite       *sitecfg.RuntimeConfigService
	ImageCaptchaStore base64Captcha.Store
}

func New(deps Deps) ImageCaptchaApi {
	return ImageCaptchaApi{App: deps}
}
