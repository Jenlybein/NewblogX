package chat_msg_enum

type MsgType int8

// 1-文本 2-图片 3-语音 4-视频 5-文件 6-表情 7-Markdown 8-已读通知
const (
	MsgTypeText MsgType = iota + 1
	MsgTypeImage
	MsgTypeAudio
	MsgTypeVideo
	MsgTypeFile
	MsgTypeEmoji
	MsgTypeMarkdown
	MsgTypeRead
)

type MsgStatus int8

// 1-已发送 2-已送达 3-已读 4-已撤回 5-已删除
const (
	MsgStatusSend MsgStatus = iota + 1
	MsgStatusDelivered
	MsgStatusRead
	MsgStatusRecall
	MsgStatusDelete
)
