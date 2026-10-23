package chat_service

import (
	"encoding/json"
	"errors"
	"myblogx/models/enum/chat_msg_enum"
	"myblogx/utils/markdown"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	defaultMarkdownDigest = 60
)

// validateChatBase 基础校验
func validateChatBase(req *ToChatRequest) error {
	// 用户校验
	if req.SenderID == 0 || req.ReceiverID == 0 {
		return errors.New("聊天双方不能为空")
	}

	// if req.SenderID == req.ReceiverID {
	// 	return errors.New("不支持给自己发私信")
	// }

	if strings.TrimSpace(req.Content) == "" {
		return errors.New("消息内容不能为空")
	}

	// 发送时间校验，避免未填
	if req.SendTime.IsZero() {
		req.SendTime = time.Now()
	}

	// 消息状态默认为已发送
	if req.MsgStatus == 0 {
		req.MsgStatus = chat_msg_enum.MsgStatusSend
	}

	return nil
}

// marshalChatContent 负责把复杂消息体统一转成 JSON 字符串。
// 当前模型只有一个 Content 字段，这里是图片/Markdown 的适配层。
func marshalChatContent(logger *logrus.Logger, v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		if logger != nil {
			logger.Errorf("序列化聊天消息内容失败: %v", err)
		}
		return "", err
	}
	return string(b), nil
}

// buildMarkdownSummary 从 Markdown 原文中提取简短摘要。
// 这里只做轻量裁剪，不做 Markdown 语法清洗，后续如果要更精准可独立替换。
func buildMarkdownSummary(markdown string, limit int) string {
	text := strings.Join(strings.Fields(markdown), " ")
	if limit <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// 生成会话列表里的最后一条消息摘要。
func buildSessionLastMsg(msgType chat_msg_enum.MsgType, content string) string {
	// 文本直接显示文本，复杂消息降级成简短文案或摘要
	var abstract string
	switch msgType {
	case chat_msg_enum.MsgTypeText:
		abstract = content
	case chat_msg_enum.MsgTypeImage:
		abstract = "[图片]"
	case chat_msg_enum.MsgTypeMarkdown:
		abstract = markdown.MdToText(content)
	case chat_msg_enum.MsgTypeAudio:
		abstract = "[语音]"
	case chat_msg_enum.MsgTypeVideo:
		abstract = "[视频]"
	case chat_msg_enum.MsgTypeFile:
		abstract = "[文件]"
	case chat_msg_enum.MsgTypeEmoji:
		abstract = "[表情]"
	default:
		abstract = "[消息]"
	}

	// 控制字数在 20 字以内
	if len(abstract) > 20 {
		return string([]rune(abstract)[:20]) + "..."
	}

	return abstract
}
