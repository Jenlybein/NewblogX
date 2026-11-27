package middleware

import (
	"strings"

	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/platform/http/res"
	"myblogx/utils/io_util"

	"github.com/gin-gonic/gin"
)

// 验证码的**入口标识**。每个需要验证码的路由挂自己那一个。
//
// 为什么要按入口分开：登录与发码被滥用的方式不同 —— 发码要卡成本（短信是真花钱），
// 登录要卡爆破。一刀切的开关会让"只想给发码加验证码"做不到，而真的想只加一处时
// 又只能整体关掉、连带拆掉另一处的防护。
const (
	// CaptchaScopeLogin 是密码 / 邮箱 / 手机号登录入口。
	CaptchaScopeLogin = "login"
	// CaptchaScopeEmailVerify 是发邮箱验证码入口。
	CaptchaScopeEmailVerify = "email_verify"
	// CaptchaScopePhoneVerify 是发短信验证码入口。
	CaptchaScopePhoneVerify = "phone_verify"
	// CaptchaScopeRegister 是注册专用入口（只有 POST /users/email/register 挂它）。
	CaptchaScopeRegister = "register"
)

type CaptchaMiddlewareRequest struct {
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required"`
}

// CaptchaGate 按入口返回校验中间件。
//
// 是否真的校验由两件事合起来决定：总开关 `captcha.captcha` 打开，且该入口自己的
// 闸门打开。总开关关掉时所有入口都不校验（与旧行为一致），这样"先整体关、再逐条开"
// 的迁移路径是通的。
//
// 五个开关都在 captcha 段（票 481 从 site.login 迁入）：验证码要不要拦、拦哪条
// 入口，是验证码这个机制自己的属性。读者站仍然能知道这些闸门的**合成结果**
// （GET /api/site/login_capabilities 的 captcha_required），只是不再靠公开整个
// site 段拿到 —— 那一段现在只剩密码登录开关与登录失败限流。
func CaptchaGate(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		runtimeFromContext(c).captchaGate(c, scope)
	}
}

// captchaGate 是 CaptchaGate 的实现。
func (h Runtime) captchaGate(c *gin.Context, scope string) {
	if h.RuntimeSite == nil {
		return
	}
	captchaConf := h.RuntimeSite.GetRuntimeCaptcha()
	if !captchaConf.Captcha {
		return
	}
	if !captchaGateEnabled(captchaConf, scope) {
		return
	}

	var cr CaptchaMiddlewareRequest
	if err := io_util.ShouldBindJSONWithRecover(c, &cr); err != nil {
		if h.Logger != nil {
			h.Logger.Errorf("图形验证失败：请求体绑定失败：%v", err)
		}
		res.FailWithMsg("图形验证失败：请求体绑定失败", c)
		c.Abort()
		return
	}

	if h.ImageCaptchaStore == nil || !h.ImageCaptchaStore.Verify(cr.CaptchaID, cr.CaptchaCode, true) {
		res.FailWithMsg(captchaFailMessage(h.RuntimeSite), c)
		c.Abort()
		return
	}
}

// captchaGateEnabled 判断某个入口的闸门是否打开。
//
// 未知 scope 一律不校验：写错挂载点时宁可少校验一次（会立刻被登录链路测出来），
// 也不要把一个没登记的路由默认当成"要验证码"，那会让整条链路静默 400。
func captchaGateEnabled(captchaConf conf.Captcha, scope string) bool {
	switch scope {
	case CaptchaScopeLogin:
		return captchaConf.CaptchaLogin
	case CaptchaScopeEmailVerify:
		return captchaConf.CaptchaEmailVerify
	case CaptchaScopePhoneVerify:
		return captchaConf.CaptchaPhoneVerify
	case CaptchaScopeRegister:
		return captchaConf.CaptchaRegister
	}
	return false
}

// DefaultCaptchaFailMessage 是读者填错验证码时的默认提示。
//
// 与旧实现的字面量逐字一致：没在后台配过的站点，行为不该因为这次改动而变。
const DefaultCaptchaFailMessage = "图形验证码错误"

// captchaFailMessage 取当前配置的失败文案；没配（空串或配置服务缺失）就用默认值。
//
// 每次现读：后台改完这句立刻生效，不需要重启（与图形参数的读取口径一致）。
func captchaFailMessage(runtimeSite *sitecfg.RuntimeConfigService) string {
	if runtimeSite == nil {
		return DefaultCaptchaFailMessage
	}
	if msg := strings.TrimSpace(runtimeSite.GetRuntimeCaptcha().FailMessage); msg != "" {
		return msg
	}
	return DefaultCaptchaFailMessage
}
