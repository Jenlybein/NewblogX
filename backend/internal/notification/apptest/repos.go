// Package apptest 提供通知模块的内存替身，供用例测试与路由表测试使用。
package apptest

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/app"
	"myblogx/internal/notification/domain"
)

// Messages 是站内信的内存实现。
type Messages struct {
	mu   sync.Mutex
	rows []domain.Message
	next uint64
}

var _ domain.MessageRepository = (*Messages)(nil)

// NewMessages 构造空仓库。
func NewMessages() *Messages { return &Messages{next: 1} }

// List 实现 domain.MessageRepository。
func (m *Messages) List(_ context.Context, receiver uint64, types []domain.MessageType, page domain.Page) ([]domain.Message, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	page = page.Normalize()
	allowed := make(map[domain.MessageType]struct{}, len(types))
	for _, typ := range types {
		allowed[typ] = struct{}{}
	}
	var hit []domain.Message
	for _, row := range m.rows {
		if row.Receiver != receiver {
			continue
		}
		if _, ok := allowed[row.Type]; !ok {
			continue
		}
		hit = append(hit, row)
	}
	sort.SliceStable(hit, func(i, j int) bool { return hit[i].CreatedAt.After(hit[j].CreatedAt) })
	offset := page.Offset()
	if offset >= len(hit) {
		return nil, false, nil
	}
	hit = hit[offset:]
	hasMore := len(hit) > page.Limit
	if hasMore {
		hit = hit[:page.Limit]
	}
	return append([]domain.Message(nil), hit...), hasMore, nil
}

// Find 实现 domain.MessageRepository。
func (m *Messages) Find(_ context.Context, receiver, id uint64) (domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.ID == id && row.Receiver == receiver {
			return row, nil
		}
	}
	return domain.Message{}, apperr.New(apperr.CodeMessageNotFound)
}

// MarkRead 实现 domain.MessageRepository。
func (m *Messages) MarkRead(_ context.Context, receiver, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		if m.rows[i].ID == id && m.rows[i].Receiver == receiver {
			now := time.Now()
			m.rows[i].IsRead = true
			m.rows[i].ReadAt = &now
			return nil
		}
	}
	return apperr.New(apperr.CodeMessageNotFound)
}

// MarkGroupRead 实现 domain.MessageRepository。
func (m *Messages) MarkGroupRead(_ context.Context, receiver uint64, types []domain.MessageType) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed := typeSet(types)
	var count int64
	now := time.Now()
	for i := range m.rows {
		if m.rows[i].Receiver != receiver || m.rows[i].IsRead {
			continue
		}
		if _, ok := allowed[m.rows[i].Type]; !ok {
			continue
		}
		m.rows[i].IsRead = true
		m.rows[i].ReadAt = &now
		count++
	}
	return count, nil
}

// Delete 实现 domain.MessageRepository。
func (m *Messages) Delete(_ context.Context, receiver, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		if m.rows[i].ID == id && m.rows[i].Receiver == receiver {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return nil
		}
	}
	return apperr.New(apperr.CodeMessageNotFound)
}

// DeleteGroup 实现 domain.MessageRepository。
func (m *Messages) DeleteGroup(_ context.Context, receiver uint64, types []domain.MessageType) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed := typeSet(types)
	kept := make([]domain.Message, 0, len(m.rows))
	var count int64
	for _, row := range m.rows {
		if row.Receiver == receiver {
			if _, ok := allowed[row.Type]; ok {
				count++
				continue
			}
		}
		kept = append(kept, row)
	}
	m.rows = kept
	return count, nil
}

// Create 实现 domain.MessageRepository。
func (m *Messages) Create(_ context.Context, msg *domain.Message) error {
	if msg == nil || msg.Receiver == 0 {
		return apperr.New(apperr.CodeMessageNotFound)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg.ID == 0 {
		msg.ID = m.next
		m.next++
	}
	now := time.Now()
	msg.CreatedAt = now
	msg.UpdatedAt = now
	m.rows = append(m.rows, *msg)
	return nil
}

// ExistsDigg 实现 domain.MessageRepository。
func (m *Messages) ExistsDigg(_ context.Context, actionUser uint64, typ domain.MessageType, articleID, commentID uint64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.ActionUser != actionUser || row.Type != typ {
			continue
		}
		if commentID != 0 {
			if row.CommentID == commentID {
				return true, nil
			}
			continue
		}
		if row.ArticleID == articleID {
			return true, nil
		}
	}
	return false, nil
}

// CountUnread 实现 domain.MessageRepository。
func (m *Messages) CountUnread(_ context.Context, receiver uint64) (map[domain.MessageType]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[domain.MessageType]int64)
	for _, row := range m.rows {
		if row.Receiver == receiver && !row.IsRead {
			out[row.Type]++
		}
	}
	return out, nil
}

// All 返回全部消息（测试断言用）。
func (m *Messages) All() []domain.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.Message(nil), m.rows...)
}

// Globals 是全局公告的内存实现。
type Globals struct {
	mu      sync.Mutex
	rows    []domain.GlobalNotif
	next    uint64
	read    map[uint64]map[uint64]bool // userID -> msgID -> read
	deleted map[uint64]map[uint64]bool // userID -> msgID -> deleted
	now     func() time.Time
}

var _ domain.GlobalRepository = (*Globals)(nil)

// NewGlobals 构造空仓库。
func NewGlobals(now func() time.Time) *Globals {
	if now == nil {
		now = time.Now
	}
	return &Globals{
		next:    1,
		read:    make(map[uint64]map[uint64]bool),
		deleted: make(map[uint64]map[uint64]bool),
		now:     now,
	}
}

// ListVisible 实现 domain.GlobalRepository。
func (g *Globals) ListVisible(_ context.Context, userID uint64, userCreatedAt time.Time, page domain.Page) ([]domain.GlobalNotif, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	var hit []domain.GlobalNotif
	for _, row := range g.rows {
		if g.deleted[userID][row.ID] {
			continue
		}
		if row.VisibleAt(userCreatedAt, now) && matchKey(row, page.Key) {
			hit = append(hit, row)
		}
	}
	return paginate(hit, page)
}

// ListAll 实现 domain.GlobalRepository。
func (g *Globals) ListAll(_ context.Context, page domain.Page) ([]domain.GlobalNotif, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	hit := make([]domain.GlobalNotif, 0, len(g.rows))
	for _, row := range g.rows {
		if matchKey(row, page.Key) {
			hit = append(hit, row)
		}
	}
	return paginate(hit, page)
}

// FindVisible 实现 domain.GlobalRepository。
func (g *Globals) FindVisible(_ context.Context, userID uint64, ids []uint64, userCreatedAt time.Time) ([]domain.GlobalNotif, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	want := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	var out []domain.GlobalNotif
	for _, row := range g.rows {
		if _, ok := want[row.ID]; !ok {
			continue
		}
		if g.deleted[userID][row.ID] {
			continue
		}
		if row.VisibleAt(userCreatedAt, now) {
			out = append(out, row)
		}
	}
	return out, nil
}

// MarkRead 实现 domain.GlobalRepository。
func (g *Globals) MarkRead(_ context.Context, userID uint64, ids []uint64) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.read[userID] == nil {
		g.read[userID] = make(map[uint64]bool)
	}
	count := 0
	for _, id := range ids {
		if g.read[userID][id] || g.deleted[userID][id] {
			continue
		}
		g.read[userID][id] = true
		count++
	}
	return count, nil
}

// DeleteForUser 实现 domain.GlobalRepository。
func (g *Globals) DeleteForUser(_ context.Context, userID uint64, ids []uint64) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.deleted[userID] == nil {
		g.deleted[userID] = make(map[uint64]bool)
	}
	count := 0
	for _, id := range ids {
		if g.deleted[userID][id] {
			continue
		}
		g.deleted[userID][id] = true
		count++
	}
	return count, nil
}

// ReadSet 实现 domain.GlobalRepository。
func (g *Globals) ReadSet(_ context.Context, userID uint64, ids []uint64) (map[uint64]bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		if g.read[userID][id] {
			out[id] = true
		}
	}
	return out, nil
}

// CountUnread 实现 domain.GlobalRepository。
func (g *Globals) CountUnread(_ context.Context, userID uint64, userCreatedAt time.Time) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	var count int64
	for _, row := range g.rows {
		if g.deleted[userID][row.ID] || g.read[userID][row.ID] {
			continue
		}
		if row.VisibleAt(userCreatedAt, now) {
			count++
		}
	}
	return count, nil
}

// Create 实现 domain.GlobalRepository。
func (g *Globals) Create(_ context.Context, n *domain.GlobalNotif) error {
	if n == nil {
		return apperr.New(apperr.CodeNotifCreateFailed)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if n.ID == 0 {
		n.ID = g.next
		g.next++
	}
	n.CreatedAt = g.now()
	g.rows = append(g.rows, *n)
	return nil
}

// ExistsTitle 实现 domain.GlobalRepository。
func (g *Globals) ExistsTitle(_ context.Context, title string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, row := range g.rows {
		if row.Title == title {
			return true, nil
		}
	}
	return false, nil
}

// DeleteMany 实现 domain.GlobalRepository。
func (g *Globals) DeleteMany(_ context.Context, ids []uint64) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	want := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	kept := make([]domain.GlobalNotif, 0, len(g.rows))
	count := 0
	for _, row := range g.rows {
		if _, ok := want[row.ID]; ok {
			count++
			continue
		}
		kept = append(kept, row)
	}
	g.rows = kept
	return count, nil
}

// Rows 返回全部公告（测试断言用）。
func (g *Globals) Rows() []domain.GlobalNotif {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]domain.GlobalNotif(nil), g.rows...)
}

// Confs 是通知开关的内存实现。
type Confs struct {
	mu   sync.Mutex
	rows map[uint64]domain.Conf
}

var _ domain.ConfRepository = (*Confs)(nil)

// NewConfs 构造空仓库。
func NewConfs() *Confs { return &Confs{rows: make(map[uint64]domain.Conf)} }

// Put 直接写入一份配置（测试播种用；对应真实环境里注册时建的那行）。
func (c *Confs) Put(userID uint64, conf domain.Conf) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[userID] = conf
}

// Find 实现 domain.ConfRepository。
func (c *Confs) Find(_ context.Context, userID uint64) (domain.Conf, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conf, ok := c.rows[userID]
	if !ok {
		return domain.Conf{}, apperr.New(apperr.CodeMessageConfMissing)
	}
	return conf, nil
}

// Replace 实现 domain.ConfRepository。
func (c *Confs) Replace(_ context.Context, userID uint64, conf domain.Conf) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.rows[userID]; !ok {
		return apperr.New(apperr.CodeMessageConfMissing)
	}
	c.rows[userID] = conf
	return nil
}

// Users 是用户目录的内存实现。
type Users struct {
	mu   sync.Mutex
	rows map[uint64]User
}

// User 是一条用户替身数据。
type User struct {
	CreatedAt time.Time
	Nickname  string
	Avatar    string
}

var _ domain.UserDirectory = (*Users)(nil)

// NewUsers 构造空目录。
func NewUsers() *Users { return &Users{rows: make(map[uint64]User)} }

// Put 写入一个用户。
func (u *Users) Put(id uint64, user User) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.rows[id] = user
}

// CreatedAt 实现 domain.UserDirectory。
func (u *Users) CreatedAt(_ context.Context, userID uint64) (time.Time, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	user, ok := u.rows[userID]
	if !ok {
		return time.Time{}, apperr.New(apperr.CodeNotifUserMissing)
	}
	return user.CreatedAt, nil
}

// Profile 实现 domain.UserDirectory。
func (u *Users) Profile(_ context.Context, userID uint64) (string, string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	user, ok := u.rows[userID]
	if !ok {
		return "", "", apperr.New(apperr.CodeNotifUserMissing)
	}
	return user.Nickname, user.Avatar, nil
}

// Chat 是私信未读的内存实现。
type Chat struct {
	mu     sync.Mutex
	unread map[uint64]int64
}

var _ domain.ChatUnreadCounter = (*Chat)(nil)

// NewChat 构造空计数器。
func NewChat() *Chat { return &Chat{unread: make(map[uint64]int64)} }

// Set 设置某用户的私信未读数。
func (c *Chat) Set(userID uint64, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unread[userID] = n
}

// UnreadTotal 实现 domain.ChatUnreadCounter。
func (c *Chat) UnreadTotal(_ context.Context, userID uint64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unread[userID], nil
}

// Env 把一组替身打包，便于测试一次性构造。
type Env struct {
	Messages *Messages
	Globals  *Globals
	Confs    *Confs
	Users    *Users
	Chat     *Chat
	Service  *app.Service
}

// NewEnv 用当前时间构造一整套内存替身。
func NewEnv() *Env {
	messages := NewMessages()
	globals := NewGlobals(time.Now)
	confs := NewConfs()
	users := NewUsers()
	chat := NewChat()
	return &Env{
		Messages: messages,
		Globals:  globals,
		Confs:    confs,
		Users:    users,
		Chat:     chat,
		Service:  app.New(messages, globals, confs, users, chat, nil, nil),
	}
}

// NewDefaultService 用空实现组装服务（路由表测试用，不关心行为）。
func NewDefaultService() *app.Service { return NewEnv().Service }

// ── 内部助手 ───────────────────────────────────────────────────

func typeSet(types []domain.MessageType) map[domain.MessageType]struct{} {
	out := make(map[domain.MessageType]struct{}, len(types))
	for _, typ := range types {
		out[typ] = struct{}{}
	}
	return out
}

func matchKey(row domain.GlobalNotif, key string) bool {
	if key == "" {
		return true
	}
	return strings.Contains(row.Title, key) || strings.Contains(row.Content, key)
}

func paginate(rows []domain.GlobalNotif, page domain.Page) ([]domain.GlobalNotif, bool, error) {
	page = page.Normalize()
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	offset := page.Offset()
	if offset >= len(rows) {
		return nil, false, nil
	}
	rows = rows[offset:]
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return rows, hasMore, nil
}
