package captcha_api_test

import (
	"encoding/json"
	"myblogx/api/captcha_api"
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/models"
	"myblogx/service/site_service"
	"myblogx/test/testutil"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type captchaResp struct {
	Code int            `json:"code"`
	Data map[string]any `json:"data"`
	Msg  string         `json:"msg"`
}

func newCaptchaCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func readCaptchaResp(t *testing.T, w *httptest.ResponseRecorder) captchaResp {
	t.Helper()
	var resp captchaResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析验证码接口响应失败: %v, body=%s", err, w.Body.String())
	}
	return resp
}

func setupCaptchaConfig(t *testing.T, enable bool) {
	t.Helper()
	testutil.InitGlobals()
	_ = testutil.SetupSQLite(t, &models.RuntimeSiteConfigModel{})
	testutil.SetConfig(&conf.Config{
		Site: conf.Site{
			Login: confsite.Login{Captcha: enable},
		},
	})
}

func newCaptchaAPI(t *testing.T) captcha_api.ImageCaptchaApi {
	t.Helper()
	runtimeSvc := site_service.NewRuntimeConfigService(testutil.Config().Site, testutil.Config().AI, testutil.Logger(), testutil.DB(), "")
	if err := runtimeSvc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化运行时配置失败: %v", err)
	}
	return captcha_api.New(captcha_api.Deps{
		RuntimeSite:       runtimeSvc,
		ImageCaptchaStore: testutil.ImageCaptchaStore(),
	})
}

func TestCaptchaViewDisabled(t *testing.T) {
	setupCaptchaConfig(t, false)
	api := newCaptchaAPI(t)

	c, w := newCaptchaCtx()
	api.CaptchaView(c)

	resp := readCaptchaResp(t, w)
	if resp.Code == 0 {
		t.Fatalf("验证码关闭时应返回失败, body=%s", w.Body.String())
	}
}

func TestCaptchaViewSuccess(t *testing.T) {
	setupCaptchaConfig(t, true)
	api := newCaptchaAPI(t)

	c, w := newCaptchaCtx()
	api.CaptchaView(c)

	resp := readCaptchaResp(t, w)
	if resp.Code != 0 {
		t.Fatalf("验证码生成应成功, body=%s", w.Body.String())
	}

	id, ok := resp.Data["captcha_id"].(string)
	if !ok || id == "" {
		t.Fatalf("captcha_id 为空或类型错误, data=%v", resp.Data)
	}

	base64, ok := resp.Data["base64"].(string)
	if !ok || len(base64) < 16 {
		t.Fatalf("base64 为空或类型错误, data=%v", resp.Data)
	}
}
