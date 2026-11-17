package captcha_test

import (
	"image/color"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/platform/captcha"

	"github.com/mojocn/base64Captcha"
)

// stringDriverConfig 给一份"一定能构建成功"的字符型配置。
//
// 字符型必须带合法背景色：BuildDriver 先解析 BgColor 再构造驱动，背景色为空会让
// 构建提前失败。字段断言用例若共用一份坏配置，红的是"颜色解析失败"，而不是
// "宽高没传到驱动"——两种失败的排查方向完全不同，所以这里显式给合法值。
func stringDriverConfig() conf.Captcha {
	return conf.Captcha{
		Driver:         captcha.DriverString,
		Width:          200,
		Height:         60,
		Length:         4,
		Source:         "23456789ABCDEFGHJKMNPQRSTUVWXYZ",
		NoiseCount:     3,
		ShowHollowLine: true,
		ShowSlimeLine:  true,
		BgColor:        "#0A14320A",
		ExpireSeconds:  600,
	}
}

// 三种驱动名都要能构建出对应的具体驱动。
//
// 这条断言守的是配置里的 driver 字符串与生成实现之间的映射：任何一个分支漏接，
// 后台存下 "math" 之后登录页只会看到"未知的验证码类型"，而保存时是完全成功的。
// 三种驱动的字段集不同（字符型有字符集、数字型有倾斜），所以必须确认真的走到了
// 各自那支，而不是"随便返回一个能画图的默认驱动"。
func TestBuildDriverSupportsThreeDrivers(t *testing.T) {
	cfg := stringDriverConfig()
	driver, err := captcha.BuildDriver(cfg)
	if err != nil {
		t.Fatalf("字符型驱动应能构建，实际报错: %v", err)
	}
	if _, ok := driver.(*base64Captcha.DriverString); !ok {
		t.Fatalf("driver=string 应构建 *base64Captcha.DriverString，得到 %T", driver)
	}

	mathCfg := cfg
	mathCfg.Driver = captcha.DriverMath
	driver, err = captcha.BuildDriver(mathCfg)
	if err != nil {
		t.Fatalf("算术型驱动应能构建，实际报错: %v", err)
	}
	if _, ok := driver.(*base64Captcha.DriverMath); !ok {
		t.Fatalf("driver=math 应构建 *base64Captcha.DriverMath，得到 %T", driver)
	}

	digitCfg := cfg
	digitCfg.Driver = captcha.DriverDigit
	digitCfg.Length = 5
	digitCfg.MaxSkew = 0.5
	driver, err = captcha.BuildDriver(digitCfg)
	if err != nil {
		t.Fatalf("纯数字驱动应能构建，实际报错: %v", err)
	}
	if _, ok := driver.(*base64Captcha.DriverDigit); !ok {
		t.Fatalf("driver=digit 应构建 *base64Captcha.DriverDigit，得到 %T", driver)
	}
}

// 字符型配置里的宽/高/字符集/字符数/噪点必须真的落到驱动字段上。
//
// 后台改这些参数的全部意义就是"不发版也能改图"。如果某一项忘了搬运，
// 后果是保存成功、GET 读回新值，但生成的图还是旧样子——接口层完全看不出来，
// 只有人盯着验证码图片才发现，所以只能靠字段级比对兜住。
func TestBuildDriverStringCarriesImageParams(t *testing.T) {
	cfg := stringDriverConfig()
	cfg.Width = 220
	cfg.Height = 70
	cfg.Length = 5
	cfg.Source = "ABCEF"
	cfg.NoiseCount = 4
	cfg.ShowHollowLine = true
	cfg.ShowSlimeLine = true
	cfg.BgColor = "#11223344"

	raw, err := captcha.BuildDriver(cfg)
	if err != nil {
		t.Fatalf("字符型驱动应能构建，实际报错: %v", err)
	}
	driver, ok := raw.(*base64Captcha.DriverString)
	if !ok {
		t.Fatalf("应得到 *base64Captcha.DriverString，得到 %T", raw)
	}

	if driver.Width != 220 || driver.Height != 70 {
		t.Fatalf("图片尺寸应为 220x70，实际 %dx%d", driver.Width, driver.Height)
	}
	if driver.Length != 5 || driver.Source != "ABCEF" {
		t.Fatalf("字符数与字符集应为 5/ABCEF，实际 %d/%q", driver.Length, driver.Source)
	}
	if driver.NoiseCount != 4 {
		t.Fatalf("噪点数应为 4，实际 %d", driver.NoiseCount)
	}
	if driver.BgColor == nil {
		t.Fatal("字符型驱动必须带背景色：不给颜色时库会随机取浅色，后台配的背景色等于没生效")
	}
	// #11223344 是八位写法：第 7–8 位是 alpha，这里必须是 0x44 而不是不透明 0xFF。
	if want := (color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0x44}); *driver.BgColor != want {
		t.Fatalf("背景色应为 %+v，实际 %+v", want, *driver.BgColor)
	}
}

// 三个干扰线开关要翻成库的位值组合（空心=2、泥浆=4、正弦=8）。
//
// 位值写错不会报任何错：运营"打开了正弦线"却看不到正弦线，只会以为保存没生效。
// 这里把 2/4/8 钉成字面量而**不**引用 base64Captcha 的常量：配置/后台面板对外承诺的是
// 这三个位，库若在升级时偷偷改了编码，这条用例必须红，而不是跟着库一起漂。
func TestBuildDriverLineOptionsBitmask(t *testing.T) {
	cases := []struct {
		name                string
		hollow, slime, sine bool
		want                int
	}{
		{"三个都关（不画干扰线）", false, false, false, 0},
		{"只开空心线", true, false, false, 2},
		{"只开泥浆线", false, true, false, 4},
		{"只开正弦线", false, false, true, 8},
		{"空心 + 泥浆（默认组合）", true, true, false, 6},
		{"三个都开", true, true, true, 14},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := stringDriverConfig()
			cfg.ShowHollowLine = c.hollow
			cfg.ShowSlimeLine = c.slime
			cfg.ShowSineLine = c.sine

			raw, err := captcha.BuildDriver(cfg)
			if err != nil {
				t.Fatalf("字符型驱动应能构建，实际报错: %v", err)
			}
			driver := raw.(*base64Captcha.DriverString)
			if driver.ShowLineOptions != c.want {
				t.Fatalf("干扰线位值应为 %d，实际 %d", c.want, driver.ShowLineOptions)
			}
		})
	}
}

// 纯数字型只收尺寸/位数/倾斜/圆点四项，其余参数必须被忽略。
//
// 归一化虽然已经把 NoiseCount/BgColor/干扰线清零，但构建层也必须不依赖这一点：
// 后台如果绕过保存校验直接写库（或老数据里留了字符型的背景色），纯数字驱动
// 不该因为一个用不上的坏背景色就打不开登录页。
func TestBuildDriverDigitCarriesDigitParams(t *testing.T) {
	cfg := conf.Captcha{
		Driver:        captcha.DriverDigit,
		Width:         200,
		Height:        60,
		Length:        6,
		MaxSkew:       0.6,
		DotCount:      9,
		ExpireSeconds: 600,
		// 下面三项对纯数字型不适用，故意填坏/填开，证明构建层不看它们。
		BgColor:        "深蓝",
		NoiseCount:     5,
		ShowHollowLine: true,
		ShowSineLine:   true,
	}

	raw, err := captcha.BuildDriver(cfg)
	if err != nil {
		t.Fatalf("纯数字驱动不该因背景色/干扰线报错，实际: %v", err)
	}
	driver, ok := raw.(*base64Captcha.DriverDigit)
	if !ok {
		t.Fatalf("应得到 *base64Captcha.DriverDigit，得到 %T", raw)
	}
	if driver.Width != 200 || driver.Height != 60 {
		t.Fatalf("图片尺寸应为 200x60，实际 %dx%d", driver.Width, driver.Height)
	}
	if driver.Length != 6 {
		t.Fatalf("数字位数应为 6，实际 %d", driver.Length)
	}
	if driver.MaxSkew != 0.6 {
		t.Fatalf("最大倾斜应为 0.6，实际 %v", driver.MaxSkew)
	}
	if driver.DotCount != 9 {
		t.Fatalf("背景圆点应为 9，实际 %d", driver.DotCount)
	}
}

// 未知驱动名必须报错，不能静默回落到字符型。
//
// 静默回落的后果：运营在后台选了"纯数字"（想让手机键盘好输入），实际发的还是
// 字母图，用户在小键盘上根本敲不出来；而保存接口返回成功、接口层面毫无异常。
func TestBuildDriverRejectsUnknownDriver(t *testing.T) {
	cfg := stringDriverConfig()
	cfg.Driver = "wechat"

	driver, err := captcha.BuildDriver(cfg)
	if err == nil {
		t.Fatalf("未知驱动名必须报错，实际返回驱动 %T", driver)
	}
	if driver != nil {
		t.Fatalf("失败时不应返回半成品驱动，实际 %T", driver)
	}
	if !strings.Contains(err.Error(), "wechat") {
		t.Fatalf("错误应点明不认识的驱动名，实际: %v", err)
	}
}

// ParseBGColor 的六种输入形态。
//
// 背景色是运营从取色器里粘过来的十六进制串，写法天然有六位/八位/带不带 # 三种；
// 六位按不透明处理（用户没想到 alpha 时不该得到一张半透明图），八位第 7–8 位是 alpha。
// 非法输入必须报错而不是给个黑色兜底：黑底黑线的图会让用户以为"验证码坏了"，
// 而真正的原因只是运营粘错了一个字符。
func TestParseBGColor(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    color.RGBA
		wantErr bool
	}{
		{"六位带井号", "#112233", color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF}, false},
		{"六位省略井号", "112233", color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF}, false},
		{"八位带井号", "#11223344", color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0x44}, false},
		{"前后空白被忽略", "  #AABBCC  ", color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0xFF}, false},
		{"中文颜色名不接受", "深蓝", color.RGBA{}, true},
		{"五位十六进制非法", "#12345", color.RGBA{}, true},
		{"空串非法", "", color.RGBA{}, true},
		{"非十六进制字符", "#GGGGGG", color.RGBA{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := captcha.ParseBGColor(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("输入 %q 应报错，实际解析成 %+v", c.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("输入 %q 应解析成功，实际报错: %v", c.input, err)
			}
			if got != c.want {
				t.Fatalf("输入 %q 应解析成 %+v，实际 %+v", c.input, c.want, got)
			}
		})
	}
}
