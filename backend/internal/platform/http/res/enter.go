// 响应封装

package res

import (
	"myblogx/internal/apperr"
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

// OkWithCursorList 返回带复合游标的消息列表 data 载荷。
//
// data 由调用方构造成具名 DTO（私信模块是 domain.MessageListData），
// 这样 swag 注解能直接引用类型，运行时 JSON 形状也只有一处定义。
// "没有更多"时调用方必须把 next_cursor 留成零值：后端不替它猜，
// 免得把"能继续翻页"的语义藏进响应封装里。
func OkWithCursorList(data any, c *gin.Context) {
	OkWithData(data, c)
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

// FailWithError 把错误翻成"服务异常"（1002）响应。
func FailWithError(err error, c *gin.Context) {
	data, msg := validate.ValidateError(err)
	FailWithData(data, msg, c)
}

// FailWithValidError 把错误翻成"参数校验失败"（1001）响应。
//
// 与 FailWithError 的唯一区别是包体业务码：错误值仍必须由 apperr 产生（文案与
// 错误码只有一处来源），但参数类失败在协议里的码是 1001 而不是 1002
// （"服务异常"会让调用方以为后端坏了）。两个 helper 并存，才让"错误值统一
// 收口到 apperr"与"参数错误用 1001"同时成立。
func FailWithValidError(err error, c *gin.Context) {
	data, msg := validate.ValidateError(err)
	Response{
		Code: FailValidCode,
		Data: data,
		Msg:  msg,
	}.Json(c)
}

// Fail 是共享响应层对"错误值"的唯一出口。
//
// 协议码由错误码的类别决定（分类只在 apperr 一处登记）；中文文案只从
// apperr 的消息表取；data 恒为空对象。未知错误（没有错误码）统一给通用
// 服务异常文案：它仍然是一次失败（不吞），但 err.Error() 里的实现细节、
// 连接串或包装前缀一律不进响应。调用方只需把错误值原样递进来。
func Fail(err error, c *gin.Context) {
	if err == nil {
		err = apperr.New(apperr.CodeCommonServiceFailed)
	}
	msg := apperr.Message(apperr.CodeCommonServiceFailed)
	if code, ok := apperr.CodeOf(err); ok {
		msg = apperr.Message(code)
	}
	data := map[string]any{}

	switch apperr.ProtocolOf(err) {
	case apperr.ProtocolValidation:
		Response{Code: FailValidCode, Data: data, Msg: msg}.Json(c)
	case apperr.ProtocolAuth:
		Response{Code: FailAuthCode, Data: data, Msg: msg}.Json(c)
	default:
		Response{Code: FailServiceCode, Data: data, Msg: msg}.Json(c)
	}
}
