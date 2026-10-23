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
	}
	return ""
}

func (r Response) Json(c *gin.Context) {
	c.JSON(200, r)
}

func Ok(data any, msg string, c *gin.Context) {
	Response{
		Code: SuccessCode,
		Data: data,
		Msg:  msg,
	}.Json(c)
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

func FailWithData(data any, msg string, c *gin.Context) {
	Response{
		Code: FailServiceCode,
		Data: data,
		Msg:  msg,
	}.Json(c)
}

func FailWithCode(code Code, c *gin.Context) {
	Response{
		Code: code,
		Data: map[string]any{},
		Msg:  code.String(),
	}.Json(c)
}

func FailWithError(err error, c *gin.Context) {
	data, msg := validate.ValidateError(err)
	FailWithData(data, msg, c)
}
