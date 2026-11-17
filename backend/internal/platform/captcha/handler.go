// Package captcha 提供图片验证码的生成能力。
//
// 为什么放在 platform 而不是建一个业务模块：它没有领域状态、没有业务规则，
// 只是把 base64Captcha 包一层并读配置。为这点代码造 domain/app/adapter 三层，
// 那是为了结构而结构。
//
// 与校验侧的耦合：校验在 middleware 的 CaptchaGate，它读的是同一份运行时配置
// （内部实现走 middleware.RuntimeOptions.ImageCaptchaStore）。
// **生成与校验必须指向同一个 store 实例**，否则前端拿到的 captcha_id 校验时查不到 ——
// 装配层（app.WireHTTP）因此只创建一个 store，同时传给这里和 RuntimeOptions。
package captcha

import (
	"myblogx/conf"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Settings 读站点是否启用验证码，以及当前的图形与时效参数。
//
// 做成窄接口而不是直接收 RuntimeConfigService：装配层负责适配，
// 平台包不依赖具体的配置实现。
type Settings interface {
	// CaptchaEnabled 是总开关（site.login.captcha）。总开关关掉时生成接口直接拒绝。
	CaptchaEnabled() bool
	// CaptchaConfig 是第 12 段的图形与时效参数（ADR-0033）。
	CaptchaConfig() conf.Captcha
}

// Handler 生成图片验证码。
type Handler struct {
	settings Settings
	store    base64Captcha.Store
	logger   Logger
}

// Logger 是窄日志接口（平台包不依赖具体日志实现）。
type Logger interface {
	Errorf(format string, args ...any)
}

// NewHandler 构造处理器。store 必须与校验侧是同一个实例。
func NewHandler(settings Settings, store base64Captcha.Store, logger Logger) *Handler {
	if store == nil {
		panic("captcha: 验证码存储未注入")
	}
	return &Handler{settings: settings, store: store, logger: logger}
}

// ImageResponse 是验证码响应。
type ImageResponse struct {
	CaptchaID string `json:"captcha_id"`
	Base64    string `json:"base64"`
	// ExpireSeconds 回显当前有效期：前端据此决定"多久没操作就主动换一张"，
	// 也让运营在后台改完有效期后能在响应里立刻看到生效值。
	ExpireSeconds int `json:"expire_seconds"`
}

// Image 生成图片验证码并返回 base64 图片。
// @Summary 获取图片验证码
// @Tags captcha
// @Produce json
// @Success 200 {object} kernel.Envelope[ImageResponse]
// @Router /imagecaptcha [get]
func (h *Handler) Image(c *gin.Context) {
	if h.settings == nil || !h.settings.CaptchaEnabled() {
		res.FailWithMsg("站点未启用验证码功能", c)
		return
	}

	cfg := h.settings.CaptchaConfig()
	driver, err := BuildDriver(cfg)
	if err != nil {
		// 配置写坏了（例如背景色写成 "深蓝"）—— 这是配置问题不是用户问题，
		// 所以文案直接说清楚，别再让用户以为是自己手速慢。
		res.FailWithMsg("验证码配置无效："+err.Error(), c)
		return
	}

	captcha := base64Captcha.NewCaptcha(driver, h.store)
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		// 不把 err.Error() 回给调用方：这个接口是匿名的，而错误里可能是
		// `dial tcp redis:6379: connect: connection refused` 这类内部拓扑。
		// 存储抖动时的正确表现是"这张图取不到，让用户刷新重试"，不是把内部信息透出去。
		if h.logger != nil {
			h.logger.Errorf("生成验证码失败: %v", err)
		}
		res.FailWithMsg("验证码生成失败，请刷新重试", c)
		return
	}
	res.OkWithData(&ImageResponse{
		CaptchaID:     id,
		Base64:        b64s,
		ExpireSeconds: cfg.ExpireSeconds,
	}, c)
}
