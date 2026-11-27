package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"

	"github.com/gin-gonic/gin"
)

// loginCapabilitiesBody 是端点响应的 data 载荷（与契约里的形状一一对应）。
type loginCapabilitiesBody struct {
	LoginMethods struct {
		UsernamePwd bool `json:"username_pwd"`
		Email       bool `json:"email"`
		Phone       bool `json:"phone"`
		QQ          bool `json:"qq"`
	} `json:"login_methods"`
	CaptchaRequired struct {
		Login       bool `json:"login"`
		EmailVerify bool `json:"email_verify"`
		PhoneVerify bool `json:"phone_verify"`
		Register    bool `json:"register"`
	} `json:"captcha_required"`
}

// readLoginCapabilities 调一次处理器并解出 data 载荷。
func readLoginCapabilities(t *testing.T, h *sitehttp.Handler) loginCapabilitiesBody {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/site/login_capabilities", nil)
	h.LoginCapabilities(c)

	var envelope struct {
		Code int                   `json:"code"`
		Data loginCapabilitiesBody `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, recorder.Body.String())
	}
	if envelope.Code != 0 {
		t.Fatalf("响应业务码应为 0，实际 %d body=%s", envelope.Code, recorder.Body.String())
	}
	return envelope.Data
}

// 端点把「登录方式」与「验证码闸门」拼成固定形状，且每一档都来自各自所属的段。
//
// 断言到具体字段而不仅是"非空"，是因为这条端点存在的全部理由就是从五个段里
// 取对值：取错一段（例如继续读 site.login 里已经被删掉的 email_login）会静默
// 变成 false，前端就会把登录入口藏起来，而没有任何报错。
func TestLoginCapabilitiesReadsEachSection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{Login: siteconf.Login{UsernamePwdLogin: true}}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)

	// 各段都先给出与 site.login 相反的值，证明读的不是 site 段。
	if err := env.Runtime.UpdateMail(conf.Mail{EmailLogin: true, EmailRegister: true}); err != nil {
		t.Fatalf("写入邮件段失败: %v", err)
	}
	if err := env.Runtime.UpdateSMS(conf.SMS{PhoneLogin: true}); err != nil {
		t.Fatalf("写入短信段失败: %v", err)
	}
	if err := env.Runtime.UpdateOAuth(conf.QQ{QQLogin: false}); err != nil {
		t.Fatalf("写入第三方登录段失败: %v", err)
	}
	if err := env.Runtime.UpdateCaptcha(conf.Captcha{
		Captcha:            true,
		CaptchaLogin:       true,
		CaptchaEmailVerify: true,
		CaptchaPhoneVerify: false,
		CaptchaRegister:    true,
	}); err != nil {
		t.Fatalf("写入验证码段失败: %v", err)
	}

	got := readLoginCapabilities(t, h)
	if !got.LoginMethods.UsernamePwd || !got.LoginMethods.Email || !got.LoginMethods.Phone || got.LoginMethods.QQ {
		t.Fatalf("登录方式应来自各段（password/email/phone 开、qq 关），实际 %+v", got.LoginMethods)
	}
	if !got.CaptchaRequired.Login || !got.CaptchaRequired.EmailVerify ||
		got.CaptchaRequired.PhoneVerify || !got.CaptchaRequired.Register {
		t.Fatalf("验证码闸门应逐入口如实给出，实际 %+v", got.CaptchaRequired)
	}

	// 每次请求现读：改一次配置，下一次调用就要变（缓存住会让"刚关掉邮箱登录、
	// 读者站还在显示入口"持续到重启）。
	if err := env.Runtime.UpdateMail(conf.Mail{EmailLogin: false}); err != nil {
		t.Fatalf("写入邮件段失败: %v", err)
	}
	if after := readLoginCapabilities(t, h); after.LoginMethods.Email {
		t.Fatal("邮箱登录关掉后，下一次请求应立刻读到 false")
	}
}

// 总开关关掉时四个闸门一律 false：端点给的是**生效值**，不是原始闸门。
//
// 分开给两个开关会让每个调用点自己再与一次，漏一处就渲染出一个永远不被校验的
// 输入框（用户填了不报错，看起来像验证码坏了）。
func TestLoginCapabilitiesFoldsMasterCaptchaSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	h := sitehttp.NewHandler(env.Service, nil)
	if err := env.Runtime.UpdateCaptcha(conf.Captcha{
		Captcha:            false,
		CaptchaLogin:       true,
		CaptchaEmailVerify: true,
		CaptchaPhoneVerify: true,
		CaptchaRegister:    true,
	}); err != nil {
		t.Fatalf("写入验证码段失败: %v", err)
	}

	got := readLoginCapabilities(t, h)
	if got.CaptchaRequired.Login || got.CaptchaRequired.EmailVerify ||
		got.CaptchaRequired.PhoneVerify || got.CaptchaRequired.Register {
		t.Fatalf("总开关关掉时四个闸门都应为 false，实际 %+v", got.CaptchaRequired)
	}
}
