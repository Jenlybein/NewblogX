package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/safego"

	"github.com/cloudwego/eino/schema"
	openaiSDK "github.com/meguminnnnnnnnn/go-openai"
)

const defaultMaxRetries = 2

// sharedFactory 是进程级默认工厂。
//
// 这是本包唯一一处包级状态，理由是连接池天然属于进程：改造前每次调用都
// newClient → 新建 Transport，MaxIdleConnsPerHost 等于白配。
var (
	sharedOnce    sync.Once
	sharedFactory *Factory
)

// SharedFactory 返回进程级工厂（共享连接池）。
func SharedFactory() *Factory {
	sharedOnce.Do(func() { sharedFactory = NewFactory() })
	return sharedFactory
}

// Chat 用给定配置发一次非流式对话，返回回复正文。
func Chat(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message) (string, error) {
	if len(msgList) == 0 {
		return "", apperr.New(apperr.CodeAIMessageEmpty)
	}
	chatModel, err := factory.ChatModel(ctx, spec)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		reply, err := chatModel.Generate(ctx, toSchemaMessages(msgList))
		if err == nil {
			if reply == nil || strings.TrimSpace(reply.Content) == "" {
				return "", apperr.New(apperr.CodeAIReplyEmpty)
			}
			return reply.Content, nil
		}
		lastErr = err

		// 调用方取消 / 超时：不重试，直接返回上下文错误。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if !retryable(err) || attempt == defaultMaxRetries {
			return "", wrapModelError(err)
		}
		if waitErr := sleepBackoff(ctx, attempt); waitErr != nil {
			return "", waitErr
		}
	}
	return "", wrapModelError(lastErr)
}

// ChatStream 发送流式对话，返回内容通道与错误通道。
//
// channel 契约与改造前一致（内容通道关闭即结束），所以上层 SSE 转发不用改。
// 读取协程经 safego 启动：R7 规则因此不会把它算作裸 goroutine。
func ChatStream(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message) (chan string, chan error) {
	contentChan := make(chan string)
	errChan := make(chan error, 1) // 带缓冲，防止 goroutine 阻塞

	safego.Go(nil, func() {
		defer close(contentChan)
		defer close(errChan)

		if len(msgList) == 0 {
			errChan <- apperr.New(apperr.CodeAIMessageEmpty)
			return
		}

		chatModel, err := factory.ChatModel(ctx, spec)
		if err != nil {
			errChan <- err
			return
		}

		stream, err := chatModel.Stream(ctx, toSchemaMessages(msgList))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				errChan <- ctxErr
				return
			}
			errChan <- wrapModelError(err)
			return
		}
		defer stream.Close()

		for {
			chunk, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				return
			}
			if recvErr != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					errChan <- ctxErr
					return
				}
				errChan <- wrapModelError(recvErr)
				return
			}
			if chunk == nil || chunk.Content == "" {
				continue
			}
			select {
			case contentChan <- chunk.Content:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	})

	return contentChan, errChan
}

// toSchemaMessages 把本包的消息类型转成 eino 的消息类型。
//
// 角色不认识时按 user 处理而不是报错：模型配置是管理员填的，
// 上游拼错了角色名不该让整次调用失败。
func toSchemaMessages(msgList []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(msgList))
	for _, item := range msgList {
		role := schema.RoleType(strings.ToLower(strings.TrimSpace(item.Role)))
		switch role {
		case schema.System, schema.User, schema.Assistant, schema.Tool:
		default:
			role = schema.User
		}
		out = append(out, &schema.Message{Role: role, Content: item.Content})
	}
	return out
}

// retryable 判断这个错误值不值得重试。
//
// 判据只有两条：网络层错误、以及 429/5xx。4xx 一律不重试 —— 把 401 重试三遍
// 只会让用户多等两轮，而且日志里看起来像"上游不稳定"。
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *openaiSDK.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatusCode == http.StatusTooManyRequests || apiErr.HTTPStatusCode >= 500
	}

	// 连接被拒、DNS 失败、握手超时等：重试有意义。
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return true
	}
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "no such host")
}

// wrapModelError 把 eino / 供应商 SDK 的错误翻成统一的错误码。
//
// 保留原始错误作为 cause：日志里要能看到上游到底说了什么。
func wrapModelError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openaiSDK.APIError
	if errors.As(err, &apiErr) {
		return apperr.Wrap(apperr.CodeAIStatusError,
			fmt.Errorf("status=%d message=%s", apiErr.HTTPStatusCode, apiErr.Message))
	}
	return apperr.Wrap(apperr.CodeAISendFailed, err)
}

// sleepBackoff 按 200ms * 2^attempt 退避，可被 ctx 取消。
func sleepBackoff(ctx context.Context, attempt int) error {
	d := time.Duration(200*(1<<attempt)) * time.Millisecond
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Config 是构造旧式客户端的参数。
//
// Deprecated: 模型配置改成"列表 + 启用一条"之后，调用方直接用 ProviderSpec。
// 保留它是为了让"换框架"这一步不牵动上层，等 conf.AI 瘦身时一起删。
type Config struct {
	Enable     bool
	BaseURL    string
	SecretKey  string
	ChatModel  string
	Timeout    time.Duration // 单次请求总超时，<=0 用默认 60s
	MaxRetries int           // 已废弃：重试次数现在由包内统一决定
}

// Client 是兼容旧调用方的薄壳，内部走 Factory + ProviderSpec。
type Client struct {
	spec   ProviderSpec
	enable bool
}

// NewClient 构造客户端。连接池由 SharedFactory 共享，不再每次新建。
func NewClient(cfg Config) *Client {
	return &Client{
		enable: cfg.Enable,
		spec: ProviderSpec{
			Vendor:    VendorOpenAI,
			Model:     cfg.ChatModel,
			APIKey:    cfg.SecretKey,
			BaseURL:   cfg.BaseURL,
			Timeout:   cfg.Timeout,
			MaxTokens: 0,
		},
	}
}

// Chat 发送一次非流式对话并返回回复正文。
func (c *Client) Chat(ctx context.Context, msgList []Message) (string, error) {
	if !c.enable {
		return "", apperr.New(apperr.CodeAIServiceOff)
	}
	return Chat(ctx, SharedFactory(), c.spec, msgList)
}

// ChatStream 发送流式对话，返回内容通道与错误通道。
func (c *Client) ChatStream(ctx context.Context, msgList []Message) (chan string, chan error) {
	if !c.enable {
		contentChan := make(chan string)
		errChan := make(chan error, 1)
		errChan <- apperr.New(apperr.CodeAIServiceOff)
		close(contentChan)
		close(errChan)
		return contentChan, errChan
	}
	return ChatStream(ctx, SharedFactory(), c.spec, msgList)
}
