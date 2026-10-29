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
//
// 需要 token 用量的调用方用 ChatDetailed；这里保留原签名是为了不牵动
// 上层与既有单测（llm_test.go 的 11 个用例）。
func Chat(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message) (string, error) {
	content, _, err := ChatDetailed(ctx, factory, spec, msgList)
	return content, err
}

// ChatDetailed 在 Chat 之上把用量与耗时一起带出。
//
// 用命名返回值 + defer 算 Duration：这函数有 5 个 return 点，逐个填迟早漏
// 一个，漏了就是"耗时恒为 0"的脏数据，而且不会报错。
func ChatDetailed(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message) (content string, usage CallUsage, err error) {
	usage.Model = spec.Model
	usage.Vendor = spec.vendor()
	started := time.Now()
	defer func() { usage.Duration = time.Since(started) }()

	if len(msgList) == 0 {
		return "", usage, apperr.New(apperr.CodeAIMessageEmpty)
	}
	chatModel, buildErr := factory.ChatModel(ctx, spec)
	if buildErr != nil {
		return "", usage, buildErr
	}

	var lastErr error
	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		usage.Attempts++
		reply, genErr := chatModel.Generate(ctx, toSchemaMessages(msgList))
		if genErr == nil {
			// 先取用量再判空正文：空回复也是真花了 token 的，
			// 先判空会把这一次的用量连同错误一起丢掉。
			usage.mergeMessage(reply)
			if reply == nil || strings.TrimSpace(reply.Content) == "" {
				return "", usage, apperr.New(apperr.CodeAIReplyEmpty)
			}
			return reply.Content, usage, nil
		}
		lastErr = genErr

		// 调用方取消 / 超时：不重试，直接返回上下文错误。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", usage, ctxErr
		}
		if !retryable(genErr) || attempt == defaultMaxRetries {
			return "", usage, wrapModelError(genErr)
		}
		if waitErr := sleepBackoff(ctx, attempt); waitErr != nil {
			return "", usage, waitErr
		}
	}
	return "", usage, wrapModelError(lastErr)
}

// ChatStream 发送流式对话，返回内容通道与错误通道。
//
// channel 契约与改造前一致（内容通道关闭即结束），所以上层 SSE 转发不用改。
// 需要 token 用量的调用方用 ChatStreamDetailed。
func ChatStream(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message) (chan string, chan error) {
	return ChatStreamDetailed(ctx, factory, spec, msgList, nil)
}

// ChatStreamDetailed 与 ChatStream 行为一致，只在写协程退出前回调一次用量。
//
// 为什么用回调而不是多开一条 usage channel：channel 多一条就多一条"必须有人读"
// 的契约，没人读时发送方要么阻塞泄漏、要么得加缓冲 + 非阻塞发送而丢数据；
// 想稳妥还得再起一个读协程去 drain。回调只跑一次、在写协程退出路径上，
// 天然不存在泄漏，也不需要调用方改 channel 消费逻辑。
//
// onDone 必须 O(1) 且不阻塞：它会被日志发射器直接调用（入队即返回）。
func ChatStreamDetailed(ctx context.Context, factory *Factory, spec ProviderSpec, msgList []Message, onDone func(CallUsage, error)) (chan string, chan error) {
	contentChan := make(chan string)
	errChan := make(chan error, 1) // 带缓冲，防止 goroutine 阻塞

	usage := CallUsage{Model: spec.Model, Vendor: spec.vendor()}
	started := time.Now()

	// 读取协程经 safego 启动：R7 规则因此不会把它算作裸 goroutine。
	safego.Go(nil, func() {
		// finalErr 是这次流式调用的终态，回调据此判定成功/失败。
		var finalErr error
		// defer 是 LIFO，这里注册顺序与执行顺序相反，最终执行序为：
		//   回调 → close(errChan) → close(contentChan)
		//
		// 回调必须在关通道**之前**：调用方（drainStream / 单测）是用
		// `for range contentChan` 判断结束的，一旦读到通道关闭就去看用量，
		// 若回调排在后面，那时用量还是零值 —— 这是只有 -race 才稳定暴露的竞态。
		//
		// 反过来，回调 panic 也不会让下游卡死：defer 链在 panic 展开时仍会
		// 继续执行，close 照常发生，最后由 safego 收掉 panic。
		defer close(contentChan)
		defer close(errChan)
		defer func() {
			usage.Duration = time.Since(started)
			if onDone != nil {
				onDone(usage, finalErr)
			}
		}()

		// fail 记下终态并上报。errChan 容量 1 且每条路径最多 fail 一次，不会阻塞。
		fail := func(e error) {
			finalErr = e
			errChan <- e
		}

		if len(msgList) == 0 {
			fail(apperr.New(apperr.CodeAIMessageEmpty))
			return
		}

		chatModel, err := factory.ChatModel(ctx, spec)
		if err != nil {
			fail(err)
			return
		}

		stream, err := chatModel.Stream(ctx, toSchemaMessages(msgList))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				fail(ctxErr)
				return
			}
			fail(wrapModelError(err))
			return
		}
		defer stream.Close()

		usage.Attempts = 1

		for {
			chunk, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				return
			}
			if recvErr != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					fail(ctxErr)
					return
				}
				fail(wrapModelError(recvErr))
				return
			}
			if chunk == nil {
				continue
			}
			// 必须先取用量再判空正文：OpenAI 系（含 deepseek / ark / qwen 等
			// 走同一组件的供应商）的收尾帧就是"Content 为空、只挂
			// ResponseMeta.Usage"，先判空正好把它扔掉 —— 改造前就是这么丢的。
			usage.mergeMessage(chunk)
			if chunk.Content == "" {
				continue
			}
			select {
			case contentChan <- chunk.Content:
			case <-ctx.Done():
				fail(ctx.Err())
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
