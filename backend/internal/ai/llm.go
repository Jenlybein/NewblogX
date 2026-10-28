package ai

import (
	"context"
	"strconv"
	"time"

	"myblogx/conf"
	"myblogx/internal/apperr"
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

// specFromConfig 把 conf.AI 里的 Active 投影成 llm 层的输入。
//
// 这里做的是"领域字段 → 供应商字段"的翻译：配置表里 Fields 是
// map[string]string（键由供应商元数据定义），llm 层则要强类型的字段。
func specFromConfig(aiConf conf.AI) (llm.ProviderSpec, error) {
	active := aiConf.Active
	if active == nil {
		return llm.ProviderSpec{}, apperr.New(apperr.CodeAIProviderMissing)
	}
	fields := active.Fields
	return llm.ProviderSpec{
		Vendor:      active.Vendor,
		Model:       active.Model,
		APIKey:      fields["api_key"],
		BaseURL:     fields["base_url"],
		Region:      fields["region"],
		AccessKey:   fields["access_key"],
		SecretKey:   fields["secret_key"],
		MaxTokens:   atoiOrZero(fields["max_tokens"]),
		Temperature: aiConf.Temperature,
		Timeout:     time.Duration(aiConf.TimeoutSec) * time.Second,
	}, nil
}

// Chat 非流式 AI 对话。
//
// ctx 由调用方一路传进来：客户端断开时上游请求要能一起取消，否则一次点开
// 又关掉的对话会让上游白跑满整个超时。
func Chat(ctx context.Context, aiConf conf.AI, msgList []Message) (string, error) {
	if !aiConf.Enable {
		return "", apperr.New(apperr.CodeAIServiceOff)
	}
	spec, err := specFromConfig(aiConf)
	if err != nil {
		return "", err
	}
	return llm.Chat(ctx, llm.SharedFactory(), spec, msgList)
}

// ChatStream 流式 AI 对话，返回内容通道与错误通道。
func ChatStream(ctx context.Context, aiConf conf.AI, msgList []Message) (chan string, chan error) {
	if !aiConf.Enable {
		contentChan := make(chan string)
		errChan := make(chan error, 1)
		errChan <- apperr.New(apperr.CodeAIServiceOff)
		close(contentChan)
		close(errChan)
		return contentChan, errChan
	}
	spec, err := specFromConfig(aiConf)
	if err != nil {
		contentChan := make(chan string)
		errChan := make(chan error, 1)
		errChan <- err
		close(contentChan)
		close(errChan)
		return contentChan, errChan
	}
	return llm.ChatStream(ctx, llm.SharedFactory(), spec, msgList)
}

// atoiOrZero 解析可选的数字字段；解析不了就当作"没填"。
func atoiOrZero(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
