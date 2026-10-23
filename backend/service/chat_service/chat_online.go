package chat_service

import (
	"encoding/json"
	"sync"
	"time"

	"myblogx/models/ctype"

	"github.com/gorilla/websocket"
)

const chatPushWriteWait = 10 * time.Second

// ChatConn 表示一条聊天 WebSocket 连接。
// 写锁和连接绑定，避免心跳和业务推送并发写同一条连接。
type ChatConn struct {
	Conn        *websocket.Conn
	UserID      ctype.ID
	ConnectedAt time.Time
	writeMu     sync.Mutex
}

func NewChatConn(userID ctype.ID, conn *websocket.Conn) *ChatConn {
	return &ChatConn{
		Conn:        conn,
		UserID:      userID,
		ConnectedAt: time.Now(),
	}
}

// 给写操作设超时，避免连接卡死时一直阻塞。
func (c *ChatConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

func (c *ChatConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *ChatConn) SetReadLimit(limit int64) {
	c.Conn.SetReadLimit(limit)
}

func (c *ChatConn) SetPongHandler(h func(string) error) {
	c.Conn.SetPongHandler(h)
}

func (c *ChatConn) ReadMessage() (messageType int, p []byte, err error) {
	return c.Conn.ReadMessage()
}

// 内部先加锁，安全写一条普通 ws 消息
func (c *ChatConn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.Conn.WriteMessage(messageType, data)
}

// 内部先加锁，安全写一条控制 ws 消息（用于心跳等）
func (c *ChatConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.Conn.WriteControl(messageType, data, deadline)
}

// 内部先设置超时，写一条消息。
func (c *ChatConn) WriteMessageTimeout(msgType int, msgContent []byte, wait time.Duration) error {
	_ = c.SetWriteDeadline(time.Now().Add(wait))
	return c.WriteMessage(msgType, []byte(msgContent))
}

// 把结构体转成 JSON 后发出去
func (c *ChatConn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteMessage(websocket.TextMessage, data)
}

func (c *ChatConn) Close() error {
	return c.Conn.Close()
}

// RunPingLoop 定时向客户端发送 ping 控制帧。
// 浏览器端会自动回 pong，配合 ws 层的 PongHandler 可以持续续期读超时。
func (c *ChatConn) RunPingLoop(done <-chan struct{}, pingPeriod, writeWait time.Duration) error {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			_ = c.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(writeWait)); err != nil {
				return err
			}
		}
	}
}

// OnlineUserStore 维护用户在线连接。
// 一个用户可能同时有多条连接，例如多个标签页或多个设备。
type OnlineUserStore struct {
	mu    sync.RWMutex
	users map[ctype.ID]map[*ChatConn]struct{}
}

func NewOnlineUserStore() *OnlineUserStore {
	return &OnlineUserStore{
		users: make(map[ctype.ID]map[*ChatConn]struct{}),
	}
}

func (s *OnlineUserStore) Register(conn *ChatConn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	connSet, ok := s.users[conn.UserID]
	if !ok {
		connSet = make(map[*ChatConn]struct{})
		s.users[conn.UserID] = connSet
	}
	connSet[conn] = struct{}{}
}

func (s *OnlineUserStore) Unregister(conn *ChatConn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	connSet, ok := s.users[conn.UserID]
	if !ok {
		return
	}
	delete(connSet, conn)
	if len(connSet) == 0 {
		delete(s.users, conn.UserID)
	}
}

func (s *OnlineUserStore) IsOnline(userID ctype.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users[userID]) > 0
}

func (s *OnlineUserStore) Count(userID ctype.ID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users[userID])
}

// Snapshot 返回某个用户当前连接列表的快照。
// 作用是避免“拿着全局锁去给每条连接发消息”，即避免锁住整个在线池，遍历用户所有连接，一条条写 socket。只要某条连接很慢，全局锁就会被拖住
// 正确做法是：先加读锁，把当前连接列表复制出来后立刻解锁，再慢慢给这些连接发消息
func (s *OnlineUserStore) Snapshot(userID ctype.ID) []*ChatConn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connSet := s.users[userID]
	list := make([]*ChatConn, 0, len(connSet))
	for conn := range connSet {
		list = append(list, conn)
	}
	return list
}

// 给某个用户的所有在线连接发一条消息。
func (s *OnlineUserStore) PushToUser(userID ctype.ID, messageType int, data []byte) (successCount int) {
	for _, conn := range s.Snapshot(userID) {
		_ = conn.SetWriteDeadline(time.Now().Add(chatPushWriteWait))
		if err := conn.WriteMessage(messageType, data); err != nil {
			s.Unregister(conn)
			_ = conn.Close()
			continue
		}
		successCount++
	}
	return successCount
}

func (s *OnlineUserStore) PushJSONToUser(userID ctype.ID, v any) (successCount int) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return s.PushToUser(userID, websocket.TextMessage, data)
}

var defaultOnlineUserStore = NewOnlineUserStore()

func GetOnlineUserStore() *OnlineUserStore {
	return defaultOnlineUserStore
}
