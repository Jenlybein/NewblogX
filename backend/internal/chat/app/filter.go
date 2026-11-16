package app

import (
	"context"
	"strings"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 私信正文的内容过滤（票 397 / ADR-0032 决策 3）。
//
// 私信是最容易被当成"绕过通道"的面：正文不公开渲染，但会直接推到对方屏幕上。
// 过滤发生在**落库之前**，所以 HTTP 落库、WS 通知、会话摘要三处读到的都是
// 替换后的同一份文本 —— 只要有一处按原文读，私信就还是那个绕过通道。
//
// 图片消息不过滤：它的"正文"是 {"kind":"image","image_url":...} 的 JSON，
// 命中替换会改写 URL 让图片失效。图片内容另有图片审核（ADR-0032 决策 3）。
//
// 命中记录的内容 ID 是空的：私信主键在写适配器里生成（幂等键派生），
// 而 #393 的处置动作只覆盖文章 / 评论 / 合集 / 用户资料，私信不做"删除内容"，
// 因此不需要回指。风险页按文本面筛选仍然能看到这些记录。

// FilterText 是一段要在落库前过滤的私信正文。
type FilterText struct {
	// UserID 是发送人：命中记录按它统计（风险页的"命中用户 Top"）。
	UserID ctype.ID
	Text   string
}

// FilteredText 是过滤结果。
type FilteredText struct {
	// Masked 是替换后的文本（与输入等长，按 rune 计）。
	Masked string
	// FilteredCount 是实际被替换的处数。
	FilteredCount int
}

// ChatTextFilterer 在全站唯一的内容过滤层里替换私信正文并记一条关键词风险。
type ChatTextFilterer interface {
	FilterChatText(ctx context.Context, in FilterText) (FilteredText, error)
}

// filterChatText 过滤一条私信正文；失败方向 fail-open（ADR-0032 决策 11）。
//
// 未装配或报错时返回原文：私信发不出去比漏过滤更糟，审计层的问题不该变成可用性故障。
func (s *Service) filterChatText(ctx context.Context, senderID ctype.ID, msgType domain.MsgType, content string) string {
	if s.contentFilter == nil {
		return content
	}
	// 图片消息的正文是 JSON，替换会打坏 image_url，因此不送进过滤器。
	if msgType != domain.TypeText && msgType != domain.TypeMarkdown {
		return content
	}
	if strings.TrimSpace(content) == "" {
		return content
	}
	got, err := s.contentFilter.FilterChatText(ctx, FilterText{
		UserID: senderID,
		Text:   content,
	})
	if err != nil {
		if s.logger != nil { s.logger.Errorf("过滤私信正文失败，按原文发送: sender=%s err=%v", senderID, err) }
		return content
	}
	if got.Masked == "" {
		return content
	}
	return got.Masked
}



