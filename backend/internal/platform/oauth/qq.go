// Package oauth 封装第三方登录的协议调用。
//
// 只做"拿 code 换用户信息"这一件事，不涉及账号建档 —— 那是 user 模块的职责。
package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"myblogx/conf"
	"myblogx/internal/apperr"
)

// QQAccessToken 是 QQ 换 token 接口的响应。
type QQAccessToken struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	ErrorDesc    string `json:"error_description"`
}

// QQUserInfoResp 是 QQ 用户信息接口的原始响应。
type QQUserInfoResp struct {
	Ret             int    `json:"ret"`
	Msg             string `json:"msg"`
	IsLost          int    `json:"is_lost"`
	NickName        string `json:"nickname"`
	Gender          string `json:"gender"`
	GenderType      int    `json:"gender_type"`
	Province        string `json:"province"`
	City            string `json:"city"`
	Year            string `json:"year"`
	FigureURL       string `json:"figureurl"`
	FigureURL1      string `json:"figureurl_1"`
	FigureURL2      string `json:"figureurl_2"`
	FigureURLQq1    string `json:"figureurl_qq_1"`
	FigureURLQq2    string `json:"figureurl_qq_2"`
	FigureURLQq     string `json:"figureurl_qq"`
	IsYellowVip     string `json:"is_yellow_vip"`
	Vip             string `json:"vip"`
	YellowVipLevel  string `json:"yellow_vip_level"`
	Level           string `json:"level"`
	IsYellowYearVip string `json:"is_yellow_year_vip"`
}

// QQUserInfo 是对外暴露的最小字段集。
type QQUserInfo struct {
	OpenID   string
	NickName string
	Avatar   string
}

// GetQQUserInfo 用授权 code 换取 QQ 用户信息。
func GetQQUserInfo(config conf.QQ, code string) (QQUserInfo, error) {
	at, err := qqAccessToken(config, code)
	if err != nil {
		return QQUserInfo{}, err
	}

	u, err := qqUserInfo(config, at)
	if err != nil {
		return QQUserInfo{}, err
	}

	return QQUserInfo{
		OpenID:   at.OpenID,
		NickName: u.NickName,
		// 用 figureurl_qq_2（100×100）作为头像，与旧实现一致。
		Avatar: u.FigureURLQq2,
	}, nil
}

// qqAccessToken 用 code 换 access_token 与 openid。
func qqAccessToken(config conf.QQ, code string) (QQAccessToken, error) {
	var atResp QQAccessToken

	baseURL, err := url.Parse("https://graph.qq.com/oauth2.0/token")
	if err != nil {
		return atResp, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}

	p := url.Values{}
	p.Add("grant_type", "authorization_code")
	p.Add("client_id", config.AppID)
	p.Add("client_secret", config.AppKey)
	p.Add("code", code)
	p.Add("redirect_uri", config.Redirect)
	p.Add("fmt", "json") // 不加则返回 x-www-form-urlencoded
	p.Add("need_openid", "1")
	baseURL.RawQuery = p.Encode()

	body, err := httpGet(baseURL.String())
	if err != nil {
		return atResp, err
	}
	if err = json.Unmarshal(body, &atResp); err != nil {
		return atResp, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}
	if atResp.AccessToken == "" {
		return atResp, apperr.Wrap(apperr.CodeUserOAuthFailed,
			fmt.Errorf("empty access_token: %s", atResp.ErrorDesc))
	}
	return atResp, nil
}

// qqUserInfo 用 access_token 拉用户资料。
//
// 已知可疑点（**本刀未改，保持原行为**）：这里的地址是 oauth2.0/token，
// 而 QQ 开放平台取用户资料的接口是 graph.qq.com/user/get_user_info。
// 原实现即如此，且没有真实 QQ 环境可验证改动后果，所以结构重构不夹带行为修正。
// 若要修，应单独一刀 + 用真实 AppID 做端到端验证。
func qqUserInfo(config conf.QQ, at QQAccessToken) (QQUserInfoResp, error) {
	var userInfoResp QQUserInfoResp

	baseURL, err := url.Parse("https://graph.qq.com/oauth2.0/token")
	if err != nil {
		return userInfoResp, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}

	p := url.Values{}
	p.Add("access_token", at.AccessToken)
	p.Add("oauth_consumer_key", config.AppID)
	p.Add("openid", at.OpenID)
	baseURL.RawQuery = p.Encode()

	body, err := httpGet(baseURL.String())
	if err != nil {
		return userInfoResp, err
	}
	if err = json.Unmarshal(body, &userInfoResp); err != nil {
		return userInfoResp, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}
	if userInfoResp.Ret != 0 {
		return userInfoResp, apperr.Wrap(apperr.CodeUserOAuthFailed,
			fmt.Errorf("qq ret=%d msg=%s", userInfoResp.Ret, userInfoResp.Msg))
	}
	return userInfoResp, nil
}

// httpGet 发起 GET 并读完响应。
//
// 原实现漏了 resp.Body.Close()，每次登录泄漏一条连接（连接池耗尽后
// 表现为"登录越来越慢"）。这里补上，属于纯资源修复，不改变调用语义。
func httpGet(rawURL string) ([]byte, error) {
	res, err := http.Get(rawURL)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUserOAuthFailed, err)
	}
	return body, nil
}
