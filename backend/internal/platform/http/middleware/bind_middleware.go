package middleware

import (
	"myblogx/common/res"
	"myblogx/utils/validate"

	"github.com/gin-gonic/gin"
)

func Bind[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBind(&cr)
	if err != nil {
		failBind(c, err, "")
		c.Abort()
		return
	}
	c.Set("request", cr)
}

func BindJson[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBindJSON(&cr)
	if err != nil {
		failBind(c, err, "json")
		c.Abort()
		return
	}
	c.Set("requestJson", cr)
}

func BindQuery[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBindQuery(&cr)
	if err != nil {
		failBind(c, err, "query")
		c.Abort()
		return
	}
	c.Set("requestQuery", cr)
}

func BindUri[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBindUri(&cr)
	if err != nil {
		failBind(c, err, "uri")
		c.Abort()
		return
	}
	c.Set("requestUri", cr)
}

func failBind(c *gin.Context, err error, bindType string) {
	data, msg := validate.ValidateError(err)
	if data == nil {
		data = map[string]any{}
	}

	switch bindType {
	case "json":
		if msg == "请求体不能为空" {
			msg = "请求体不能为空，请在 Body 中传 JSON 参数"
			data["body"] = msg
		}
	case "query":
		if msg == "" {
			msg = "查询参数错误"
		}
	case "uri":
		if msg == "" {
			msg = "路径参数错误"
		}
	}

	res.FailWithData(data, msg, c)
}

func GetBind[T any](c *gin.Context) T {
	return c.MustGet("request").(T)
}

func GetBindJson[T any](c *gin.Context) T {
	return c.MustGet("requestJson").(T)
}

func GetBindQuery[T any](c *gin.Context) T {
	return c.MustGet("requestQuery").(T)
}

func GetBindUri[T any](c *gin.Context) T {
	return c.MustGet("requestUri").(T)
}
