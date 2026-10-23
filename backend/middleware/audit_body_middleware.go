package middleware

import (
	"bytes"
	"io"

	"myblogx/service/log_service"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CaptureLog 在请求进入业务前缓存原始请求信息，并在响应结束后缓存原始响应信息。
// 核心中间件：根据指定模式采集请求/响应数据，用于审计日志/问题排查
// 参数：mode - 采集模式（是否采集头/体）
func CaptureLog(mode log_service.CaptureLogMode) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 采集请求头（如果开启）
		if c.Request != nil && mode.NeedRequestHeader() {
			log_service.SetRawRequestHeader(c, log_service.PrepareCapturedHeaders(c.Request.Header.Clone()))
		}

		// 采集请求体（如果开启）
		if mode.NeedRequestBody() && c.Request != nil && c.Request.Body != nil {
			// 读取原始请求体
			rawRequestBody, err := io.ReadAll(c.Request.Body)
			if err != nil {
				logrus.Warnf("采集原始请求体失败: %v", err)
			} else {
				// 将请求体存入上下文
				log_service.SetRawRequestBody(c, log_service.PrepareCapturedBody(rawRequestBody, c.GetHeader("Content-Type")))
				// 重新封装请求体，不影响后续业务读取
				c.Request.Body = io.NopCloser(bytes.NewBuffer(rawRequestBody))
			}
		}

		// 包装响应Writer，用于采集响应数据
		var responseWriter *auditBodyWriter
		if (mode.NeedResponseBody() || mode.NeedResponseHeader()) && c.Writer != nil {
			// 包装响应 Writer
			responseWriter = &auditBodyWriter{ResponseWriter: c.Writer, ctx: c, mode: mode}
			// 替换响应 Writer
			c.Writer = responseWriter
		}

		// 执行后续中间件/业务逻辑
		c.Next()

		// 响应完成后，强制同步一次采集数据
		if responseWriter != nil {
			// 同步采集到的响应体到上下文
			if mode.NeedResponseBody() {
				responseWriter.syncCapturedResponseBody()
			}

			// 同步采集到的响应头到上下文
			if mode.NeedResponseHeader() {
				responseWriter.syncCapturedResponseHeader()
			}
		}
	}
}

const (
	// None 表示不采集任何原始请求/响应信息。
	None = log_service.None
	// ReqBody 表示采集原始请求体。
	ReqBody = log_service.ReqBody
	// RespBody 表示采集原始响应体。
	RespBody = log_service.RespBody
	// ReqHeader 表示采集原始请求头。
	ReqHeader = log_service.ReqHeader
	// RespHeader 表示采集原始响应头。
	RespHeader = log_service.RespHeader
	// BothBody 表示同时采集请求体和响应体。
	BothBody = log_service.BothBody
	// BothHeader 表示同时采集请求头和响应头。
	BothHeader = log_service.BothHeader
	// All 表示同时采集 body 与 header。
	All = log_service.All
)

// auditBodyWriter 包装 Gin ResponseWriter，用于复制响应体内容供审计日志使用。
// 实现 gin.ResponseWriter 接口，在不影响正常响应的前提下，缓存一份响应数据
type auditBodyWriter struct {
	gin.ResponseWriter                            // 嵌入原始 ResponseWriter
	ctx                *gin.Context               // Gin 上下文
	mode               log_service.CaptureLogMode // 采集模式
	body               bytes.Buffer               // 缓存响应体内容
}

// Write 在响应正常输出给客户端的同时，额外缓存一份响应体。
// 重写 Write 方法，实现响应体的透明采集
func (w *auditBodyWriter) Write(data []byte) (int, error) {
	// 根据采集模式判断是否需要缓存响应体
	if w.mode.NeedResponseBody() && len(data) > 0 {
		_, _ = w.body.Write(data)
	}
	// 调用原始 Write 方法，保证正常响应
	written, err := w.ResponseWriter.Write(data)

	// 同步采集到的响应体到上下文
	if w.mode.NeedResponseBody() {
		w.syncCapturedResponseBody()
	}
	// 同步采集到的响应头到上下文
	if w.mode.NeedResponseHeader() {
		w.syncCapturedResponseHeader()
	}
	return written, err
}

// WriteString 在响应输出字符串时，同步缓存一份字符串内容。
// 重写字符串写入方法，兼容字符串类型的响应输出
func (w *auditBodyWriter) WriteString(value string) (int, error) {
	// 根据采集模式判断是否需要缓存响应体
	if w.mode.NeedResponseBody() && value != "" {
		_, _ = w.body.WriteString(value)
	}
	// 调用原始 WriteString 方法
	written, err := w.ResponseWriter.WriteString(value)

	// 同步采集到的响应体到上下文
	if w.mode.NeedResponseBody() {
		w.syncCapturedResponseBody()
	}
	// 同步采集到的响应头到上下文
	if w.mode.NeedResponseHeader() {
		w.syncCapturedResponseHeader()
	}
	return written, err
}

// syncCapturedResponseBody 将缓存的响应体同步到 Gin 上下文
// 供后续日志中间件读取并记录到审计日志
func (w *auditBodyWriter) syncCapturedResponseBody() {
	if w == nil || w.ctx == nil {
		return
	}
	// 预处理响应体并设置到上下文
	log_service.SetRawResponseBody(w.ctx, log_service.PrepareCapturedBody(w.body.Bytes(), w.Header().Get("Content-Type")))
}

// syncCapturedResponseHeader 将响应头同步到 Gin 上下文
// 供后续日志中间件读取并记录到审计日志
func (w *auditBodyWriter) syncCapturedResponseHeader() {
	if w == nil || w.ctx == nil {
		return
	}
	// 克隆响应头并设置到上下文
	log_service.SetRawResponseHeader(w.ctx, log_service.PrepareCapturedHeaders(w.Header().Clone()))
}
