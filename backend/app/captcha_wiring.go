package app

import (
	"myblogx/conf"
	"myblogx/internal/platform/captcha"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/router"

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

// CaptchaEnabled 读总开关（site.login.captcha）。
// 配置服务缺失时按"关闭"处理，与旧实现的 nil 判断一致。
func (s captchaSettings) CaptchaEnabled() bool {
	if s.site == nil {
		return false
	}
	return s.site.GetRuntimeLogin().Captcha
}

// CaptchaConfig 读第 12 段的图形与时效参数。
//
// 每次现读：后台保存后立刻生效，请求路径不做"重建驱动"的缓存。
func (s captchaSettings) CaptchaConfig() conf.Captcha {
	if s.site == nil {
		return conf.Captcha{}
	}
	return s.site.GetRuntimeCaptcha()
}
