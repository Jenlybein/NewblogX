// 响应封装

package res

import (
	"myblogx/utils/validate"

	"github.com/gin-gonic/gin"
)

type Code int

const (
	SuccessCode     Code = 0
	FailValidCode   Code = 1001
	FailServiceCode Code = 1002
	// FailAuthCode 是"登录态不可用"的专属码。
	//
	// 为什么需要一个独立的码：鉴权失败此前也塌缩成 1001，与"参数错误"
	// 无法区分，前端只能靠错误消息里的中文关键词判断（"登录""令牌"…），
	// 而文案一改就断。有了专属码，前端可以只认码、不认文案。
	FailAuthCode Code = 1003
)

type Response struct {
	Code Code   `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func (c Code) String() string {
	switch c {
	case SuccessCode:
		return "成功"
	case FailValidCode:
		return "校验失败"
	case FailServiceCode:
		return "服务异常"
	case FailAuthCode:
		return "登录态不可用"
	}
	return ""
}

func (r Response) Json(c *gin.Context) {
	c.JSON(200, r)
}

func OkWithData(data any, c *gin.Context) {
	Response{
		Code: SuccessCode,
		Data: data,
		Msg:  "成功",
	}.Json(c)
}

func OkWithMsg(msg string, c *gin.Context) {
	Response{
		Code: SuccessCode,
		Data: map[string]any{},
		Msg:  msg,
	}.Json(c)
}

func OkWithList(list any, count int, c *gin.Context) {
	Response{
		Code: SuccessCode,
		Data: map[string]any{
			"list":  list,
			"count": count,
		},
		Msg: "成功",
	}.Json(c)
}

func OkWithHasMoreList(list any, hasMore bool, c *gin.Context) {
	Response{
		Code: SuccessCode,
		Data: map[string]any{
			"list":     list,
			"has_more": hasMore,
		},
		Msg: "成功",
	}.Json(c)
}

func FailWithMsg(msg string, c *gin.Context) {
	Response{
		Code: FailValidCode,
		Data: map[string]any{},
		Msg:  msg,
	}.Json(c)
}

// FailWithAuthMsg 返回"登录态不可用"。
//
// 用专属码 {@link FailAuthCode} 而不是 1001：前端据此判断要不要刷新令牌，
// 不需要再匹配错误消息里的中文关键词。
func FailWithAuthMsg(msg string, c *gin.Context) {
	Response{
		Code: FailAuthCode,
		Data: map[string]any{},
		Msg:  msg,
	}.Json(c)
}

func FailWithData(data any, msg string, c *gin.Context) {
	Response{
		Code: FailServiceCode,
		Data: data,
		Msg:  msg,
	}.Json(c)
}

func FailWithError(err error, c *gin.Context) {
	data, msg := validate.ValidateError(err)
	FailWithData(data, msg, c)
}
