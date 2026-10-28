package app

import (
	"fmt"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/platform/llm"
)

// VendorField 是某个供应商需要的单个配置项。
//
// 前端按它动态渲染表单：新增一个供应商只改这里，不用动前端代码。
type VendorField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"` // 前端用密码框，回显时遮蔽
	Placeholder string `json:"placeholder"`
	Default     string `json:"default"`
}

// VendorSpec 是一个供应商的元数据。
type VendorSpec struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Note   string        `json:"note"`
	Fields []VendorField `json:"fields"`
}

// ── 字段构造器：八个供应商的字段高度重合，抽出来避免同一条文案写八遍 ──

func keyField(placeholder string) VendorField {
	return VendorField{
		Key:         "api_key",
		Label:       "API Key",
		Required:    true,
		Secret:      true,
		Placeholder: placeholder,
	}
}

func baseURLField(def string) VendorField {
	return VendorField{
		Key:         "base_url",
		Label:       "接口地址",
		Placeholder: def,
		Default:     def,
	}
}

func modelField(placeholder string) VendorField {
	return VendorField{
		Key:         "model",
		Label:       "模型",
		Required:    true,
		Placeholder: placeholder,
	}
}

func maxTokensField() VendorField {
	return VendorField{
		Key:         "max_tokens",
		Label:       "最大输出 tokens（留空用供应商默认）",
		Placeholder: "2048",
	}
}

// vendorSpecs 是全部供应商的元数据。
//
// note 是给管理员看的取舍说明，会直接显示在表单顶部 —— 这些差异（endpoint id、
// 必填 max_tokens、只能走兼容协议）不说清楚，填错只会得到一句无关的 4xx。
var vendorSpecs = []VendorSpec{
	{
		ID:   llm.VendorOpenAI,
		Name: "OpenAI",
		Note: "官方接口；任何 OpenAI 兼容网关（自建代理、硅基流动等）也选这个，改接口地址即可",
		Fields: []VendorField{
			keyField("sk-..."),
			baseURLField("https://api.openai.com/v1"),
			modelField("gpt-4.1-mini"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorClaude,
		Name: "Anthropic Claude",
		Note: "走 Anthropic Messages API；最大输出 tokens 在本家是必填项，留空按 4096 处理",
		Fields: []VendorField{
			keyField("sk-ant-..."),
			baseURLField("（留空用官方地址）"),
			modelField("claude-sonnet-4-5"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorGemini,
		Name: "Google Gemini",
		Note: "走 Google GenAI SDK；接口地址只在需要走代理时填写",
		Fields: []VendorField{
			keyField("AIza..."),
			baseURLField("（留空用官方地址）"),
			modelField("gemini-2.5-flash"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorArk,
		Name: "火山引擎 方舟",
		Note: "模型一栏填「推理接入点 ID」（ep-xxxx），不是模型名；认证可用 API Key，也可留空后用 AK/SK",
		Fields: []VendorField{
			keyField("（可用 AK/SK 代替）"),
			{Key: "access_key", Label: "AccessKey（API Key 留空时填）", Secret: true},
			{Key: "secret_key", Label: "SecretKey（API Key 留空时填）", Secret: true},
			baseURLField("https://ark.cn-beijing.volces.com/api/v3"),
			modelField("ep-2024xxxx"),
			{Key: "region", Label: "地域", Placeholder: "cn-beijing", Default: "cn-beijing"},
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorDeepSeek,
		Name: "DeepSeek",
		Note: "官方接口，OpenAI 兼容",
		Fields: []VendorField{
			keyField("sk-..."),
			baseURLField("https://api.deepseek.com/v1"),
			modelField("deepseek-chat"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorQwen,
		Name: "通义千问（DashScope）",
		Note: "走阿里云 DashScope 的兼容模式接口",
		Fields: []VendorField{
			keyField("sk-..."),
			baseURLField("https://dashscope.aliyuncs.com/compatible-mode/v1"),
			modelField("qwen-plus"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorQianfan,
		Name: "百度千帆",
		Note: "走千帆 v2 的 OpenAI 兼容接口（填 API Key）。没有用千帆专用 SDK：它的 AK/SK 是进程级单例，多条配置会互相覆盖",
		Fields: []VendorField{
			keyField("bce-v3/..."),
			baseURLField("https://qianfan.baidubce.com/v2"),
			modelField("ernie-4.5-turbo-128k"),
			maxTokensField(),
		},
	},
	{
		ID:   llm.VendorOpenRouter,
		Name: "OpenRouter",
		Note: "聚合网关，模型名用 provider/model 形式",
		Fields: []VendorField{
			keyField("sk-or-..."),
			baseURLField("https://openrouter.ai/api/v1"),
			modelField("openai/gpt-4.1-mini"),
			maxTokensField(),
		},
	},
}

// Vendors 返回全部供应商元数据（后台下拉与动态表单用）。
func Vendors() []VendorSpec {
	out := make([]VendorSpec, len(vendorSpecs))
	copy(out, vendorSpecs)
	return out
}

// VendorByID 按标识查元数据。
func VendorByID(id string) (VendorSpec, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(id))
	for _, spec := range vendorSpecs {
		if spec.ID == trimmed {
			return spec, true
		}
	}
	return VendorSpec{}, false
}

// Validate 校验字段是否满足该供应商的必填要求。
//
// model 单独校验：它有独立的列（AIProvider.Model），不在 Fields 里，
// 前端虽然把它渲染成表单项，但提交时放在顶层。
//
// 文案里只带字段 key，不带中文说明：archcheck R8 禁止 errors.New 写中文，
// 这里统一用 apperr 的固定文案 + 字段名，前端会把 key 映射成 label 展示。
func (s VendorSpec) Validate(fields map[string]string) error {
	for _, field := range s.Fields {
		if !field.Required || field.Key == "model" {
			continue
		}
		value := strings.TrimSpace(fields[field.Key])
		if value == "" || value == sensitivePlaceholder {
			return apperr.Wrap(apperr.CodeAIProviderInvalid, fmt.Errorf("missing field %s", field.Key))
		}
	}
	return nil
}

// WithDefaults 把留空且带默认值的字段补上默认值，返回新 map（不改调用方入参）。
//
// 保存时补齐而不是每次调用时兜底：后台列表里看到的地址就是实际会用的地址，
// 免得"界面上空着、实际走的是默认值"这种对不上的情况。
func (s VendorSpec) WithDefaults(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields)+len(s.Fields))
	for key, value := range fields {
		out[key] = value
	}
	for _, field := range s.Fields {
		if strings.TrimSpace(out[field.Key]) == "" && field.Default != "" {
			out[field.Key] = field.Default
		}
	}
	return out
}

// SecretKeys 返回该供应商所有敏感字段的键名。
func (s VendorSpec) SecretKeys() []string {
	out := make([]string, 0, 2)
	for _, field := range s.Fields {
		if field.Secret {
			out = append(out, field.Key)
		}
	}
	return out
}
