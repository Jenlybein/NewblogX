package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"myblogx/internal/apperr"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

const defaultTimeout = 60 * time.Second

// Factory 按供应商构造 eino 的 ChatModel，并按配置指纹缓存。
//
// 为什么必须缓存：eino 的组件构造会建 http.Client 与内部状态，每次调用都
// 重建等于连接池失效（改造前的实现就是这样：每次 newClient 都新建 Transport，
// 旧的只能等 GC）。缓存键是配置指纹，改动任一字段自动换新实例。
type Factory struct {
	mu      sync.RWMutex
	models  map[string]model.BaseChatModel
	clients map[time.Duration]*http.Client

	// buildFn 只在测试里注入：用它换掉真实的供应商组件，
	// 让单测不必打网络，也不必为每家供应商准备假服务器。
	buildFn func(context.Context, ProviderSpec) (model.BaseChatModel, error)
}

// NewFactory 构造工厂。它是并发安全的，进程内建一个即可。
func NewFactory() *Factory {
	return &Factory{
		models:  make(map[string]model.BaseChatModel),
		clients: make(map[time.Duration]*http.Client),
	}
}

// ChatModel 返回（并缓存）一份模型客户端。
func (f *Factory) ChatModel(ctx context.Context, spec ProviderSpec) (model.BaseChatModel, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	key := spec.cacheKey()
	f.mu.RLock()
	cached, ok := f.models[key]
	f.mu.RUnlock()
	if ok {
		return cached, nil
	}

	build := f.build
	if f.buildFn != nil {
		build = f.buildFn // 测试注入
	}
	built, err := build(ctx, spec)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.models[key] = built
	f.mu.Unlock()
	return built, nil
}

// Invalidate 清空模型缓存。
//
// 调用时机：后台增删改/启用配置之后。不清也不会出错（指纹变了自然重建），
// 但会留下已删除配置的实例，白白占着连接。
func (f *Factory) Invalidate() {
	f.mu.Lock()
	f.models = make(map[string]model.BaseChatModel)
	f.mu.Unlock()
}

// httpClient 按超时返回共享的 *http.Client。
//
// 连接池只该建一次：MaxIdleConnsPerHost 的意义就是复用，每次新建等于白配。
// 不同超时值各留一个（配置里超时是站点级的，实际只会有一两个）。
func (f *Factory) httpClient(timeout time.Duration) *http.Client {
	f.mu.RLock()
	existing, ok := f.clients[timeout]
	f.mu.RUnlock()
	if ok {
		return existing
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	f.mu.Lock()
	f.clients[timeout] = client
	f.mu.Unlock()
	return client
}

// build 按供应商分发到具体的 eino 组件。
func (f *Factory) build(ctx context.Context, spec ProviderSpec) (model.BaseChatModel, error) {
	vendor := spec.vendor()
	baseURL := spec.resolvedBaseURL()
	httpClient := f.httpClient(spec.timeout())

	switch vendor {
	case VendorOpenAI, VendorQianfan, VendorOpenRouter:
		// 千帆与 OpenRouter 都提供 OpenAI 兼容协议，用同一个组件即可。
		// 特别是千帆：它的专用组件把 AK/SK 存在进程级单例里
		// （qianfan.GetQianfanSingletonConfig），多条配置会互相覆盖 ——
		// 在这套"配置列表"里那是致命的，所以走兼容协议。
		cfg := &openai.ChatModelConfig{
			APIKey:     spec.APIKey,
			Model:      spec.Model,
			BaseURL:    baseURL,
			HTTPClient: httpClient,
		}
		if spec.MaxTokens > 0 {
			cfg.MaxTokens = intPtr(spec.MaxTokens)
		}
		if spec.Temperature > 0 {
			cfg.Temperature = float32Ptr(float32(spec.Temperature))
		}
		return openai.NewChatModel(ctx, cfg)

	case VendorDeepSeek:
		return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			APIKey:      spec.APIKey,
			Model:       spec.Model,
			BaseURL:     baseURL,
			MaxTokens:   spec.MaxTokens,
			Temperature: float32(spec.Temperature),
			HTTPClient:  httpClient,
		})

	case VendorQwen:
		cfg := &qwen.ChatModelConfig{
			APIKey:     spec.APIKey,
			Model:      spec.Model,
			BaseURL:    baseURL,
			HTTPClient: httpClient,
		}
		if spec.MaxTokens > 0 {
			cfg.MaxTokens = intPtr(spec.MaxTokens)
		}
		if spec.Temperature > 0 {
			cfg.Temperature = float32Ptr(float32(spec.Temperature))
		}
		return qwen.NewChatModel(ctx, cfg)

	case VendorClaude:
		maxTokens := spec.MaxTokens
		if maxTokens <= 0 {
			maxTokens = defaultClaudeMaxTokens
		}
		cfg := &claude.Config{
			APIKey:     spec.APIKey,
			Model:      spec.Model,
			MaxTokens:  maxTokens,
			HTTPClient: httpClient,
		}
		if baseURL != "" {
			cfg.BaseURL = &baseURL
		}
		if spec.Temperature > 0 {
			cfg.Temperature = float32Ptr(float32(spec.Temperature))
		}
		return claude.NewChatModel(ctx, cfg)

	case VendorGemini:
		// Gemini 是唯一需要先建 SDK 客户端的组件：它的 Config 收的是 *genai.Client。
		clientConfig := &genai.ClientConfig{APIKey: spec.APIKey}
		if baseURL != "" {
			clientConfig.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL}
		}
		sdkClient, err := genai.NewClient(ctx, clientConfig)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeAISendFailed, err)
		}
		cfg := &gemini.Config{Client: sdkClient, Model: spec.Model}
		if spec.MaxTokens > 0 {
			cfg.MaxTokens = intPtr(spec.MaxTokens)
		}
		if spec.Temperature > 0 {
			cfg.Temperature = float32Ptr(float32(spec.Temperature))
		}
		return gemini.NewChatModel(ctx, cfg)

	case VendorArk:
		cfg := &ark.ChatModelConfig{
			APIKey:     spec.APIKey,
			Model:      spec.Model, // 火山方舟这里填的是推理接入点 ID
			BaseURL:    baseURL,
			Region:     spec.Region,
			HTTPClient: httpClient,
		}
		// 方舟支持 APIKey 与 AK/SK 两种认证，前者优先；两者都给时组件自己会用 APIKey。
		if spec.APIKey == "" {
			cfg.AccessKey = spec.AccessKey
			cfg.SecretKey = spec.SecretKey
		}
		if spec.MaxTokens > 0 {
			cfg.MaxTokens = intPtr(spec.MaxTokens)
		}
		if spec.Temperature > 0 {
			cfg.Temperature = float32Ptr(float32(spec.Temperature))
		}
		return ark.NewChatModel(ctx, cfg)

	default:
		return nil, apperr.Wrap(apperr.CodeAIVendorUnsupported, fmt.Errorf("vendor=%s", vendor))
	}
}

func intPtr(v int) *int { return &v }

func float32Ptr(v float32) *float32 { return &v }
