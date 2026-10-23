package qq_service

import (
	"encoding/json"
	"fmt"
	"io"
	"myblogx/conf"
	"net/http"
	"net/url"
)

type QQAccessTokenResp struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	ErrorDesc    string `json:"error_description"`
}

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

type QQUserInfo struct {
	OpenID   string
	NickName string
	Avatar   string
}

func getAccessToken(config conf.QQ, code string) (atResp QQAccessTokenResp, err error) {
	qq := config

	baseURL, err := url.Parse("https://graph.qq.com/oauth2.0/token")
	if err != nil {
		return atResp, err
	}

	p := url.Values{}
	p.Add("grant_type", "authorization_code")
	p.Add("client_id", qq.AppID)
	p.Add("client_secret", qq.AppKey)
	p.Add("code", code)
	p.Add("redirect_uri", qq.Redirect)
	p.Add("fmt", "json") // 可选，默认则返回x-www-form-urlencoded格式
	p.Add("need_openid", "1")

	baseURL.RawQuery = p.Encode()

	res, err := http.Get(baseURL.String())
	if err != nil {
		return atResp, err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return atResp, err
	}

	if err = json.Unmarshal(body, &atResp); err != nil {
		return atResp, err
	}

	if atResp.AccessToken == "" {
		return atResp, fmt.Errorf("获取access_token失败 %s", atResp.ErrorDesc)
	}

	return atResp, nil
}

func getUserInfo(config conf.QQ, at QQAccessTokenResp) (userInfoResp QQUserInfoResp, err error) {
	qq := config

	baseURL, err := url.Parse("https://graph.qq.com/oauth2.0/token")
	if err != nil {
		return
	}

	p := url.Values{}
	p.Add("access_token", at.AccessToken)
	p.Add("oauth_consumer_key", qq.AppID)
	p.Add("openid", at.OpenID)

	baseURL.RawQuery = p.Encode()

	res, err := http.Get(baseURL.String())
	if err != nil {
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &userInfoResp)
	if err != nil {
		return
	}
	if userInfoResp.Ret != 0 {
		return userInfoResp, fmt.Errorf("获取用户信息失败：%s", userInfoResp.Msg)
	}
	return userInfoResp, nil
}

func GetUserInfo(config conf.QQ, code string) (info QQUserInfo, err error) {
	at, err := getAccessToken(config, code)
	if err != nil {
		return
	}

	u, err := getUserInfo(config, at)
	if err != nil {
		return
	}

	info = QQUserInfo{
		OpenID:   at.OpenID,
		NickName: u.NickName,
		Avatar:   u.FigureURLQq2,
	}

	return
}
