package middleware

import (
	"bytes"
	"io"

	"myblogx/internal/platform/log/writer"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// maxCaptureBytes 是请求体采集的硬上限：超过部分直接丢弃，绝不整个读进内存。
// 注意它大于 PrepareCapturedBody 的 16KB 输出上限——先限量读，再脱敏/截断。
const maxCaptureBytes = 1 << 20 // 1MB

// CaptureLog 在请求进入业务前采集请求体，并在响应结束后采集响应体。
//
// L.10 起不再采集 header：原始报文默认不入库（见 07 §0）。
// 请求体用 io.LimitReader 限量读取，再交给 PrepareCapturedBody 做脱敏 + ≤16KB 截断；
// 采集结果只会在**失败请求**时被写进审计日志。
func CaptureLog(mode writer.CaptureLogMode) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 采集请求体（如果开启）
		if mode.NeedRequestBody() && c.Request != nil && c.Request.Body != nil {
			limited := io.LimitReader(c.Request.Body, maxCaptureBytes+1)
			rawRequestBody, err := io.ReadAll(limited)
			if err != nil {
				logrus.Warnf("采集请求体失败: %v", err)
			} else {
				truncated := int64(len(rawRequestBody)) > maxCaptureBytes
				if truncated {
					rawRequestBody = rawRequestBody[:maxCaptureBytes]
				}
				writer.SetCapturedRequestBody(c, writer.PrepareCapturedBody(rawRequestBody, c.GetHeader("Content-Type")))
				// 重新封装请求体，不影响后续业务读取（业务拿到的是被 1MB 截断后的部分）。
				c.Request.Body = io.NopCloser(bytes.NewBuffer(rawRequestBody))
			}
		}

		// 包装响应Writer，用于采集响应体
		var responseWriter *auditBodyWriter
		if mode.NeedResponseBody() && c.Writer != nil {
			responseWriter = &auditBodyWriter{ResponseWriter: c.Writer, ctx: c, mode: mode}
			c.Writer = responseWriter
		}

		// 执行后续中间件/业务逻辑
		c.Next()

		// 响应完成后，强制同步一次采集数据
		if responseWriter != nil && mode.NeedResponseBody() {
			responseWriter.syncCapturedResponseBody()
		}
	}
}

const (
	// None 表示不采集任何请求/响应体。
	None = writer.None
	// ReqBody 表示采集请求体。
	ReqBody = writer.ReqBody
	// RespBody 表示采集响应体。
	RespBody = writer.RespBody
	// BothBody 表示同时采集请求体和响应体。
	BothBody = writer.BothBody
	// All 是 BothBody 的别名。
	All = writer.All
)

// auditBodyWriter 包装 Gin ResponseWriter，用于复制响应体内容供审计日志使用。
type auditBodyWriter struct {
	gin.ResponseWriter                       // 嵌入原始 ResponseWriter
	ctx                *gin.Context          // Gin 上下文
	mode               writer.CaptureLogMode // 采集模式
	body               bytes.Buffer          // 缓存响应体内容
}

// Write 在响应正常输出给客户端的同时，额外缓存一份响应体。
func (w *auditBodyWriter) Write(data []byte) (int, error) {
	if w.mode.NeedResponseBody() && len(data) > 0 {
		_, _ = w.body.Write(data)
	}
	written, err := w.ResponseWriter.Write(data)
	if w.mode.NeedResponseBody() {
		w.syncCapturedResponseBody()
	}
	return written, err
}

// WriteString 在响应输出字符串时，同步缓存一份字符串内容。
func (w *auditBodyWriter) WriteString(value string) (int, error) {
	if w.mode.NeedResponseBody() && value != "" {
		_, _ = w.body.WriteString(value)
	}
	written, err := w.ResponseWriter.WriteString(value)
	if w.mode.NeedResponseBody() {
		w.syncCapturedResponseBody()
	}
	return written, err
}

// syncCapturedResponseBody 将缓存的响应体同步到 Gin 上下文。
func (w *auditBodyWriter) syncCapturedResponseBody() {
	if w == nil || w.ctx == nil {
		return
	}
	writer.SetCapturedResponseBody(w.ctx, writer.PrepareCapturedBody(w.body.Bytes(), w.Header().Get("Content-Type")))
}
