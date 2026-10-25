package oauth

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/apperr"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// withMockTransport 替换 http.DefaultTransport，按请求路径返回预设响应。
func withMockTransport(t *testing.T, fn func(req *http.Request) (string, int)) {
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

func qqConfig() conf.QQ {
	return conf.QQ{AppID: "1", AppKey: "2", Redirect: "http://localhost/cb"}
}

func TestQQAccessToken(t *testing.T) {
	withMockTransport(t, func(req *http.Request) (string, int) {
		if req.URL.Query().Get("grant_type") == "authorization_code" {
			return `{"access_token":"at","expires_in":7200,"refresh_token":"rt","openid":"oid"}`, 200
		}
		return `{"ret":0,"nickname":"nick","figureurl_qq_2":"avatar"}`, 200
	})

	got, err := qqAccessToken(qqConfig(), "code1")
	if err != nil {
		t.Fatalf("qqAccessToken 失败: %v", err)
	}
	if got.AccessToken != "at" || got.OpenID != "oid" {
		t.Fatalf("qqAccessToken 返回异常: %+v", got)
	}
}

func TestGetQQUserInfoFlow(t *testing.T) {
	withMockTransport(t, func(req *http.Request) (string, int) {
		if req.URL.Query().Get("grant_type") == "authorization_code" {
			return `{"access_token":"at","expires_in":7200,"refresh_token":"rt","openid":"oid"}`, 200
		}
		return `{"ret":0,"nickname":"nick","figureurl_qq_2":"avatar"}`, 200
	})

	user, err := GetQQUserInfo(qqConfig(), "code2")
	if err != nil {
		t.Fatalf("GetQQUserInfo 失败: %v", err)
	}
	if user.OpenID != "oid" || user.NickName != "nick" || user.Avatar != "avatar" {
		t.Fatalf("GetQQUserInfo 返回异常: %+v", user)
	}
}

func TestQQAccessTokenFail(t *testing.T) {
	withMockTransport(t, func(req *http.Request) (string, int) {
		return `{"error_description":"bad code"}`, 200
	})

	_, err := qqAccessToken(qqConfig(), "bad")
	if err == nil {
		t.Fatal("access_token 缺失应报错")
	}
	// 失败必须带用户可翻译的错误码，而不是裸的 fmt.Errorf。
	if !apperr.IsCode(err, apperr.CodeUserOAuthFailed) {
		t.Fatalf("期望错误码 user.oauth_failed，实际=%v", err)
	}
}

// TestQQUserInfoErrorCode 覆盖 ret != 0 的分支。
func TestQQUserInfoErrorCode(t *testing.T) {
	withMockTransport(t, func(req *http.Request) (string, int) {
		return `{"ret":1002,"msg":"token invalid"}`, 200
	})

	_, err := qqUserInfo(qqConfig(), QQAccessToken{AccessToken: "at", OpenID: "oid"})
	if err == nil {
		t.Fatal("ret != 0 应报错")
	}
	if !apperr.IsCode(err, apperr.CodeUserOAuthFailed) {
		t.Fatalf("期望错误码 user.oauth_failed，实际=%v", err)
	}
}
