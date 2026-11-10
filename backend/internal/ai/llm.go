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
//
// 每次调用都会向日志管道上报一条用量记录（成功/失败都报）。两个早退分支
// （未开启、没有启用的模型配置）不报：它们没有真正打到上游，记进去只会
// 把"调用次数"这个指标污染成"用户点了多少次"。
func Chat(ctx context.Context, aiConf conf.AI, msgList []Message) (string, error) {
	if !aiConf.Enable {
		return "", apperr.New(apperr.CodeAIServiceOff)
	}
	spec, err := specFromConfig(aiConf)
	if err != nil {
		return "", err
	}
	// 每日额度：出站口是唯一的收费点，业务侧不需要各写一遍（ADR-0038）。
	lease, err := chargeCall(ctx, aiConf)
	if err != nil {
		// 被额度挡住同样要留一条调用日志：否则后台只能听用户说"我用不了"，
		// 看不出今天有多少人被挡、挡在哪个场景。用量为空，失败原因即错误码。
		reportCall(ctx, llm.CallUsage{}, err, false)
		return "", err
	}
	content, usage, err := llm.ChatDetailed(ctx, llm.SharedFactory(), spec, msgList)
	reportCall(ctx, usage, err, false)
	if err != nil {
		// 没有真正拿到结论的系统性失败不占额度（与投稿配额同一条口径）。
		releaseQuota(ctx, lease)
	}
	return content, err
}

// closedStream 造一对"只有一个错误"的通道。
//
// 返回的通道已经关闭：调用方的 for-range 会立刻结束，错误从 errChan 读一次。
func closedStream(err error) (chan string, chan error) {
	contentChan := make(chan string)
	errChan := make(chan error, 1)
	errChan <- err
	close(contentChan)
	close(errChan)
	return contentChan, errChan
}

// ChatStream 流式 AI 对话，返回内容通道与错误通道。
func ChatStream(ctx context.Context, aiConf conf.AI, msgList []Message) (chan string, chan error) {
	if !aiConf.Enable {
		return closedStream(apperr.New(apperr.CodeAIServiceOff))
	}
	spec, err := specFromConfig(aiConf)
	if err != nil {
		return closedStream(err)
	}
	lease, err := chargeCall(ctx, aiConf)
	if err != nil {
		reportCall(ctx, llm.CallUsage{}, err, true)
		return closedStream(err)
	}
	// 用量只能在流的写协程退出前拿到（收尾帧里），所以走回调而不是返回值。
	return llm.ChatStreamDetailed(ctx, llm.SharedFactory(), spec, msgList,
		func(usage llm.CallUsage, streamErr error) {
			reportCall(ctx, usage, streamErr, true)
			if streamErr != nil {
				// 流失败（包括写完一半断流）归还预占：这一条没有产出可用结论。
				releaseQuota(ctx, lease)
			}
		})
}

// atoiOrZero 解析可选的数字字段；解析不了就当作"没填"。
func atoiOrZero(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
