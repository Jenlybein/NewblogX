// Package llm 是 AI 大模型调用的唯一出口。
//
// 它把供应商差异（协议、鉴权、参数命名）收拢到一处：上层只说"给这段消息、
// 拿回复"，不关心底下是 OpenAI、Claude 还是 Gemini。
//
// 实现基于 CloudWeGo eino：各供应商由 eino-ext 的组件适配，本包负责三件事——
// 把配置映射成组件参数（factory.go）、按配置指纹缓存客户端、把流式读取包装成
// channel 契约（chat.go）。
//
// 分层约定：
//   - 本包不 import 任何业务包（包括 internal/ai）：入参是 ProviderSpec。
//   - 业务包负责把领域实体映射成 ProviderSpec。
//   - 流式读取协程经 safego 启动（archcheck R7）。
package llm
