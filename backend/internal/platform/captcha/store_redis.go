package captcha

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
)

// KeyPrefix 是验证码在 Redis 里的键前缀。
//
// 带前缀的理由和这个仓库其它 Redis 用途一样：同一个库被会话、限流、验证码共用，
// 键名不带命名空间的话，一次 KEYS 扫描就分不清谁是谁，出问题也没法单独清某一类。
const KeyPrefix = "blogx:captcha:"

// TTLFunc 返回当前应当使用的有效期。
//
// 做成回调而不是构造时传入的定值：有效期是**运行时配置**，改它不该要求重启。
// 生成时按现值写 TTL，所以只影响新生成的验证码 —— 手里还没填的那张仍按它被
// 生成时的 TTL 过期，这符合"改配置不打扰正在登录的人"。
type TTLFunc func() time.Duration

// RedisStore 是 base64Captcha.Store 的 Redis 实现。
//
// 为什么不用库自带的 DefaultMemStore：它是**进程内**的 sync.Map。
//   - 重启即失效：正在登录的人刷新页面，手里的 captcha_id 就查不到了；
//   - 多副本必坏：api 起两个实例后，A 生成的 id 到 B 校验永远失败，
//     而接口层面完全看不出来（两边都返回 200，只是永远"验证码错误"）。
//
// 单实例时这两个问题都不显形，所以它们会在扩容/重启的那天才炸 —— 这属于
// 「先修掉」的那一类，不是等出问题再说。
type RedisStore struct {
	client *redis.Client
	ttl    TTLFunc
	logger *logrus.Logger
}

// NewRedisStore 构造 Redis 存储。client 为空时调用方应回退到内存存储。
func NewRedisStore(client *redis.Client, ttl TTLFunc, logger *logrus.Logger) *RedisStore {
	return &RedisStore{client: client, ttl: ttl, logger: logger}
}

// Set 写入一条验证码，TTL 取当前配置值。
//
// 写失败不 panic：验证码是"锦上添花"的安全手段，Redis 抖动时宁可退化成
// "这张图校验不过、让用户重试"，也不能把登录接口打成 500。
func (s *RedisStore) Set(id, value string) error {
	if s == nil || s.client == nil {
		// 这是编程错误（构造时没注入 client），不是给用户看的文案。
		// archcheck 的 R8 禁的是中文 errors.New（那类应当走 apperr 错误码），
		// 所以这里给英文并注明它是内部不变量。
		return errors.New("captcha store: redis client is nil")
	}
	ttl := s.currentTTL()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.client.Set(ctx, KeyPrefix+id, value, ttl).Err(); err != nil {
		if s.logger != nil {
			// 只记错误不记键、更不记答案：答案是验证码本身，进日志等于把答案写进磁盘。
			s.logger.Errorf("写入验证码失败: %v", err)
		}
		return err
	}
	return nil
}

// Get 读一条验证码；clear 为真时顺便删除（一次性使用）。
//
// 读不到返回空串：base64Captcha 的 Store 契约就是这么约定的
// （见库里的 MemoryStore.Get），换成自定义错误会让 Verify 的语义变味。
func (s *RedisStore) Get(id string, clear bool) string {
	if s == nil || s.client == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := KeyPrefix + id

	// clear=true 时用 GETDEL 一次取走并删除：GET + DEL 两步之间两个并发请求能各用一次，
	// 而"同一张图答对一次就作废"是这条链路上的防重放约定（ADR-0033 约束 8）。
	var value string
	var err error
	if clear {
		value, err = s.client.GetDel(ctx, key).Result()
	} else {
		value, err = s.client.Get(ctx, key).Result()
	}
	if err != nil {
		if !errors.Is(err, redis.Nil) && s.logger != nil {
			s.logger.Errorf("读取验证码失败: %v", err)
		}
		return ""
	}
	return value
}

// Verify 校验答案，比较时忽略大小写（字符集里是大写字母，用户习惯小写输入）。
//
// 成功即删除（一次性）。口径见 equalFoldTrim：两种存储（Redis 与降级内存）必须
// 共用同一个比较函数 —— 换存储不该悄悄改变"大小写敏不敏感"。
func (s *RedisStore) Verify(id, answer string, clear bool) bool {
	want := s.Get(id, clear)
	if want == "" {
		return false
	}
	return equalFoldTrim(want, answer)
}

// equalFoldTrim 是两种存储共用的答案比较口径：去首尾空白 + 忽略大小写。
//
// 与旧实现（base64Captcha 的 DefaultMemStore 用 `strings.ToLower(x) == strings.ToLower(y)`）
// 一致，另加 trim：手机键盘容易带出尾部空格，而"带个空格就错"是最常见的无效投诉。
func equalFoldTrim(want, answer string) bool {
	return strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(answer))
}

// currentTTL 取当前有效期；配置给 0 或异常时兜底 10 分钟（库默认值）。
func (s *RedisStore) currentTTL() time.Duration {
	if s == nil || s.ttl == nil {
		return 10 * time.Minute
	}
	ttl := s.ttl()
	if ttl <= 0 {
		return 10 * time.Minute
	}
	return ttl
}

// 编译期确认满足 base64Captcha 的存储契约。
var _ base64Captcha.Store = (*RedisStore)(nil)

// String 只用于日志与测试断言，不暴露答案。
func (s *RedisStore) String() string {
	return fmt.Sprintf("captcha.RedisStore(prefix=%s)", KeyPrefix)
}
