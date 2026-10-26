package res

import (
	"encoding/json"
	"time"

	"myblogx/internal/platform/http/ws"

	"github.com/gorilla/websocket"
)

// SendConnFailWithMsg 给单条连接回失败。
func SendConnFailWithMsg(msg string, conn *ws.Conn, wait time.Duration) error {
	resp := Response{FailValidCode, nil, msg}
	byteData, _ := json.Marshal(resp)
	return conn.WriteMessageTimeout(websocket.TextMessage, byteData, wait)
}

// SendConnOkWithMsg 给单条连接回成功消息。
func SendConnOkWithMsg(msg string, conn *ws.Conn, wait time.Duration) error {
	resp := Response{SuccessCode, nil, msg}
	byteData, _ := json.Marshal(resp)
	return conn.WriteMessageTimeout(websocket.TextMessage, byteData, wait)
}
