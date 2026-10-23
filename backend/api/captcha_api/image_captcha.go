package captcha_api

import (
	"image/color"
	"myblogx/common/res"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

type ImageCaptchaApi struct {
	App Deps
}

type ImageCaptchaResponse struct {
	CaptchaId string `json:"captcha_id"`
	Base64    string `json:"base64"`
}

func (h *ImageCaptchaApi) CaptchaView(c *gin.Context) {
	if h.App.RuntimeSite == nil || !h.App.RuntimeSite.GetRuntimeLogin().Captcha {
		res.FailWithMsg("站点未启用验证码功能", c)
		return
	}

	//配置验证码
	driverString := base64Captcha.DriverString{
		Height:          60,
		Width:           200,
		NoiseCount:      3,     //噪点数
		ShowLineOptions: 2 | 4, //干扰线
		Length:          4,
		Source:          "123456789ABCDEFGHIJKLHIJKLMNOPQRSTUVWXYZ",
		BgColor:         &color.RGBA{R: 10, G: 20, B: 50, A: 10}, // 背景颜色
		// Fonts:           []string{"wqy-microhei.ttc"},            // 字体文件
	}

	var driver base64Captcha.Driver = driverString.ConvertFonts()

	//生成验证码
	captcha := base64Captcha.NewCaptcha(driver, h.App.ImageCaptchaStore)
	if id, b64s, _, err := captcha.Generate(); err == nil {
		res.OkWithData(&ImageCaptchaResponse{
			CaptchaId: id,
			Base64:    b64s,
		}, c)
	} else {
		res.FailWithMsg(err.Error(), c)
		return
	}
}
