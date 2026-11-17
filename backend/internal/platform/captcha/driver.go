package captcha

import (
	"fmt"

	"myblogx/conf"

	"github.com/mojocn/base64Captcha"
)

// 三种驱动名（与 conf.Captcha.Driver 的取值、前端下拉一致）。
const (
	DriverString = "string"
	DriverMath   = "math"
	DriverDigit  = "digit"
)

// 干扰线的位编码来自 base64Captcha（OptionShowHollowLine=2 / SlimeLine=4 / SineLine=8）。
//
// 这三位是**库的内部编码**，不出现在配置里：后台让运营点三个开关，这里翻成位值。
// 让运营去记"2|4 代表空心线加泥浆线"是不合理的。
const (
	optionShowHollowLine = base64Captcha.OptionShowHollowLine
	optionShowSlimeLine  = base64Captcha.OptionShowSlimeLine
	optionShowSineLine   = base64Captcha.OptionShowSineLine
)

// BuildDriver 按配置构建一个 driver；配置非法（颜色写错、驱动名不认识）时返回错误。
//
// 纯函数，不碰存储与 HTTP —— 三种驱动的参数适用性是这条链路里最容易看错的地方，
// 单独可测才敢改。
func BuildDriver(cfg conf.Captcha) (base64Captcha.Driver, error) {
	lineOptions := 0
	if cfg.ShowHollowLine {
		lineOptions |= optionShowHollowLine
	}
	if cfg.ShowSlimeLine {
		lineOptions |= optionShowSlimeLine
	}
	if cfg.ShowSineLine {
		lineOptions |= optionShowSineLine
	}

	switch cfg.Driver {
	case DriverString:
		bg, err := ParseBGColor(cfg.BgColor)
		if err != nil {
			return nil, err
		}
		driver := base64Captcha.DriverString{
			Height:          cfg.Height,
			Width:           cfg.Width,
			NoiseCount:      cfg.NoiseCount,
			ShowLineOptions: lineOptions,
			Length:          cfg.Length,
			Source:          cfg.Source,
			BgColor:         &bg,
		}
		// ConvertFonts 是指针接收者，且它会填默认字体；不调它图片会画不出来。
		return driver.ConvertFonts(), nil

	case DriverMath:
		bg, err := ParseBGColor(cfg.BgColor)
		if err != nil {
			return nil, err
		}
		// 算术型的题目固定是两位数运算，长度与字符集不适用（归一化已把它们清空）。
		return &base64Captcha.DriverMath{
			Height:          cfg.Height,
			Width:           cfg.Width,
			NoiseCount:      cfg.NoiseCount,
			ShowLineOptions: lineOptions,
			BgColor:         &bg,
		}, nil

	case DriverDigit:
		// 纯数字驱动不收背景色与干扰线：它只有尺寸、位数、倾斜、背景点四个参数。
		return &base64Captcha.DriverDigit{
			Height:   cfg.Height,
			Width:    cfg.Width,
			Length:   cfg.Length,
			MaxSkew:  cfg.MaxSkew,
			DotCount: cfg.DotCount,
		}, nil
	}
	return nil, fmt.Errorf("未知的验证码类型 %q", cfg.Driver)
}
