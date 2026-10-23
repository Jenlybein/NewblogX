package res

import (
	"encoding/json"
	"myblogx/models/ctype"
	"myblogx/service/chat_service"
	"time"

	"github.com/gorilla/websocket"
)

func SendConnFailWithMsg(msg string, conn *chat_service.ChatConn, wait time.Duration) error {
	resp := Response{FailValidCode, nil, msg}
	byteData, _ := json.Marshal(resp)
	return conn.WriteMessageTimeout(websocket.TextMessage, byteData, wait)
}

func SendConnOkWithMsg(msg string, conn *chat_service.ChatConn, wait time.Duration) error {
	resp := Response{SuccessCode, nil, msg}
	byteData, _ := json.Marshal(resp)
	return conn.WriteMessageTimeout(websocket.TextMessage, byteData, wait)
}

func SendConnOkWithData(data any, conn *chat_service.ChatConn, wait time.Duration) error {
	resp := Response{SuccessCode, data, "成功"}
	byteData, _ := json.Marshal(resp)
	return conn.WriteMessageTimeout(websocket.TextMessage, byteData, wait)
}

func SendWsMsg(data any, store *chat_service.OnlineUserStore, receiverID ctype.ID) int {
	resp := Response{SuccessCode, data, "成功"}
	byteData, _ := json.Marshal(resp)
	return store.PushToUser(receiverID, websocket.TextMessage, byteData)
}
