package app

import (
	"myblogx/internal/platform/captcha"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/router"

	"github.com/mojocn/base64Captcha"
)

// WireCaptcha 装配验证码处理器。
//
// store 由调用方传入，且必须与 middleware.RuntimeOptions.ImageCaptchaStore
// 是同一个实例 —— 生成与校验分属两个包，共享 store 才能让 captcha_id 对得上。
func WireCaptcha(infra *Infra, store base64Captcha.Store) {
	if infra == nil {
		return
	}
	router.SetCaptchaHandler(captcha.NewHandler(
		captchaSettings{site: infra.RuntimeSite},
		store,
	))
}

// captchaSettings 把运行时配置适配成 captcha.Settings。
type captchaSettings struct {
	site *sitecfg.RuntimeConfigService
}

// CaptchaEnabled 读站点开关。配置服务缺失时按"关闭"处理，与旧实现的 nil 判断一致。
func (s captchaSettings) CaptchaEnabled() bool {
	if s.site == nil {
		return false
	}
	return s.site.GetRuntimeLogin().Captcha
}
