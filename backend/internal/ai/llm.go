package ai

import (
	"context"

	"myblogx/conf"
	"myblogx/internal/platform/llm"
)

// 这些类型以别名形式保留在 ai_service 下，调用方（ai_search / ai_scoring /
// ai_diagnose / ai_metainfo / ai_overwrite …）无需改动。
//
// ChatCompletion 与 StreamData 已经删掉：报文解析现在归 eino 的组件，
// 这两个类型原本只被自研的 SSE 解析器使用，业务侧从未引用。
type (
	Message = llm.Message
	Request = llm.Request
)

// newClient 根据配置构造 AI 客户端。
//
// 真正的 HTTP 细节（超时 / ctx / 重试 / 连接池）在 internal/platform/llm，
// 本包只负责把 conf.AI 映射成 llm.Config。
func newClient(aiConf conf.AI) *llm.Client {
	return llm.NewClient(llm.Config{
		Enable:    aiConf.Enable,
		BaseURL:   aiConf.BaseURL,
		SecretKey: aiConf.SecretKey,
		ChatModel: aiConf.ChatModel,
	})
}

// Chat 非流式 AI 对话。
func Chat(aiConf conf.AI, msgList []Message) (string, error) {
	return newClient(aiConf).Chat(context.Background(), msgList)
}

// ChatStream 流式 AI 对话，返回内容通道与错误通道。
func ChatStream(aiConf conf.AI, msgList []Message) (chan string, chan error) {
	return newClient(aiConf).ChatStream(context.Background(), msgList)
}
