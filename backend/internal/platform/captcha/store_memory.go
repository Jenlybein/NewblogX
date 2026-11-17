package captcha

import (
	"sync"
	"time"

	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
)

// TtlMemoryStore 是**进程内**的验证码存储，但有效期取配置现值。
//
// 它只用于 Redis 不可用时的降级（本地开发、单测、Redis 尚未接线的部署）。
// 为什么不用库自带的 base64Captcha.DefaultMemStore：
//   - 它的有效期在构造时固定成 10 分钟（包级 `Expiration`），后台改 expire_seconds
//     在降级路径上完全不生效，而 GET /imagecaptcha 仍会回显配置值 —— 两套时钟，
//     正是"保存成功但没效果"那种失败；
//   - 它的清理是"存满 10240 条才 GC 一次"，本地开发时等于不清理。
//
// 语义与 Redis 实现保持一致：Verify 忽略大小写、clear=true 一次性删除。
type TtlMemoryStore struct {
	mu      sync.Mutex
	entries map[string]entry
	ttl     TTLFunc
	logger  *logrus.Logger
}

type entry struct {
	value   string
	expires time.Time
}

// NewTtlMemoryStore 构造降级用的内存存储。
func NewTtlMemoryStore(ttl TTLFunc, logger *logrus.Logger) *TtlMemoryStore {
	return &TtlMemoryStore{entries: make(map[string]entry), ttl: ttl, logger: logger}
}

// Set 写入一条，TTL 取当前配置值；同时顺手清掉过期的条目（量小，不必单独 GC 协程）。
func (s *TtlMemoryStore) Set(id, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.gcLocked(now)
	s.entries[id] = entry{value: value, expires: now.Add(s.currentTTL())}
	return nil
}

// Get 读一条；clear 为真时删除。过期条目按不存在处理。
func (s *TtlMemoryStore) Get(id string, clear bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return ""
	}
	if !e.expires.After(time.Now()) {
		delete(s.entries, id)
		return ""
	}
	if clear {
		delete(s.entries, id)
	}
	return e.value
}

// Verify 校验答案，忽略大小写；成功即删除（与 Redis 实现同一口径）。
func (s *TtlMemoryStore) Verify(id, answer string, clear bool) bool {
	want := s.Get(id, clear)
	if want == "" {
		return false
	}
	return equalFoldTrim(want, answer)
}

// gcLocked 清掉过期条目。
func (s *TtlMemoryStore) gcLocked(now time.Time) {
	for id, e := range s.entries {
		if !e.expires.After(now) {
			delete(s.entries, id)
		}
	}
}

// currentTTL 与 Redis 实现同一兜底：配置给 0 或 ttl 缺失时用库默认的 10 分钟。
func (s *TtlMemoryStore) currentTTL() time.Duration {
	if s == nil || s.ttl == nil {
		return 10 * time.Minute
	}
	ttl := s.ttl()
	if ttl <= 0 {
		return 10 * time.Minute
	}
	return ttl
}

// String 只用于日志，不暴露条目内容。
func (s *TtlMemoryStore) String() string { return "captcha.TtlMemoryStore" }

var (
	_ base64Captcha.Store = (*TtlMemoryStore)(nil)
	_ base64Captcha.Store = (*RedisStore)(nil)
)
