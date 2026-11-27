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

// multiReadCloser 把"已读前缀 + 未读剩余"拼成一个可关闭的 Reader。
//
// 单独写类型而不是 io.NopCloser：必须保留原始 body 的 Close，
// 否则连接上的请求体不会被释放（Keep-Alive / 客户端提前断开时尤其明显）。
type multiReadCloser struct {
	io.Reader
	io.Closer
}

// CaptureLog 在请求进入业务前采集请求体，并在响应结束后采集响应体。
//
// L.10 起不再采集 header：原始报文默认不入库（见 docs/architecture/governance.md §4.2）。
// 请求体只读前 maxCaptureBytes+1 字节做**采集样本**，再交给 PrepareCapturedBody 做脱敏
// + ≤16KB 截断；采集结果只会在**失败请求**时被写进审计日志。
//
// # 请求体必须完整还给业务（2026-11-27）
//
// 旧实现把 `c.Request.Body` 换成"只含前 1MB 的 buffer"，并在注释里写明
// "业务拿到的是被 1MB 截断后的部分"。这条注释描述的就是缺陷本身：任何超过
// 1MB 的 JSON 请求体都会变成半截，`ShouldBindJSON` 拿到 `unexpected EOF`，
// 用户看到「JSON 格式不完整，请检查请求体」。
//
// 实际踩到的场景：后台「关键词过滤」保存词表把整张词表随请求体下发，
// 6 万条约 4MB —— 保存**必失败**，而且失败发生在业务之前，
// 库里一行都没写（词表本身一直没坏，所以更难判断）。
//
// 现在的做法：读样本后把"已读前缀 + 尚未读取的剩余部分"重新接成 Reader，
// 内存只多占 1MB，请求体对业务保持逐字节完整；采集侧仍然只看前 1MB。
func CaptureLog(mode writer.CaptureLogMode) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 采集请求体（如果开启）
		if mode.NeedRequestBody() && c.Request != nil && c.Request.Body != nil {
			originalBody := c.Request.Body
			prefix, err := io.ReadAll(io.LimitReader(originalBody, maxCaptureBytes+1))
			if err != nil {
				logrus.Warnf("采集请求体失败: %v", err)
				// 读失败时 body 已被部分消费，只能把前缀尽量接回去，让业务自己报体读取错误。
				c.Request.Body = &multiReadCloser{
					Reader: io.MultiReader(bytes.NewReader(prefix), originalBody),
					Closer: originalBody,
				}
			} else {
				captured := prefix
				if len(captured) > maxCaptureBytes {
					captured = captured[:maxCaptureBytes]
				}
				writer.SetCapturedRequestBody(c, writer.PrepareCapturedBody(captured, c.GetHeader("Content-Type")))
				// 关键：完整恢复。超过采集上限时，未读的剩余部分仍在 originalBody 里。
				c.Request.Body = &multiReadCloser{
					Reader: io.MultiReader(bytes.NewReader(prefix), originalBody),
					Closer: originalBody,
				}
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

