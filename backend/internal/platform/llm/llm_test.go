package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	openaiSDK "github.com/meguminnnnnnnnn/go-openai"
)

// fakeChatModel 是最小的 model.BaseChatModel 实现。
//
// 用它替代真实组件：既有单测都是靠 httptest 假服务器 + BaseURL 打真 HTTP，
// 换成框架后那样测等于在测 eino 的 HTTP 细节，与本包无关。
type fakeChatModel struct {
	reply     string
	chunks    []string
	genErr    error
	streamErr error
	genCalls  int
	// usage 非 nil 时，Generate 的回复与流的最后一帧都会带上它。
	// 流式那条特意做成"Content 为空、只挂 ResponseMeta" —— 这正是
	// openai / deepseek / ark 等组件在 EOF 时补发的收尾帧形状。
	usage *schema.TokenUsage
	// delay 模拟上游耗时。断言 Duration 时必须有它：纯内存的 fake 调用
	// 只要几百纳秒，Windows 上的单调时钟精度会把耗时舍成 0。
	delay time.Duration
}

func (f *fakeChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.genCalls++
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.genErr != nil {
		return nil, f.genErr
	}
	msg := &schema.Message{Role: schema.Assistant, Content: f.reply}
	if f.usage != nil {
		msg.ResponseMeta = &schema.ResponseMeta{Usage: f.usage}
	}
	return msg, nil
}

func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	chunks := make([]*schema.Message, 0, len(f.chunks)+1)
	for _, item := range f.chunks {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: item})
	}
	if f.usage != nil {
		chunks = append(chunks, &schema.Message{
			Role:         schema.Assistant,
			ResponseMeta: &schema.ResponseMeta{Usage: f.usage},
		})
	}
	return schema.StreamReaderFromArray(chunks), nil
}

func newFakeFactory(fake *fakeChatModel) *Factory {
	f := NewFactory()
	f.buildFn = func(context.Context, ProviderSpec) (model.BaseChatModel, error) { return fake, nil }
	return f
}

func validSpec() ProviderSpec {
	return ProviderSpec{
		Vendor:  VendorOpenAI,
		Model:   "test-model",
		APIKey:  "test-key",
		BaseURL: "https://example.com/v1",
		Timeout: 2 * time.Second,
	}
}

// TestChat_Success 验证正常响应能取出正文。
func TestChat_Success(t *testing.T) {
	fake := &fakeChatModel{reply: "你好"}
	got, err := Chat(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if got != "你好" {
		t.Fatalf("got %q, want 你好", got)
	}
}

// TestChat_EmptyReply 验证空回复被当成错误，而不是把空串交给上层。
func TestChat_EmptyReply(t *testing.T) {
	fake := &fakeChatModel{reply: "   "}
	_, err := Chat(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if !apperr.IsCode(err, apperr.CodeAIReplyEmpty) {
		t.Fatalf("err = %v, want CodeAIReplyEmpty", err)
	}
}

// TestChat_EmptyMessages 验证空消息列表直接报错，不构造客户端。
func TestChat_EmptyMessages(t *testing.T) {
	_, err := Chat(context.Background(), newFakeFactory(&fakeChatModel{reply: "x"}), validSpec(), nil)
	if !apperr.IsCode(err, apperr.CodeAIMessageEmpty) {
		t.Fatalf("err = %v, want CodeAIMessageEmpty", err)
	}
}

// TestChat_RetriesOn5xx 验证 5xx 会重试到上限：1 次首试 + 2 次重试 = 3 次。
func TestChat_RetriesOn5xx(t *testing.T) {
	fake := &fakeChatModel{genErr: &openaiSDK.APIError{HTTPStatusCode: http.StatusInternalServerError, Message: "boom"}}
	if _, err := Chat(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("期望报错")
	}
	if fake.genCalls != 3 {
		t.Fatalf("调用 %d 次，期望 3 次（1 + 2 次重试）", fake.genCalls)
	}
}

// TestChat_NoRetryOn4xx 验证 4xx 只请求一次。
func TestChat_NoRetryOn4xx(t *testing.T) {
	fake := &fakeChatModel{genErr: &openaiSDK.APIError{HTTPStatusCode: http.StatusUnauthorized, Message: "bad key"}}
	if _, err := Chat(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("期望报错")
	}
	if fake.genCalls != 1 {
		t.Fatalf("调用 %d 次，期望 1 次（4xx 不重试）", fake.genCalls)
	}
}

// TestChat_ContextCanceled 验证 ctx 取消时直接返回上下文错误、不重试。
func TestChat_ContextCanceled(t *testing.T) {
	fake := &fakeChatModel{genErr: context.Canceled}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Chat(ctx, newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if fake.genCalls != 1 {
		t.Fatalf("调用 %d 次，取消后不该重试", fake.genCalls)
	}
}

// TestChatStream 验证分片按序拼出完整内容，且 channel 正常关闭。
func TestChatStream(t *testing.T) {
	fake := &fakeChatModel{chunks: []string{"你", "好", ""}}
	content, errs := ChatStream(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})

	var sb strings.Builder
	for text := range content {
		sb.WriteString(text)
	}
	for err := range errs {
		t.Fatalf("stream 错误: %v", err)
	}
	if sb.String() != "你好" {
		t.Fatalf("got %q, want 你好", sb.String())
	}
}

// TestChatStream_Error 验证流式错误走错误通道。
func TestChatStream_Error(t *testing.T) {
	fake := &fakeChatModel{streamErr: &openaiSDK.APIError{HTTPStatusCode: http.StatusBadRequest, Message: "bad"}}
	content, errs := ChatStream(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	for range content {
	}
	var got error
	for err := range errs {
		got = err
	}
	if !apperr.IsCode(got, apperr.CodeAIStatusError) {
		t.Fatalf("err = %v, want CodeAIStatusError", got)
	}
}

// TestSpecValidate 覆盖构造前的最小必要校验。
func TestSpecValidate(t *testing.T) {
	cases := []struct {
		name string
		spec ProviderSpec
		code apperr.Code
	}{
		{"缺模型", ProviderSpec{Vendor: VendorOpenAI, APIKey: "k", BaseURL: "https://x/v1"}, apperr.CodeAIProviderInvalid},
		{"缺密钥", ProviderSpec{Vendor: VendorOpenAI, Model: "m", BaseURL: "https://x/v1"}, apperr.CodeAISecretMissing},
		{"未知供应商", ProviderSpec{Vendor: "nope", Model: "m", APIKey: "k"}, apperr.CodeAIVendorUnsupported},
		{"ark 用 AK/SK 也合法", ProviderSpec{Vendor: VendorArk, Model: "ep-1", AccessKey: "a", SecretKey: "b"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.validate()
			if tc.code == "" {
				if err != nil {
					t.Fatalf("期望通过，得到 %v", err)
				}
				return
			}
			if !apperr.IsCode(err, tc.code) {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}

// TestNormalizeBaseURL 验证完整 endpoint 能被归一成 base。
func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1/chat/completions": "https://api.openai.com/v1",
		"https://api.openai.com/v1/":                 "https://api.openai.com/v1",
		"https://x.example/completions":              "https://x.example",
		"  https://x.example/v1  ":                   "https://x.example/v1",
		"":                                           "",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFactory_Cache 验证同配置只构造一次，改字段后重新构造。
func TestFactory_Cache(t *testing.T) {
	f := NewFactory()
	calls := 0
	f.buildFn = func(context.Context, ProviderSpec) (model.BaseChatModel, error) {
		calls++
		return &fakeChatModel{reply: "ok"}, nil
	}

	spec := validSpec()
	for i := 0; i < 3; i++ {
		if _, err := f.ChatModel(context.Background(), spec); err != nil {
			t.Fatalf("构造失败: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("构造 %d 次，期望缓存命中只构造 1 次", calls)
	}

	changed := spec
	changed.Model = "other-model"
	if _, err := f.ChatModel(context.Background(), changed); err != nil {
		t.Fatalf("构造失败: %v", err)
	}
	if calls != 2 {
		t.Fatalf("改模型后构造 %d 次，期望 2 次", calls)
	}
}

// TestClient_NotEnabled 验证未开启时直接报错，不触达模型。
func TestClient_NotEnabled(t *testing.T) {
	c := NewClient(Config{Enable: false})
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}); !apperr.IsCode(err, apperr.CodeAIServiceOff) {
		t.Fatalf("err = %v, want CodeAIServiceOff", err)
	}
}

// TestChatDetailed_Usage 验证非流式能把用量与耗时带出来。
func TestChatDetailed_Usage(t *testing.T) {
	fake := &fakeChatModel{
		reply: "你好",
		delay: time.Millisecond,
		usage: &schema.TokenUsage{
			PromptTokens:     12,
			CompletionTokens: 7,
			TotalTokens:      19,
			PromptTokenDetails:     schema.PromptTokenDetails{CachedTokens: 3},
			CompletionTokensDetails: schema.CompletionTokensDetails{ReasoningTokens: 2},
		},
	}
	content, usage, err := ChatDetailed(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("ChatDetailed 失败: %v", err)
	}
	if content != "你好" {
		t.Fatalf("content = %q, want 你好", content)
	}
	if !usage.Available {
		t.Fatal("上游回了 usage，Available 应为 true")
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.TotalTokens != 19 {
		t.Fatalf("用量不对: %+v", usage)
	}
	if usage.CachedTokens != 3 || usage.ReasoningTokens != 2 {
		t.Fatalf("明细字段不对: %+v", usage)
	}
	if usage.Attempts != 1 {
		t.Fatalf("Attempts = %d, 期望 1", usage.Attempts)
	}
	if usage.Model != "test-model" || usage.Vendor != VendorOpenAI {
		t.Fatalf("模型标识不对: %+v", usage)
	}
	if usage.Duration <= 0 {
		t.Fatalf("Duration = %v, 期望正数", usage.Duration)
	}
}

// TestChatDetailed_TotalFallback 验证上游只回 prompt/completion 时用两者之和兜底。
func TestChatDetailed_TotalFallback(t *testing.T) {
	fake := &fakeChatModel{
		reply: "x",
		usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 6},
	}
	_, usage, err := ChatDetailed(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("ChatDetailed 失败: %v", err)
	}
	if got := usage.Total(); got != 11 {
		t.Fatalf("Total() = %d, 期望 11（上游没给 total，用 p+c 兜底）", got)
	}
}

// TestChatDetailed_EmptyReplyKeepsUsage 验证空回复也保留用量。
//
// 空回复（模型只吐空白）同样是真花了 token 的；若先判空再取用量，
// 这一次的开销会连同错误一起消失，后台就少统计一批。
func TestChatDetailed_EmptyReplyKeepsUsage(t *testing.T) {
	fake := &fakeChatModel{reply: "  ", usage: &schema.TokenUsage{PromptTokens: 4, TotalTokens: 4}}
	_, usage, err := ChatDetailed(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if !apperr.IsCode(err, apperr.CodeAIReplyEmpty) {
		t.Fatalf("err = %v, want CodeAIReplyEmpty", err)
	}
	if !usage.Available || usage.PromptTokens != 4 {
		t.Fatalf("空回复的用量丢了: %+v", usage)
	}
}

// TestChatDetailed_RetryCountedInAttempts 验证重试次数被记进 Attempts。
func TestChatDetailed_RetryCountedInAttempts(t *testing.T) {
	fake := &fakeChatModel{genErr: &openaiSDK.APIError{HTTPStatusCode: http.StatusInternalServerError, Message: "boom"}}
	_, usage, err := ChatDetailed(context.Background(), newFakeFactory(fake), validSpec(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("期望报错")
	}
	if usage.Attempts != 3 {
		t.Fatalf("Attempts = %d, 期望 3（1 次首试 + 2 次重试）", usage.Attempts)
	}
	if usage.Available {
		t.Fatal("上游报错时没有用量，Available 应为 false")
	}
}

// TestChatStreamDetailed_UsageFromTrailingChunk 验证流式能从"只挂 usage 的收尾帧"取到用量。
//
// 这是最关键的一条：OpenAI 系的收尾帧 Content 是空串，改造前正是被
// `chunk.Content == "" → continue` 跳过的。
func TestChatStreamDetailed_UsageFromTrailingChunk(t *testing.T) {
	fake := &fakeChatModel{
		chunks: []string{"你", "好"},
		usage:  &schema.TokenUsage{PromptTokens: 9, CompletionTokens: 3, TotalTokens: 12},
	}
	var done *CallUsage
	content, errs := ChatStreamDetailed(context.Background(), newFakeFactory(fake), validSpec(),
		[]Message{{Role: "user", Content: "hi"}},
		func(u CallUsage, err error) { captured := u; done = &captured })

	var sb strings.Builder
	for text := range content {
		sb.WriteString(text)
	}
	for err := range errs {
		t.Fatalf("stream 错误: %v", err)
	}
	if sb.String() != "你好" {
		t.Fatalf("content = %q, want 你好", sb.String())
	}
	if done == nil {
		t.Fatal("onDone 没有被调用")
	}
	if !done.Available || done.PromptTokens != 9 || done.TotalTokens != 12 {
		t.Fatalf("流式用量不对: %+v", *done)
	}
	if done.Attempts != 1 {
		t.Fatalf("Attempts = %d, 期望 1", done.Attempts)
	}
}

// TestChatStreamDetailed_UsageMissing 验证供应商不回 usage 时 Available=false。
//
// 自建 OpenAI 兼容网关（vLLM / one-api / ollama）普遍忽略 stream_options，
// 永远不回用量。这时必须能和"真的用了 0 token"区分开。
func TestChatStreamDetailed_UsageMissing(t *testing.T) {
	fake := &fakeChatModel{chunks: []string{"ok"}}
	var done *CallUsage
	content, errs := ChatStreamDetailed(context.Background(), newFakeFactory(fake), validSpec(),
		[]Message{{Role: "user", Content: "hi"}},
		func(u CallUsage, err error) { captured := u; done = &captured })
	for range content {
	}
	for err := range errs {
		t.Fatalf("stream 错误: %v", err)
	}
	if done == nil {
		t.Fatal("onDone 没有被调用")
	}
	if done.Available {
		t.Fatal("上游没回 usage，Available 应为 false")
	}
	if done.Total() != 0 {
		t.Fatalf("Total() = %d, 期望 0", done.Total())
	}
}

// TestChatStreamDetailed_ErrorReportedToCallback 验证流式失败时回调能拿到错误。
func TestChatStreamDetailed_ErrorReportedToCallback(t *testing.T) {
	fake := &fakeChatModel{streamErr: &openaiSDK.APIError{HTTPStatusCode: http.StatusBadRequest, Message: "bad"}}
	var callbackErr error
	called := false
	content, errs := ChatStreamDetailed(context.Background(), newFakeFactory(fake), validSpec(),
		[]Message{{Role: "user", Content: "hi"}},
		func(u CallUsage, err error) { called = true; callbackErr = err })
	for range content {
	}
	for range errs {
	}
	if !called {
		t.Fatal("onDone 没有被调用")
	}
	if !apperr.IsCode(callbackErr, apperr.CodeAIStatusError) {
		t.Fatalf("回调拿到的错误 = %v, want CodeAIStatusError", callbackErr)
	}
}
