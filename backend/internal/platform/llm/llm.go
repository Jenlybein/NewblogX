// Package llm 是 AI 大模型调用的唯一出口。
//
// 它把 HTTP 细节（超时、上下文、连接池、重试）收拢到一处，
// 业务层只关心“发消息、拿回复”，不再自己 new http.Client。
//
// 设计要点：
//  1. 所有请求走 http.NewRequestWithContext，调用方可取消/设截止时间。
//  2. 固定超时的 http.Client + 连接池，不再每次请求新建连接。
//  3. 网络错误 / 429 / 5xx 按指数退避重试；4xx 直接返回，不重试。
//  4. 流式读取协程经 safego 启动，避免裸 goroutine 的 panic 风险（R7）。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"myblogx/internal/apperr"
	"net"
	"net/http"
	"strings"
	"time"

	"myblogx/internal/platform/safego"
)

// Message 是一条对话消息。
type Message struct {
	Role    string `json:"role"`    // 角色：system/user/assistant
	Content string `json:"content"` // 消息内容
}

// Request 是发给兼容 OpenAI 协议的请求体。
type Request struct {
	Model    string    `json:"model"`    // 模型名称
	Messages []Message `json:"messages"` // 对话消息列表
	Stream   bool      `json:"stream"`   // 是否开启流式响应
}

// ChatCompletion 是非流式响应。
type ChatCompletion struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Logprobs     any    `json:"logprobs"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Object  string `json:"object"`
	Usage   struct {
		CompletionTokens        int `json:"completion_tokens"`
		PromptTokens            int `json:"prompt_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		PromptTokensDetails struct {
			AudioTokens  int `json:"audio_tokens"`
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	SystemFingerprint any `json:"system_fingerprint"`
}

// StreamData 是流式响应中的单个分片。
type StreamData struct {
	ID      string `json:"id"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Logprobs     any `json:"logprobs"`
		FinishReason any `json:"finish_reason"`
	} `json:"choices"`
	Created           int    `json:"created"`
	Model             string `json:"model"`
	Object            string `json:"object"`
	SystemFingerprint any    `json:"system_fingerprint"`
}

// Config 是构造客户端的参数。
type Config struct {
	Enable     bool
	BaseURL    string
	SecretKey  string
	ChatModel  string
	Timeout    time.Duration // 单次请求总超时，<=0 用默认 60s
	MaxRetries int           // 重试次数，0 用默认 2；<0 表示不重试
}

// Client 是并发安全的 AI 客户端；同一进程内可复用。
type Client struct {
	httpClient *http.Client
	enable     bool
	baseURL    string
	apiKey     string
	model      string
	maxRetries int
}

const (
	defaultTimeout    = 60 * time.Second
	defaultMaxRetries = 2
)

// NewClient 构造客户端。http.Client 带超时与连接池，避免每次请求新建连接。
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	retries := cfg.MaxRetries
	switch {
	case cfg.MaxRetries < 0:
		retries = 0
	case cfg.MaxRetries == 0:
		retries = defaultMaxRetries
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

	return &Client{
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
		enable:     cfg.Enable,
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.SecretKey,
		model:      cfg.ChatModel,
		maxRetries: retries,
	}
}

// validate 校验基础配置。
func (c *Client) validate() error {
	if !c.enable {
		return apperr.New(apperr.CodeAIServiceOff)
	}
	if c.apiKey == "" {
		return apperr.New(apperr.CodeAISecretMissing)
	}
	if c.baseURL == "" {
		return apperr.New(apperr.CodeAIBaseURLMissing)
	}
	return nil
}

// do 发送请求，命中网络错误 / 429 / 5xx 时按指数退避重试。
func (c *Client) do(ctx context.Context, payload Request) (*http.Response, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeAIEncodeFailed, err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeAIRequestBuild, err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		res, err := c.httpClient.Do(req)
		if err != nil {
			// 调用方主动取消 / 超时，不重试，直接返回上下文错误。
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = apperr.Wrap(apperr.CodeAISendFailed, err)
			if attempt < c.maxRetries {
				if werr := sleepBackoff(ctx, attempt); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, lastErr
		}

		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			raw, _ := io.ReadAll(res.Body)
			res.Body.Close()
			msg := strings.TrimSpace(string(raw))
			if msg == "" {
				lastErr = apperr.Wrap(apperr.CodeAIStatusError, fmt.Errorf("status=%d", res.StatusCode))
			} else {
				lastErr = apperr.Wrap(apperr.CodeAIStatusError, fmt.Errorf("status=%d body=%s", res.StatusCode, msg))
			}
			if attempt < c.maxRetries {
				if werr := sleepBackoff(ctx, attempt); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, lastErr
		}

		return res, nil
	}

	return nil, lastErr
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

// Chat 发送一次非流式对话并返回回复正文。
func (c *Client) Chat(ctx context.Context, msgList []Message) (string, error) {
	if len(msgList) == 0 {
		return "", apperr.New(apperr.CodeAIMessageEmpty)
	}

	res, err := c.do(ctx, Request{Model: c.model, Messages: msgList, Stream: false})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			return "", apperr.Wrap(apperr.CodeAIStatusError, fmt.Errorf("status=%d", res.StatusCode))
		}
		return "", apperr.Wrap(apperr.CodeAIStatusError, fmt.Errorf("status=%d body=%s", res.StatusCode, msg))
	}

	var response ChatCompletion
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", apperr.Wrap(apperr.CodeAIJSONInvalid, err)
	}
	if len(response.Choices) == 0 {
		return "", apperr.New(apperr.CodeAIChoicesEmpty)
	}
	reply := response.Choices[0].Message.Content
	if reply == "" {
		return "", apperr.New(apperr.CodeAIReplyEmpty)
	}
	return reply, nil
}

// ChatStream 发送流式对话，返回内容通道与错误通道。
//
// 后台读取协程经 safego 启动：R7 规则因此不会把它算作裸 goroutine。
func (c *Client) ChatStream(ctx context.Context, msgList []Message) (chan string, chan error) {
	contentChan := make(chan string)
	errChan := make(chan error, 1) // 带缓冲，防止 goroutine 阻塞

	safego.Go(nil, func() {
		defer close(contentChan)
		defer close(errChan)

		if len(msgList) == 0 {
			errChan <- apperr.New(apperr.CodeAIMessageEmpty)
			return
		}

		res, err := c.do(ctx, Request{Model: c.model, Messages: msgList, Stream: true})
		if err != nil {
			errChan <- err
			return
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			errChan <- fmt.Errorf("流式请求失败，响应状态码: %d", res.StatusCode)
			return
		}

		scanner := bufio.NewScanner(res.Body)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			text := scanner.Text()
			if text == "" || !strings.HasPrefix(text, "data: ") {
				continue
			}
			data := strings.TrimPrefix(text, "data: ")
			if data == "[DONE]" {
				break
			}
			var item StreamData
			if err := json.Unmarshal([]byte(data), &item); err != nil {
				continue // 单个分片解析失败，继续下一条
			}
			if len(item.Choices) > 0 {
				select {
				case contentChan <- item.Choices[0].Delta.Content:
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("读取响应流失败: %w", err)
		}
	})

	return contentChan, errChan
}
