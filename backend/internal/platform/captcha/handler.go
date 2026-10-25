// Package captcha 提供图片验证码的生成能力。
//
// 为什么放在 platform 而不是建一个业务模块：它没有领域状态、没有业务规则，
// 只是把 base64Captcha 包一层并读一个开关。为 170 行代码造 domain/app/adapter
// 三层，那是为了结构而结构。
//
// 与校验侧的耦合：校验在 middleware.CaptchaMiddleware（登录、发邮箱验证码两条
// 路由挂了它），它读的是 middleware.RuntimeOptions.ImageCaptchaStore。
// **生成与校验必须指向同一个 store 实例**，否则前端拿到的 captcha_id 校验时查不到。
// 装配层（app.WireHTTP）因此只创建一个 store 实例，同时传给这里和 RuntimeOptions。
package captcha

import (
	"image/color"

	"myblogx/common/res"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

// Settings 读站点是否启用验证码。
//
// 做成窄接口而不是直接收 RuntimeConfigService：装配层负责适配，
// 平台包不依赖具体的配置实现。
type Settings interface {
	CaptchaEnabled() bool
}

// Handler 生成图片验证码。
type Handler struct {
	settings Settings
	store    base64Captcha.Store
}

// NewHandler 构造处理器。
func NewHandler(settings Settings, store base64Captcha.Store) *Handler {
	if store == nil {
		panic("captcha: 验证码存储未注入")
	}
	return &Handler{settings: settings, store: store}
}

// ImageResponse 是验证码响应。
type ImageResponse struct {
	CaptchaID string `json:"captcha_id"`
	Base64    string `json:"base64"`
}

// Image 生成图片验证码并返回 base64 图片。
// @Summary 获取图片验证码
// @Tags captcha
// @Produce json
// @Success 200 {object} object{code=int,data=ImageResponse,msg=string}
// @Router /imagecaptcha [get]
func (h *Handler) Image(c *gin.Context) {
	if h.settings == nil || !h.settings.CaptchaEnabled() {
		res.FailWithMsg("站点未启用验证码功能", c)
		return
	}

	// 尺寸、噪点、干扰线、字符集与背景色沿用旧参数：改这些会让
	// 前端已经调好的 input 尺寸对不上，属于无收益的变动。
	driverString := base64Captcha.DriverString{
		Height:          60,
		Width:           200,
		NoiseCount:      3,
		ShowLineOptions: 2 | 4,
		Length:          4,
		Source:          "123456789ABCDEFGHIJKLHIJKLMNOPQRSTUVWXYZ",
		BgColor:         &color.RGBA{R: 10, G: 20, B: 50, A: 10},
	}

	var driver base64Captcha.Driver = driverString.ConvertFonts()

	captcha := base64Captcha.NewCaptcha(driver, h.store)
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(&ImageResponse{CaptchaID: id, Base64: b64s}, c)
}
