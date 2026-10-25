// Package ws 是 WebSocket 的平台基建：连接升级、连接封装、在线连接池、心跳。
//
// 从 api/chat_api/chat_ws.go 与 service/chat_service/chat_online.go 抽出（S4.5b）。
// 业务模块只依赖这里的通用能力，不再各自维护一套 upgrader 与在线池。
package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultReadBuffer 读缓冲。
	DefaultReadBuffer = 1024
	// DefaultWriteBuffer 写缓冲。
	DefaultWriteBuffer = 1024
	// DefaultWriteWait 单次写超时。
	DefaultWriteWait = 10 * time.Second
)

// Upgrade 把 HTTP 请求升级成 WebSocket 连接。
//
// checkOrigin 由调用方传入：平台层不依赖具体的框架中间件，
// 这样以后换 Origin 策略不用改这里。
func Upgrade(w http.ResponseWriter, r *http.Request, checkOrigin func(*http.Request) bool) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  DefaultReadBuffer,
		WriteBufferSize: DefaultWriteBuffer,
	}
	if checkOrigin != nil {
		upgrader.CheckOrigin = checkOrigin
	}
	return upgrader.Upgrade(w, r, nil)
}

// Conn 是一条 WebSocket 连接。
//
// 写锁和连接绑定：心跳与业务推送可能并发写同一条连接，gorilla 不允许并发写。
type Conn struct {
	raw         *websocket.Conn
	UserID      uint64
	ConnectedAt time.Time
	writeMu     sync.Mutex
}

// NewConn 包装一条已升级的连接。
func NewConn(userID uint64, raw *websocket.Conn) *Conn {
	return &Conn{raw: raw, UserID: userID, ConnectedAt: time.Now()}
}

// SetReadLimit 设置单帧大小上限。
func (c *Conn) SetReadLimit(limit int64) { c.raw.SetReadLimit(limit) }

// SetReadDeadline 设置读超时。
func (c *Conn) SetReadDeadline(t time.Time) error { return c.raw.SetReadDeadline(t) }

// SetWriteDeadline 设置写超时。
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.raw.SetWriteDeadline(t) }

// SetPongHandler 设置 pong 回调（用于续期读超时）。
func (c *Conn) SetPongHandler(h func(string) error) { c.raw.SetPongHandler(h) }

// ReadMessage 读一条消息。
func (c *Conn) ReadMessage() (int, []byte, error) { return c.raw.ReadMessage() }

// WriteMessage 安全写一条消息。
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.raw.WriteMessage(messageType, data)
}

// WriteControl 安全写一条控制帧。
func (c *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.raw.WriteControl(messageType, data, deadline)
}

// WriteJSON 把结构体序列化后发出。
func (c *Conn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteMessage(websocket.TextMessage, data)
}

// WriteMessageTimeout 设置写超时后发一条消息。
func (c *Conn) WriteMessageTimeout(messageType int, data []byte, wait time.Duration) error {
	_ = c.SetWriteDeadline(time.Now().Add(wait))
	return c.WriteMessage(messageType, data)
}

// Close 关闭连接。
func (c *Conn) Close() error { return c.raw.Close() }

// RunPingLoop 定时发 ping，直到 done 关闭或写失败。
func (c *Conn) RunPingLoop(done <-chan struct{}, pingPeriod, writeWait time.Duration) error {
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

// Hub 维护用户在线连接（一个用户可有多条，例如多标签页/多设备）。
type Hub struct {
	mu    sync.RWMutex
	users map[uint64]map[*Conn]struct{}
}

// NewHub 构造在线连接池。
func NewHub() *Hub {
	return &Hub{users: make(map[uint64]map[*Conn]struct{})}
}

// Register 注册连接。
func (h *Hub) Register(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.users[conn.UserID]
	if !ok {
		set = make(map[*Conn]struct{})
		h.users[conn.UserID] = set
	}
	set[conn] = struct{}{}
}

// Unregister 注销连接。
func (h *Hub) Unregister(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.users[conn.UserID]
	if !ok {
		return
	}
	delete(set, conn)
	if len(set) == 0 {
		delete(h.users, conn.UserID)
	}
}

// IsOnline 判断用户是否有在线连接。
func (h *Hub) IsOnline(userID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID]) > 0
}

// Count 返回用户的连接数。
func (h *Hub) Count(userID uint64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID])
}

// Snapshot 复制用户的连接列表后立刻解锁。
//
// 不能持锁遍历并逐条写 socket：一条慢连接会把整个在线池堵住。
func (h *Hub) Snapshot(userID uint64) []*Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.users[userID]
	out := make([]*Conn, 0, len(set))
	for conn := range set {
		out = append(out, conn)
	}
	return out
}

// PushJSON 给某用户所有在线连接推 JSON；返回成功条数。
func (h *Hub) PushJSON(userID uint64, v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return h.Push(userID, websocket.TextMessage, data)
}

// Push 给某用户所有在线连接推消息；写失败的连接会被摘除。
func (h *Hub) Push(userID uint64, messageType int, data []byte) int {
	success := 0
	for _, conn := range h.Snapshot(userID) {
		_ = conn.SetWriteDeadline(time.Now().Add(DefaultWriteWait))
		if err := conn.WriteMessage(messageType, data); err != nil {
			h.Unregister(conn)
			_ = conn.Close()
			continue
		}
		success++
	}
	return success
}
