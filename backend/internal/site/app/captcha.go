package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/platform/captcha"
)

// 图形验证码段的边界（第 12 段，ADR-0033）。
//
// 这些数字不是随手取的，每一条都有具体后果：
//   - 字符数 < 2 等于没有验证码；> 8 没法在 200px 宽里看清；
//   - 图片太窄会把字符压变形、太高会把登录框撑开；
//   - 有效期 < 60 秒会让手慢的用户永远填不对；> 1 小时等于给脚本一小时的爆破窗口；
//   - 噪点 > 10 会盖住字符，人自己都读不出来。
const (
	// MinCaptchaLength 是字符数下限。
	MinCaptchaLength = 2
	// MaxCaptchaLength 是字符数上限。
	MaxCaptchaLength = 8
	// MinDigitCaptchaLength 是纯数字型的下限（数字更窄，两位太容易被猜）。
	MinDigitCaptchaLength = 3
	// MinCaptchaWidth / MaxCaptchaWidth 是图片宽度范围（像素）。
	MinCaptchaWidth = 80
	MaxCaptchaWidth = 400
	// MinCaptchaHeight / MaxCaptchaHeight 是图片高度范围（像素）。
	MinCaptchaHeight = 30
	MaxCaptchaHeight = 120
	// MinCaptchaExpireSeconds / MaxCaptchaExpireSeconds 是有效期范围（秒）。
	MinCaptchaExpireSeconds = 60
	MaxCaptchaExpireSeconds = 3600
	// MaxCaptchaNoiseCount 是噪点上限。
	MaxCaptchaNoiseCount = 10
	// MaxCaptchaDotCount 是纯数字型背景圆点上限。
	MaxCaptchaDotCount = 20
	// MaxCaptchaSourceRunes 是字符集的字符数上限（防一次填几万个字符）。
	MaxCaptchaSourceRunes = 128
	// MaxCaptchaFailMessageRunes 是失败文案的字符数上限（给读者看的一句话，不是日志）。
	MaxCaptchaFailMessageRunes = 80
)

// DefaultCaptchaSource 是字符型验证码的默认字符集。
//
// 旧实现的字符集是 `123456789ABCDEFGHIJKLHIJKLMNOPQRSTUVWXYZ`，`H` / `I` / `J` / `K` / `L`
// 各有两处（重复了 5 个字符，40 个字符只剩 35 个可用）。这里按"数字 + 大写字母、
// 去掉易混的 0/O/1/I/L"给一份明确的默认值：易混字符只会制造"我看对了却填错"的投诉。
const DefaultCaptchaSource = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// DefaultCaptchaBgColor 是默认背景色（带 10/255 不透明度，与旧实现观感一致）。
const DefaultCaptchaBgColor = "#0A14320A"

// AdminCaptcha 返回管理端的图形验证码配置。
func (s *Service) AdminCaptcha() conf.Captcha { return s.runtime.Captcha() }

// UpdateCaptcha 覆盖图形验证码配置。
//
// 保存即生效：生成请求每次都按配置现值构建驱动、按现值写 TTL，没有"重建"这一步。
// 归一化在用例层做完，让坏数据进不了库 —— 否则下一次有人打开登录页才发现图片是坏的。
func (s *Service) UpdateCaptcha(input conf.Captcha) error {
	if s.runtime == nil {
		return apperr.New(apperr.CodeSiteConfigMissing)
	}
	normalized, err := NormalizeCaptcha(input)
	if err != nil {
		return err
	}
	return s.runtime.UpdateCaptcha(normalized)
}

// NormalizeCaptcha 整理并校验图形验证码段。
//
// 口径：**越界即拒绝，不静默夹取**。理由与词表段一致 —— 后台把 width 填成 5000
// 却"保存成功"，运营看到的是一张能撑爆布局的图，而排查时没有任何线索。
// 只有"没填（0 / 空串）"才回落到默认值，这与运行时配置 merge 的语义一致。
//
// 返回的错误一律是 apperr.CodeSiteCaptchaInvalid 包住的具体原因（哪个字段、越什么界），
// 后台能直接把那句话显示给运营。
func NormalizeCaptcha(input conf.Captcha) (conf.Captcha, error) {
	out := input

	driver := strings.TrimSpace(strings.ToLower(input.Driver))
	switch driver {
	case "":
		driver = captchaDriverString
	case captchaDriverString, captchaDriverMath, captchaDriverDigit:
	default:
		return conf.Captcha{}, captchaInvalid("验证码类型只能是 string / math / digit，收到 %q", input.Driver)
	}
	out.Driver = driver

	out.Width = defaultIfZero(input.Width, 200)
	if out.Width < MinCaptchaWidth || out.Width > MaxCaptchaWidth {
		return conf.Captcha{}, captchaInvalid("图片宽度需在 %d–%d 之间，收到 %d", MinCaptchaWidth, MaxCaptchaWidth, out.Width)
	}
	out.Height = defaultIfZero(input.Height, 60)
	if out.Height < MinCaptchaHeight || out.Height > MaxCaptchaHeight {
		return conf.Captcha{}, captchaInvalid("图片高度需在 %d–%d 之间，收到 %d", MinCaptchaHeight, MaxCaptchaHeight, out.Height)
	}
	out.ExpireSeconds = defaultIfZero(input.ExpireSeconds, 600)
	if out.ExpireSeconds < MinCaptchaExpireSeconds || out.ExpireSeconds > MaxCaptchaExpireSeconds {
		return conf.Captcha{}, captchaInvalid("有效期需在 %d–%d 秒之间，收到 %d",
			MinCaptchaExpireSeconds, MaxCaptchaExpireSeconds, out.ExpireSeconds)
	}

	// 字符数：算术型的题目固定是两位数运算，长度对它没有意义，统一写 0 表示"不适用"。
	if driver == captchaDriverMath {
		out.Length = 0
	} else {
		out.Length = defaultIfZero(input.Length, 4)
		lower := MinCaptchaLength
		if driver == captchaDriverDigit {
			lower = MinDigitCaptchaLength
		}
		if out.Length < lower || out.Length > MaxCaptchaLength {
			return conf.Captcha{}, captchaInvalid("字符数需在 %d–%d 之间，收到 %d", lower, MaxCaptchaLength, out.Length)
		}
	}

	// 字符型专属：字符集去空白、去重复字符，并保证还剩至少两个可用字符。
	// 其余两种驱动把 source 清零 —— "保存成功但没效果"是运营最讨厌的那种失败。
	if driver != captchaDriverString {
		out.Source = ""
	} else {
		out.Source = strings.TrimSpace(input.Source)
		if out.Source == "" {
			out.Source = DefaultCaptchaSource
		} else {
			deduped, err := dedupCaptchaSource(out.Source)
			if err != nil {
				return conf.Captcha{}, err
			}
			out.Source = deduped
		}
	}

	// 噪点只对字符型 / 算术型有意义；纯数字驱动不收这项，统一写 0（表示不适用）。
	//
	// 注意 0 在这里是**合法值**（"不放噪点"而不是"没填"）：字段注释与后台提示都
	// 承诺了这一点，用 defaultIfZero 会把它悄悄改成 3，运营回读看到 3 却不知道
	// 自己填的 0 去哪了。"没带字段"因此也按 0 处理 —— PUT 是整段替换语义
	// （与 content_filter 一致），后台表单每次都会带回完整对象，手写请求没带就是
	// 明确要求关掉，比偷偷改回 3 诚实。
	if driver == captchaDriverDigit {
		out.NoiseCount = 0
	} else {
		out.NoiseCount = input.NoiseCount
		if out.NoiseCount < 0 || out.NoiseCount > MaxCaptchaNoiseCount {
			return conf.Captcha{}, captchaInvalid("噪点数需在 0–%d 之间，收到 %d", MaxCaptchaNoiseCount, out.NoiseCount)
		}
	}

	// 背景色只对字符型 / 算术型有意义（纯数字驱动不收背景色）。
	if driver == captchaDriverDigit {
		out.BgColor = ""
	} else {
		out.BgColor = strings.TrimSpace(input.BgColor)
		if out.BgColor == "" {
			out.BgColor = DefaultCaptchaBgColor
		} else if _, err := captcha.ParseBGColor(out.BgColor); err != nil {
			return conf.Captcha{}, captchaInvalid("%s", err.Error())
		}
	}

	// 倾斜与背景点（纯数字型专属）：**0 是合法值**（"不倾斜 / 不放点"），不回落默认值；
	// 负值或越界才是非法输入。
	//
	// 这两个字段**不按驱动清零**：清零会造成两个可见的怪事 ——
	// (a) yaml 种子 max_skew=0.5、GET→PUT 往返后变 0（保存成功但值变了）；
	// (b) 运营从 string 切到 digit 时看到的是 0 而不是自己填过的 0.5。
	// 不生效的字段留着不会改变图片（其他驱动根本不读它们），这比"悄悄改值"诚实。
	out.MaxSkew = input.MaxSkew
	if out.MaxSkew < 0 || out.MaxSkew > 1 {
		return conf.Captcha{}, captchaInvalid("最大倾斜需在 0–1 之间，收到 %v", out.MaxSkew)
	}
	out.DotCount = input.DotCount
	if out.DotCount < 0 || out.DotCount > MaxCaptchaDotCount {
		return conf.Captcha{}, captchaInvalid("背景点数需在 0–%d 之间，收到 %d", MaxCaptchaDotCount, out.DotCount)
	}

	// 失败文案：空串表示用默认文案；给了就只做去空白与长度校验。
	// 上限 80：这是给读者看的一句话，不是日志字段，太长会把登录框顶变形。
	out.FailMessage = strings.TrimSpace(input.FailMessage)
	if utf8.RuneCountInString(out.FailMessage) > MaxCaptchaFailMessageRunes {
		return conf.Captcha{}, captchaInvalid("失败文案最多 %d 个字符", MaxCaptchaFailMessageRunes)
	}
	// 干扰线三个开关对纯数字型无意义（该驱动不收这些选项），统一关掉。
	if driver == captchaDriverDigit {
		out.ShowHollowLine = false
		out.ShowSlimeLine = false
		out.ShowSineLine = false
	}

	return out, nil
}

// 三种驱动名，与 conf.Captcha.Driver 的取值、platform/captcha 的常量一致。
//
// 这里用私有字符串常量而不是引用 platform/captcha 的导出常量：本文件确实已经
// import 了 platform/captcha（为了 ParseBGColor 校验颜色），但**不再为三个字符串**
// 加一层耦合。两边的一致性由 TestNormalizeCaptchaDriversBuildDrivers 钉住 ——
// 它把归一化产出的每个 driver 交给 platform/captcha.BuildDriver，drift 会直接红。
const (
	captchaDriverString = "string"
	captchaDriverMath   = "math"
	captchaDriverDigit  = "digit"
)

// captchaInvalid 组装"哪个字段越界"的具体原因，并包成同一个错误码。
func captchaInvalid(format string, args ...any) error {
	// NewWithMessage 而不是 Wrap：这句"哪个字段越什么界"要能原样显示给运营，
	// 而 Wrap 的具体原因会被错误码的通用文案盖掉（响应层只看 userMessage）。
	return apperr.NewWithMessage(apperr.CodeSiteCaptchaInvalid, fmt.Sprintf(format, args...))
}

// dedupCaptchaSource 去掉空白与重复字符。
//
// 重复字符必须去掉：同一个字符出现两种写法只会降低穷举难度，不会让图片更好认。
// 旧默认值里的 `IJKLHIJKL` 就是这么来的。
func dedupCaptchaSource(source string) (string, error) {
	if utf8.RuneCountInString(source) > MaxCaptchaSourceRunes {
		return "", captchaInvalid("字符集最多 %d 个字符", MaxCaptchaSourceRunes)
	}
	var b strings.Builder
	seen := make(map[rune]struct{}, utf8.RuneCountInString(source))
	for _, r := range source {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		b.WriteRune(r)
	}
	if utf8.RuneCountInString(b.String()) < MinCaptchaLength {
		return "", captchaInvalid("字符集去掉重复后至少要有 %d 个字符", MinCaptchaLength)
	}
	return b.String(), nil
}

// defaultIfZero 是"没填就用默认值"的取值口径。
func defaultIfZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
