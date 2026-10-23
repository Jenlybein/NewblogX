package chat_service

import (
	"errors"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/chat_msg_enum"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ToTextChatRequest 用于创建纯文本消息。
// 文本消息直接把文本存进 Content，便于后续搜索和预览。
type ToTextChatRequest struct {
	SenderID   ctype.ID
	ReceiverID ctype.ID
	Text       string
	SendTime   time.Time
}

// ToTextChat 创建文本消息。
func ToTextChat(db *gorm.DB, logger *logrus.Logger, req ToTextChatRequest) (*models.ChatMsgModel, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, errors.New("消息内容不能为空")
	}

	// 文本消息不做额外包装，直接存文本，保证查询和预览最简单。
	return ToChat(db, logger, ToChatRequest{
		SenderID:   req.SenderID,
		ReceiverID: req.ReceiverID,
		MsgType:    chat_msg_enum.MsgTypeText,
		Content:    text,
		SendTime:   req.SendTime,
	})
}

// ToImageChat 创建图片消息
// 图片消息暂时以 JSON 字符串存入 Content，兼容现有单字段模型。
type ToImageChatRequest struct {
	SenderID    ctype.ID
	ReceiverID  ctype.ID
	ImageURL    string // 必填，原图或主展示图地址
	PreviewURL  string // 可选，缩略图地址
	FileName    string // 可选，原始文件名
	MimeType    string // 可选，例如 image/png
	Width       int    // 可选，图片宽度
	Height      int    // 可选，图片高度
	Size        int64  // 可选，文件字节数
	Alt         string // 可选，图片替代文本
	OriginalURL string // 可选，和 ImageURL 分离时可存原始资源地址
	SendTime    time.Time
}

// imageChatContent 是图片消息在 Content 字段中的最终 JSON 结构。
// 这里不直接暴露给包外，避免调用方绕过 ToImageChat 的校验逻辑。
type imageChatContent struct {
	Kind        string `json:"kind"`
	ImageURL    string `json:"image_url"`
	PreviewURL  string `json:"preview_url,omitempty"`
	OriginalURL string `json:"original_url,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Alt         string `json:"alt,omitempty"`
}

func ToImageChat(db *gorm.DB, logger *logrus.Logger, req ToImageChatRequest) (*models.ChatMsgModel, error) {
	req.ImageURL = strings.TrimSpace(req.ImageURL)
	if req.ImageURL == "" {
		return nil, errors.New("图片消息缺少图片地址")
	}

	// 图片消息统一序列化成 JSON，避免后续再去猜 Content 字段语义。
	content, err := marshalChatContent(logger, imageChatContent{
		Kind:        "image",
		ImageURL:    req.ImageURL,
		PreviewURL:  strings.TrimSpace(req.PreviewURL),
		OriginalURL: strings.TrimSpace(req.OriginalURL),
		FileName:    strings.TrimSpace(req.FileName),
		MimeType:    strings.TrimSpace(req.MimeType),
		Width:       req.Width,
		Height:      req.Height,
		Size:        req.Size,
		Alt:         strings.TrimSpace(req.Alt),
	})
	if err != nil {
		return nil, err
	}

	return ToChat(db, logger, ToChatRequest{
		SenderID:   req.SenderID,
		ReceiverID: req.ReceiverID,
		MsgType:    chat_msg_enum.MsgTypeImage,
		Content:    content,
		SendTime:   req.SendTime,
	})
}

// ToMarkdownChatRequest 用于创建 Markdown 消息。
// Markdown 原文与摘要一起序列化，方便后续列表页直接复用摘要。
type ToMarkdownChatRequest struct {
	SenderID   ctype.ID
	ReceiverID ctype.ID
	Title      string
	Markdown   string
	Summary    string
	SendTime   time.Time
}

// markdownChatContent 是 Markdown 消息在 Content 字段中的最终 JSON 结构。
// 列表场景优先使用 Summary，详情场景再读取 Markdown 原文。
type markdownChatContent struct {
	Kind     string `json:"kind"`
	Title    string `json:"title,omitempty"`
	Markdown string `json:"markdown"`
	Summary  string `json:"summary,omitempty"`
}

// ToMarkdownChat 创建 Markdown 消息。
func ToMarkdownChat(db *gorm.DB, logger *logrus.Logger, req ToMarkdownChatRequest) (*models.ChatMsgModel, error) {
	text := strings.TrimSpace(req.Markdown)
	if text == "" {
		return nil, errors.New("消息内容不能为空")
	}

	// TODO：Markdown处理
	return ToChat(db, logger, ToChatRequest{
		SenderID:   req.SenderID,
		ReceiverID: req.ReceiverID,
		MsgType:    chat_msg_enum.MsgTypeMarkdown,
		Content:    text,
		SendTime:   req.SendTime,
	})
}

// 暂时不考虑 Audio 消息，先保留统一签名，避免后续再改调用层。
func ToAudioChat(db *gorm.DB, logger *logrus.Logger, req ToChatRequest) (*models.ChatMsgModel, error) {
	req.MsgType = chat_msg_enum.MsgTypeAudio
	return ToChat(db, logger, req)
}

// 暂时不考虑 Video 消息，先保留统一签名，避免后续再改调用层。
func ToVideoChat(db *gorm.DB, logger *logrus.Logger, req ToChatRequest) (*models.ChatMsgModel, error) {
	req.MsgType = chat_msg_enum.MsgTypeVideo
	return ToChat(db, logger, req)
}

// 暂时不考虑 File 消息，先保留统一签名，避免后续再改调用层。
func ToFileChat(db *gorm.DB, logger *logrus.Logger, req ToChatRequest) (*models.ChatMsgModel, error) {
	req.MsgType = chat_msg_enum.MsgTypeFile
	return ToChat(db, logger, req)
}

// 暂时不考虑 Emoji 消息，先保留统一签名，避免后续再改调用层。
func ToEmojiChat(db *gorm.DB, logger *logrus.Logger, req ToChatRequest) (*models.ChatMsgModel, error) {
	req.MsgType = chat_msg_enum.MsgTypeEmoji
	return ToChat(db, logger, req)
}
