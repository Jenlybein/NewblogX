package user_service

import "errors"

var (
	ErrAuthRequired         = errors.New("请先登录")
	ErrAuthInvalid          = errors.New("登录已失效，请重新登录")
	ErrLoginTooFrequent     = errors.New("登录失败次数过多，请稍后再试")
	ErrSendTooFrequent      = errors.New("请求过于频繁，请稍后再试")
	ErrRefreshTokenRequired = errors.New("刷新令牌不存在，请重新登录")
	ErrRefreshTokenInvalid  = errors.New("刷新令牌无效或已过期，请重新登录")
)
