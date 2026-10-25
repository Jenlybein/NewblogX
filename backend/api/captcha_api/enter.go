package captcha_api

import (
	sitecfg "myblogx/internal/platform/config"

	"github.com/mojocn/base64Captcha"
)

// options 是验证码处理器真正需要的两项依赖。
//
// 不用 Options 命名：它只是构造函数参数的形状，不是"依赖打包"。
type options struct {
	runtimeSite *sitecfg.RuntimeConfigService
	store       base64Captcha.Store
}

// New 构造验证码处理器。
func New(runtimeSite *sitecfg.RuntimeConfigService, store base64Captcha.Store) ImageCaptchaApi {
	return ImageCaptchaApi{App: options{runtimeSite: runtimeSite, store: store}}
}
