package qq_service

import (
	"io"
	"myblogx/conf"
	"myblogx/test/testutil"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withMockQQTransport(t *testing.T, fn func(req *http.Request) (string, int)) {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, code := fn(req)
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
}

func TestGetAccessToken(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QQ: conf.QQ{AppID: "1", AppKey: "2", Redirect: "http://localhost/cb"},
	})
	qqConf := testutil.Config().QQ

	withMockQQTransport(t, func(req *http.Request) (string, int) {
		if req.URL.Query().Get("grant_type") == "authorization_code" {
			return `{"access_token":"at","expires_in":7200,"refresh_token":"rt","openid":"oid"}`, 200
		}
		return `{"ret":0,"nickname":"nick","figureurl_qq_2":"avatar"}`, 200
	})

	resp, err := getAccessToken(qqConf, "code1")
	if err != nil {
		t.Fatalf("getAccessToken 失败: %v", err)
	}
	if resp.AccessToken != "at" || resp.OpenID != "oid" {
		t.Fatalf("getAccessToken 返回异常: %+v", resp)
	}
}

func TestGetUserInfoAndGetUserInfoFlow(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QQ: conf.QQ{AppID: "1", AppKey: "2", Redirect: "http://localhost/cb"},
	})
	qqConf := testutil.Config().QQ

	withMockQQTransport(t, func(req *http.Request) (string, int) {
		if req.URL.Query().Get("grant_type") == "authorization_code" {
			return `{"access_token":"at","expires_in":7200,"refresh_token":"rt","openid":"oid"}`, 200
		}
		return `{"ret":0,"nickname":"nick","figureurl_qq_2":"avatar"}`, 200
	})

	user, err := GetUserInfo(qqConf, "code2")
	if err != nil {
		t.Fatalf("GetUserInfo 失败: %v", err)
	}
	if user.OpenID != "oid" || user.NickName != "nick" || user.Avatar != "avatar" {
		t.Fatalf("GetUserInfo 返回异常: %+v", user)
	}
}

func TestGetAccessTokenFail(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		QQ: conf.QQ{AppID: "1", AppKey: "2", Redirect: "http://localhost/cb"},
	})
	qqConf := testutil.Config().QQ

	withMockQQTransport(t, func(req *http.Request) (string, int) {
		return `{"error_description":"bad code"}`, 200
	})

	if _, err := getAccessToken(qqConf, "bad"); err == nil {
		t.Fatal("access_token 缺失应报错")
	}
}
