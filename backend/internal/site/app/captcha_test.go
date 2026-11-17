package app_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"
	captchainst "myblogx/internal/platform/captcha"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/internal/site/app"
	"myblogx/internal/site/apptest"
)

// assertCaptchaInvalid 断言"这条配置被拒绝、错误码是 captcha 段专属、原因点明了哪个字段"。
//
// 为什么断言 UserMessage 而不是 err.Error()：apperr 的 Error() 只回消息表里的
// 通用文案（"图形验证码配置不合法：类型、尺寸、有效期…"），字段级原因走
// apperr.NewWithMessage 的受控文案。断言它等于保证"后台填错 width 看到的是
// '图片宽度需在 80–400 之间'，而不是笼统的'配置不合法'"，否则运营还得自己猜。
func assertCaptchaInvalid(t *testing.T, label string, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s 应被拒绝，实际保存成功了", label)
	}
	if !apperr.IsCode(err, apperr.CodeSiteCaptchaInvalid) {
		t.Fatalf("%s 应以 %s 拒绝，实际 %v", label, apperr.CodeSiteCaptchaInvalid, err)
	}
	// 断言 UserMessage（而不是 Unwrap）：那才是后台真正会显示给运营的那一句。
	// 走 apperr.NewWithMessage 之后具体原因不再是被包住 cause，而是错误值自带的
	// 受控文案；响应层只对 Validation 类错误开放这条通道（res.Fail / FailWithValidError）。
	var carrier interface{ UserMessage() string }
	if !errors.As(err, &carrier) {
		t.Fatalf("%s 的错误没有带受控文案，后台无法知道该改哪一项", label)
	}
	if detail := carrier.UserMessage(); !strings.Contains(detail, field) {
		t.Fatalf("%s 的文案应点明字段 %q，实际: %v", label, field, detail)
	}
}

// 全零输入（等价于"从来没配过这一段"）要回落到一份能直接用的配置。
//
// 这一段是从 site 段拆出来的（ADR-0033），而 site 段的历史默认值只写在
// site_default_settings.yaml；用例不能假设调用方一定带着完整对象进来 ——
// 少了回落，"零值"就会变成"宽 0 高 0、长度 0、空字符集"，生成接口连驱动都建不出来。
//
// 注意三个布尔干扰线开关不在回落之列（布尔没有"未填"语义，见文件末尾的说明）：
// 这里断言的是"零值不回落"的真实口径。票面默认值（空心 + 泥浆开、正弦关）由
// site_default_settings.yaml 提供，由 TestUpdateCaptchaKeepsShippedDefaults 兜住。
func TestNormalizeCaptchaZeroInputDefaults(t *testing.T) {
	got, err := app.NormalizeCaptcha(conf.Captcha{})
	if err != nil {
		t.Fatalf("全零输入应回落默认值而不是报错，实际: %v", err)
	}

	if got.Driver != "string" {
		t.Fatalf("默认驱动应为 string，实际 %q", got.Driver)
	}
	if got.Width != 200 || got.Height != 60 {
		t.Fatalf("默认图片尺寸应为 200x60，实际 %dx%d", got.Width, got.Height)
	}
	if got.ExpireSeconds != 600 {
		t.Fatalf("默认有效期应为 600 秒，实际 %d", got.ExpireSeconds)
	}
	if got.Length != 4 {
		t.Fatalf("默认字符数应为 4，实际 %d", got.Length)
	}
	if got.Source != app.DefaultCaptchaSource {
		t.Fatalf("默认字符集应为 %q，实际 %q", app.DefaultCaptchaSource, got.Source)
	}
	if got.BgColor != app.DefaultCaptchaBgColor {
		t.Fatalf("默认背景色应为 %q，实际 %q", app.DefaultCaptchaBgColor, got.BgColor)
	}
	// 噪点 0 是**合法值**（"不放噪点"），不是"没填"：全零输入里它保持 0。
	// 这一条被两轮评审盯过 —— 早先实现把它回落成 3，等于把运营明确填的 0 偷偷改了。
	if got.NoiseCount != 0 {
		t.Fatalf("噪点 0 应原样保留（表示不放噪点），实际 %d", got.NoiseCount)
	}
	// 非纯数字驱动不读这两项，但**不再清零**：清掉会让 GET→PUT 往返漂移
	// （yaml 种子 max_skew=0.5 存进去变 0），也会让运营切到纯数字后看到陌生的值。
	// 全零输入下它们自然是 0，这里断言的是"没有被额外改写"。
	if got.MaxSkew != 0 || got.DotCount != 0 {
		t.Fatalf("非纯数字驱动的 max_skew/dot_count 应为 0（不适用），实际 %v/%d",
			got.MaxSkew, got.DotCount)
	}
	// 布尔开关没有"未填"状态：零值就是 false。断言这一点的意义是让"零值输入"的
	// 实际落点被钉住 —— 手工 PUT 一个 `{}` 确实会把三条干扰线全部关掉（票面口径与
	// 实现的这点差异已作为生产问题上报，见报告；真实的管理端总是回填 GET 的完整对象）。
	if got.ShowHollowLine || got.ShowSlimeLine || got.ShowSineLine {
		t.Fatalf("布尔开关的零值不应被回落到 true，实际 空心=%v 泥浆=%v 正弦=%v",
			got.ShowHollowLine, got.ShowSlimeLine, got.ShowSineLine)
	}
}

// 越界一律拒绝，且原因点明字段（不静默夹取）。
//
// 口径是"越界即拒绝"，而不是"夹到边界"：后台把 width 填成 5000 却得到
// "保存成功"，运营看到的是一张能撑爆登录框的图，排查时没有任何线索；
// 夹取还会让"我填了 5000"和"我看到 400"之间没有任何提示。每一条都单独列出来，
// 因为任一字段的边界写歪（改常量时最容易）都只影响它自己，必须逐条守。
func TestNormalizeCaptchaRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name  string
		input conf.Captcha
		field string
	}{
		{"宽度超上限", conf.Captcha{Width: 5000}, "宽度"},
		{"高度低于下限", conf.Captcha{Height: 10}, "高度"},
		{"有效期低于下限", conf.Captcha{ExpireSeconds: 30}, "有效期"},
		{"字符型字符数超上限", conf.Captcha{Length: 9}, "字符数"},
		{"纯数字型字符数低于下限", conf.Captcha{Driver: "digit", Length: 2}, "字符数"},
		{"噪点数超上限", conf.Captcha{NoiseCount: 99}, "噪点数"},
		{"纯数字型背景点数超上限", conf.Captcha{Driver: "digit", DotCount: 99}, "背景点数"},
		{"纯数字型倾斜超上限", conf.Captcha{Driver: "digit", MaxSkew: 2}, "倾斜"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := app.NormalizeCaptcha(c.input)
			if err == nil {
				t.Fatalf("%s：应报错，实际得到 %+v", c.name, got)
			}
			assertCaptchaInvalid(t, c.name, err, c.field)
		})
	}
}

// 算术型的 Length 一律清 0（题目固定是两位数运算，长度对它没有意义）。
//
// 不清 0 的后果：后台从字符型切到算术型时，旧的 length=7 会留在响应里，
// 运营以为自己改成了"7 位算术题"，而实际题目永远是 "12+7=?" 这种两位数运算 ——
// 配置面板在撒谎。写 0 是唯一的"不适用"表达，前端据此禁用输入框。
func TestNormalizeCaptchaMathClearsLength(t *testing.T) {
	got, err := app.NormalizeCaptcha(conf.Captcha{Driver: "math", Length: 9})
	if err != nil {
		// 长度对算术型不适用，越界值也不该在这里报错：报错会让"从字符型切过来的
		// 旧配置"卡住保存，而它本来就不是算术型的参数。
		t.Fatalf("算术型不该因 length 报错，实际: %v", err)
	}
	if got.Driver != "math" {
		t.Fatalf("驱动应为 math，实际 %q", got.Driver)
	}
	if got.Length != 0 {
		t.Fatalf("算术型的 length 应被清 0，实际 %d", got.Length)
	}
}

// 纯数字型把不适用的项清零，并把 MaxSkew 落到 0.5。
//
// 这些项对 digit 驱动没有意义：不清零的话，后台从字符型切到纯数字型会带来
// "背景色 #0A14320A""噪点 3""三条干扰线"这些 digit 驱动根本不读的字段。
// 它们在 GET 里存在会让人以为还在生效（改了背景色却没变化），而 MaxSkew 是
// digit 真正读的参数 —— 0 会让所有数字笔直排列（等于没有倾斜），所以默认 0.5。
func TestNormalizeCaptchaDigitClearsInapplicable(t *testing.T) {
	got, err := app.NormalizeCaptcha(conf.Captcha{
		Driver:         "digit",
		NoiseCount:     5,
		BgColor:        "深蓝",
		ShowHollowLine: true,
		ShowSlimeLine:  true,
		ShowSineLine:   true,
		MaxSkew:        0,
	})
	if err != nil {
		// 不适用项的脏值不该拦住保存：它们本来就会被丢掉，报错只会让切换驱动变难。
		t.Fatalf("纯数字型应清掉不适用项而不是报错，实际: %v", err)
	}

	if got.Driver != "digit" {
		t.Fatalf("驱动应为 digit，实际 %q", got.Driver)
	}
	if got.NoiseCount != 0 {
		t.Fatalf("纯数字型不应带噪点，实际 %d", got.NoiseCount)
	}
	if got.BgColor != "" {
		t.Fatalf("纯数字型不应带背景色，实际 %q", got.BgColor)
	}
	if got.ShowHollowLine || got.ShowSlimeLine || got.ShowSineLine {
		t.Fatalf("纯数字型不应带干扰线开关，实际 空心=%v 泥浆=%v 正弦=%v",
			got.ShowHollowLine, got.ShowSlimeLine, got.ShowSineLine)
	}
	// 倾斜 0 是**合法值**（不倾斜）：全零输入下不该被悄悄改成 0.5。
	// 想让数字带倾斜的站点在后台填正数，或让 yaml 种子（0.5）随首次保存带进来。
	if got.MaxSkew != 0 {
		t.Fatalf("全零输入下倾斜应保持 0（合法值，不倾斜），实际 %v", got.MaxSkew)
	}
}

// 字符集去掉重复字符，去重后不足 2 个字符就报错。
//
// 重复字符不是无害的：同一个字符出现两遍，等于把可用字符集砍掉一半（旧默认值
// `…IJKLHIJKL…` 就是这么白送一半难度的），而字面上完全看不出来。少于 2 个字符
// 则根本不是验证码（只有一种答案），必须当场拒绝而不是生成一张"永远唯一答案"的图。
func TestNormalizeCaptchaDedupsSource(t *testing.T) {
	got, err := app.NormalizeCaptcha(conf.Captcha{Driver: "string", Source: "AABBCC", BgColor: "#123456"})
	if err != nil {
		t.Fatalf("重复字符应被去重而不是报错，实际: %v", err)
	}
	if got.Source != "ABC" {
		t.Fatalf("字符集 AABBCC 去重后应为 ABC，实际 %q", got.Source)
	}

	if _, err := app.NormalizeCaptcha(conf.Captcha{Driver: "string", Source: "AA"}); err == nil {
		t.Fatal("去重后只剩 1 个字符的字符集应被拒绝（那不是验证码）")
	} else {
		assertCaptchaInvalid(t, "去重后不足 2 个字符", err, "字符集")
	}
}

// 字符集只做"去空白 + 去重"，不做大小写折叠、也不做字符白名单。
//
// 这里钉住两个真实口径，它们都是"输入会被原样当作字符集"的直接后果：
//   - " String " 去掉首尾空白后是 "String"（首字母大小写保留）：答案校验本来就不
//     区分大小写，折叠字符集只会把 A 和 a 并成一个字符，等于白丢一半可用字符；
//     票面口径写的是 →"string"，与实现不一致，已作为差异上报。
//   - "weird" 是合法字符集：只要去重后 ≥2 个字符就没有理由拒绝，字符集本来就能
//     由运营自定（票面口径期望它报错，实现没有字符白名单，差异同上）。
func TestNormalizeCaptchaSourceIsTrimmedNotRewritten(t *testing.T) {
	got, err := app.NormalizeCaptcha(conf.Captcha{Driver: "string", Source: " String "})
	if err != nil {
		t.Fatalf("带空白的字符集应被整理而不是报错，实际: %v", err)
	}
	if got.Source != "String" {
		t.Fatalf("字符集应只去空白、保留大小写，实际 %q", got.Source)
	}

	got, err = app.NormalizeCaptcha(conf.Captcha{Driver: "string", Source: "weird"})
	if err != nil {
		t.Fatalf("自定义字符集（无重复、≥2 字符）应被接受，实际: %v", err)
	}
	if got.Source != "weird" {
		t.Fatalf("自定义字符集应原样保留，实际 %q", got.Source)
	}
}

// 管理端 GET→PUT 往返（保存票面默认值）必须原样保留配置。
//
// 这是配图与时效参数的默认值真正落地的地方：GET 返回 site_default_settings.yaml
// 里的那份（空心线 + 泥浆线开、正弦线关），管理端原样提交回来时不能被归一化改掉
// 任何一项。一旦往返发生漂移（例如某次改归一化顺手把布尔开关清零），运营点一次
// "保存"就会把干扰线全关掉，而且界面显示的还是保存成功 —— 图却变样了。
func TestUpdateCaptchaKeepsShippedDefaults(t *testing.T) {
	env := apptest.NewEnv(conf.Site{}, conf.AI{}, "test", "")

	shipped := conf.Captcha{
		Driver:         "string",
		Length:         4,
		Width:          200,
		Height:         60,
		ExpireSeconds:  600,
		Source:         app.DefaultCaptchaSource,
		NoiseCount:     3,
		ShowHollowLine: true,
		ShowSlimeLine:  true,
		ShowSineLine:   false,
		BgColor:        app.DefaultCaptchaBgColor,
		MaxSkew:        0,
		DotCount:       0,
	}

	if err := env.Service.UpdateCaptcha(shipped); err != nil {
		t.Fatalf("保存默认配置应成功，实际: %v", err)
	}
	// 保存即生效：缓存里就是新值，没有"重建"这一步。
	if got := env.Runtime.Captcha(); got != shipped {
		t.Fatalf("运行时缓存应等于刚保存的配置\n期望 %+v\n实际 %+v", shipped, got)
	}
	// 管理端能原样读回：GET 是后台表单的初始值，读回不一致会让表单"看起来没保存"。
	if got := env.Service.AdminCaptcha(); got != shipped {
		t.Fatalf("管理读应原样返回\n期望 %+v\n实际 %+v", shipped, got)
	}
}

// 坏配置不能进缓存（保存失败后运行时配置必须保持原样）。
//
// 校验发生在用例层而不是写入层，就是为了"让坏数据进不了库"：如果先写缓存再校验，
// 一次填错 width 会让**整站登录页立刻开始返回 500 或坏图**，直到有人再保存一次。
// 所以失败后必须确认缓存的还是上一份好配置。
func TestUpdateCaptchaRejectsInvalidAndKeepsPrevious(t *testing.T) {
	env := apptest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	previous := conf.Captcha{
		Driver:         "string",
		Width:          200,
		Height:         60,
		Length:         4,
		ExpireSeconds:  600,
		Source:         app.DefaultCaptchaSource,
		BgColor:        app.DefaultCaptchaBgColor,
		NoiseCount:     3,
		ShowHollowLine: true,
		ShowSlimeLine:  true,
	}
	if err := env.Service.UpdateCaptcha(previous); err != nil {
		t.Fatalf("预置合法配置失败: %v", err)
	}

	if err := env.Service.UpdateCaptcha(conf.Captcha{Width: 5000}); !apperr.IsCode(err, apperr.CodeSiteCaptchaInvalid) {
		t.Fatalf("越界宽度应以 %s 拒绝，实际 %v", apperr.CodeSiteCaptchaInvalid, err)
	}
	if got := env.Runtime.Captcha(); got != previous {
		t.Fatalf("保存失败后缓存必须保持上一份配置\n期望 %+v\n实际 %+v", previous, got)
	}
}

// 代码里的回落值必须与 site_default_settings.yaml 的种子逐字段一致。
//
// GLOSSARY 的 `_Avoid_` 说得很清楚：默认值只能有一处。现实是 normalize 看不到
// yaml（它只收到一个结构体），所以回落常量不得不在代码里存在一份 —— 那就用这条
// 用例把两份钉死：改了一边忘了另一边，这里立刻红，而不是等运营发现
// "我填 0 怎么变成 3 了"或者"默认宽度怎么不是 200"。
func TestNormalizeFallbacksMatchShippedDefaults(t *testing.T) {
	// LoadDefaults 取同目录下的 site_default_settings.yaml，所以传 config 目录里的任意文件。
	defaults, err := sitecfg.LoadDefaults(filepath.Join("..", "..", "..", "config", "settings.yaml"))
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	shipped := defaults.Captcha
	if shipped.Driver != "string" {
		t.Fatalf("yaml 默认驱动应为 string，实际 %q", shipped.Driver)
	}

	got, err := app.NormalizeCaptcha(conf.Captcha{})
	if err != nil {
		t.Fatalf("全零输入应能归一化: %v", err)
	}
	if got.Width != shipped.Width || got.Height != shipped.Height {
		t.Fatalf("尺寸回落值应与 yaml 一致：得到 %dx%d，yaml 是 %dx%d",
			got.Width, got.Height, shipped.Width, shipped.Height)
	}
	if got.ExpireSeconds != shipped.ExpireSeconds {
		t.Fatalf("有效期回落值应与 yaml 一致：得到 %d，yaml 是 %d", got.ExpireSeconds, shipped.ExpireSeconds)
	}
	if got.Length != shipped.Length {
		t.Fatalf("字符数回落值应与 yaml 一致：得到 %d，yaml 是 %d", got.Length, shipped.Length)
	}
	if got.Source != shipped.Source {
		t.Fatalf("字符集回落值应与 yaml 一致：得到 %q，yaml 是 %q", got.Source, shipped.Source)
	}
	if got.BgColor != shipped.BgColor {
		t.Fatalf("背景色回落值应与 yaml 一致：得到 %q，yaml 是 %q", got.BgColor, shipped.BgColor)
	}
	// 噪点/倾斜/点数不在回落之列（0 是合法值），但 yaml 的种子值必须是"能存进去"的：
	// 把它们走一遍归一化，验证种子不会被改写（早期实现把 max_skew 0.5 清零过）。
	if seeded, err := app.NormalizeCaptcha(shipped); err != nil {
		t.Fatalf("yaml 种子应能通过归一化: %v", err)
	} else if seeded.NoiseCount != shipped.NoiseCount ||
		seeded.MaxSkew != shipped.MaxSkew ||
		seeded.DotCount != shipped.DotCount {
		t.Fatalf("yaml 种子经归一化被改写了：噪点 %d->%d，倾斜 %v->%v，点数 %d->%d",
			shipped.NoiseCount, seeded.NoiseCount,
			shipped.MaxSkew, seeded.MaxSkew,
			shipped.DotCount, seeded.DotCount)
	}
}

// 归一化产出的每种 driver 都必须能被图片生成实现接受。
//
// 这是一条端到端缝：app 包用私有字符串常量、platform/captcha 用导出常量，
// 两边漂移（比如一边改成 "digital"）不会在任何一个包内被发现，只会在
// 线上"保存成功但生成 500"。这里把它们接在一起。
func TestNormalizeCaptchaDriversBuildDrivers(t *testing.T) {
	for _, driver := range []string{"string", "math", "digit"} {
		got, err := app.NormalizeCaptcha(conf.Captcha{Driver: driver})
		if err != nil {
			t.Fatalf("驱动 %q 归一化失败: %v", driver, err)
		}
		if _, err := captchainst.BuildDriver(got); err != nil {
			t.Fatalf("归一化产出的驱动 %q 无法构建图片驱动: %v", got.Driver, err)
		}
	}
}
