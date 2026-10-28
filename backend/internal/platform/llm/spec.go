package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/apperr"
)

// 供应商标识。与 internal/ai/app/vendors.go 的元数据、后台下拉框一一对应。
const (
	VendorOpenAI     = "openai"
	VendorClaude     = "claude"
	VendorGemini     = "gemini"
	VendorArk        = "ark"
	VendorDeepSeek   = "deepseek"
	VendorQwen       = "qwen"
	VendorQianfan    = "qianfan"
	VendorOpenRouter = "openrouter"
)

// defaultBaseURL 是各家的默认接口地址，只在配置留空时兜底。
//
// 全部是"base"而不是完整 endpoint：eino 组件会自己拼 /chat/completions。
var defaultBaseURL = map[string]string{
	VendorOpenAI:     "https://api.openai.com/v1",
	VendorArk:        "https://ark.cn-beijing.volces.com/api/v3",
	VendorDeepSeek:   "https://api.deepseek.com/v1",
	VendorQwen:       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	VendorQianfan:    "https://qianfan.baidubce.com/v2",
	VendorOpenRouter: "https://openrouter.ai/api/v1",
}

// defaultClaudeMaxTokens 是 Claude 的兜底上限：它的 MaxTokens 是必填项，
// 不填组件会直接构造失败。
const defaultClaudeMaxTokens = 4096

// ProviderSpec 是构造一个模型客户端所需的全部信息。
//
// 为什么不直接用业务层的实体：platform 层不该依赖 internal/ai，
// 而且这里的字段要和 eino 组件的参数一一对上（BaseURL 语义、MaxTokens 指针）。
type ProviderSpec struct {
	Vendor      string
	Model       string
	APIKey      string
	BaseURL     string // 可留空，按 vendor 取默认值
	Region      string // ark 用
	AccessKey   string // ark 用 AK/SK 认证时
	SecretKey   string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration // <=0 用默认 60s
}

// vendor 归一化供应商标识；空值按 OpenAI 兼容处理（历史配置没有 vendor 字段）。
func (s ProviderSpec) vendor() string {
	v := strings.ToLower(strings.TrimSpace(s.Vendor))
	if v == "" {
		return VendorOpenAI
	}
	return v
}

// resolvedBaseURL 返回实际要用的 base 地址。
func (s ProviderSpec) resolvedBaseURL() string {
	if normalized := NormalizeBaseURL(s.BaseURL); normalized != "" {
		return normalized
	}
	return defaultBaseURL[s.vendor()]
}

// timeout 返回单次请求超时。
func (s ProviderSpec) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return defaultTimeout
}

// validate 校验构造客户端的最小必要信息。
//
// 只检查"没有它一定发不出去"的项；具体某个供应商还需要什么（endpoint id、
// region、max_tokens）由 vendors.go 的元数据在后台录入时校验，这里不重复。
func (s ProviderSpec) validate() error {
	vendor := s.vendor()
	if _, ok := supportedVendors[vendor]; !ok {
		return apperr.Wrap(apperr.CodeAIVendorUnsupported, fmt.Errorf("vendor=%s", vendor))
	}
	if strings.TrimSpace(s.Model) == "" {
		return apperr.New(apperr.CodeAIProviderInvalid)
	}
	hasCredential := s.APIKey != "" || (s.AccessKey != "" && s.SecretKey != "")
	if !hasCredential {
		return apperr.New(apperr.CodeAISecretMissing)
	}
	// ark 之外的供应商必须有 base 地址（含默认值）；ark 有默认值，不需要这里兜。
	if s.resolvedBaseURL() == "" {
		return apperr.New(apperr.CodeAIBaseURLMissing)
	}
	return nil
}

// cacheKey 是配置指纹：任一字段变了就应该重建客户端。
//
// 密钥只取哈希，避免把明文写进日志或 panic 栈里的 map key。
func (s ProviderSpec) cacheKey() string {
	secret := s.APIKey + "|" + s.AccessKey + "|" + s.SecretKey
	sum := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%s|%s|%s|%d|%.4f|%s|%s|%s",
		s.vendor(), s.Model, s.resolvedBaseURL(), s.MaxTokens, s.Temperature,
		s.Region, hex.EncodeToString(sum[:8]), s.timeout())
}

// supportedVendors 是 factory 认识的全部供应商。
var supportedVendors = map[string]struct{}{
	VendorOpenAI:     {},
	VendorClaude:     {},
	VendorGemini:     {},
	VendorArk:        {},
	VendorDeepSeek:   {},
	VendorQwen:       {},
	VendorQianfan:    {},
	VendorOpenRouter: {},
}

// SupportedVendors 返回支持的供应商标识列表（供后台校验与文档用）。
func SupportedVendors() []string {
	out := make([]string, 0, len(supportedVendors))
	for v := range supportedVendors {
		out = append(out, v)
	}
	return out
}

// NormalizeBaseURL 把"完整 endpoint"归一成 eino 组件要的 base 地址。
//
// 历史配置里填的是 https://host/v1/chat/completions，而组件会自己拼
// /chat/completions —— 不处理就会拼成 .../chat/completions/chat/completions，
// 表现为 404，而错误信息里看不出是这个原因。
func NormalizeBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	for _, suffix := range []string{"/chat/completions", "/completions"} {
		if strings.HasSuffix(trimmed, suffix) {
			trimmed = strings.TrimSuffix(trimmed, suffix)
			break
		}
	}
	return strings.TrimRight(trimmed, "/")
}
