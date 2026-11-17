package captcha

// 这个文件是**包内**测试（package captcha 而不是 captcha_test）：currentTTL 是私有
// 的取值口径，它的兜底分支只能从包里测。
//
// 存储层不连真 Redis：没有 Redis 的机器/CI 切片上，连接会以超时告终而不是跳过，
// 那样验证码的存储语义就永远没人测。这里用 miniredis（go.mod 里已有的直接依赖，
// test/testutil 一直在用），既不新增依赖，也能真的跑 SET/GET/DEL/TTL 这几条命令。

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/go-redis/redis/v8"
	"github.com/mojocn/base64Captcha"
	"github.com/sirupsen/logrus"
)

// newTestRedisStore 起一个进程内的假 Redis，并返回指向它的存储。
func newTestRedisStore(t *testing.T, ttl TTLFunc) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(server.Close)

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger := logrus.New()
	// 失败路径会打日志；用例输出里不该混进"写入验证码失败"的噪音（真失败了断言会红）。
	logger.SetOutput(io.Discard)
	return NewRedisStore(client, ttl, logger), server
}

// currentTTL 的兜底口径必须与 base64Captcha 的默认过期一致（10 分钟）。
//
// 装配层漏传 ttl 回调用的是"进程能启动但参数没接上"这种最常见的漏配方式：
// 没有一个确定的兜底时，两个方向的后果都很糟 —— 兜成 0 会让写入变成"永不过期"，
// 等于给脚本一个无限爆破窗口；兜成很小的值则让人永远填不对。
func TestCurrentTTLFallback(t *testing.T) {
	tenMinutes := 10 * time.Minute

	cases := []struct {
		name string
		ttl  TTLFunc
		want time.Duration
	}{
		{"未注入 ttl 回调", nil, tenMinutes},
		{"ttl 回调返回 0（配置段还没加载）", func() time.Duration { return 0 }, tenMinutes},
		{"ttl 回调返回负数（脏数据）", func() time.Duration { return -3 * time.Second }, tenMinutes},
		{"正常值透传", func() time.Duration { return 4 * time.Minute }, 4 * time.Minute},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := NewRedisStore(nil, c.ttl, nil)
			if got := store.currentTTL(); got != c.want {
				t.Fatalf("currentTTL 应为 %v，实际 %v", c.want, got)
			}
		})
	}

	// 连 store 本身都是 nil 时也不能 panic：构造失败/装配顺序问题的兜底分支。
	var nilStore *RedisStore
	if got := nilStore.currentTTL(); got != tenMinutes {
		t.Fatalf("nil store 的 currentTTL 应为 %v，实际 %v", tenMinutes, got)
	}
}

// 写入的键必须带 caption 命名空间前缀。
//
// 这个 Redis 库被会话、限流、验证码共用：键名不带前缀时，运维一次 KEYS 扫描
// 分不清谁是谁，想单独清掉"那一批过期验证码"也无从下手；更糟的是两个用途
// 一旦用了同一个 id 就会互相覆盖。
func TestSetUsesKeyPrefix(t *testing.T) {
	store, server := newTestRedisStore(t, func() time.Duration { return 10 * time.Minute })

	if err := store.Set("abc123", "9876"); err != nil {
		t.Fatalf("写入验证码失败: %v", err)
	}
	if value, err := server.Get(KeyPrefix + "abc123"); err != nil || value != "9876" {
		t.Fatalf("键 %q 应存着答案 9876，实际 %q, err=%v", KeyPrefix+"abc123", value, err)
	}
	keys := server.Keys()
	if len(keys) != 1 || !strings.HasPrefix(keys[0], KeyPrefix) {
		t.Fatalf("验证码只应写一个带前缀的键，实际键列表: %v", keys)
	}
}

// Set 的 TTL 取写时刻的配置现值，且改配置只影响新写入的图。
//
// 有效期是运行时配置：运营把 90 秒改成 150 秒后，新生成的图该按 150 秒过期，
// 而用户手里那张已经发出去的图仍应按它生成时的 90 秒过期 —— 这符合"改配置
// 不打扰正在登录的人"。把 TTL 做成构造时的定值，后者前者会一起变，用户会在
// 填到一半时莫名过期。
func TestSetWritesTTLFromCurrentConfig(t *testing.T) {
	ttlSeconds := 90
	store, server := newTestRedisStore(t, func() time.Duration {
		return time.Duration(ttlSeconds) * time.Second
	})

	if err := store.Set("first", "1111"); err != nil {
		t.Fatalf("写入第一张验证码失败: %v", err)
	}
	if got := server.TTL(KeyPrefix + "first"); got != 90*time.Second {
		t.Fatalf("第一张的 TTL 应为 90s，实际 %v", got)
	}

	// 模拟运营在后台把有效期改成 150 秒：同一个 store，只改配置回调的返回值。
	ttlSeconds = 150
	if err := store.Set("second", "2222"); err != nil {
		t.Fatalf("写入第二张验证码失败: %v", err)
	}
	if got := server.TTL(KeyPrefix + "second"); got != 150*time.Second {
		t.Fatalf("第二张的 TTL 应跟着新配置走（150s），实际 %v", got)
	}
	// 已经发出的那张不许被改写：它的 TTL 仍是 90 秒。
	if got := server.TTL(KeyPrefix + "first"); got != 90*time.Second {
		t.Fatalf("改配置不该改写已发出的验证码 TTL（应仍是 90s），实际 %v", got)
	}
}

// Get 的 clear 参数决定"读后是否作废"。
//
// clear=false 是"读但不作废"：同一个请求链里可能要读两次（校验 + 日志），
// 提前删掉会让正常用户莫名失败。clear=true 才是一次性语义，防重放。
func TestGetClearsOnlyWhenAsked(t *testing.T) {
	store, _ := newTestRedisStore(t, nil)
	if err := store.Set("id-1", "1234"); err != nil {
		t.Fatalf("写入验证码失败: %v", err)
	}

	if got := store.Get("id-1", false); got != "1234" {
		t.Fatalf("clear=false 应读到答案 1234，实际 %q", got)
	}
	if got := store.Get("id-1", false); got != "1234" {
		t.Fatalf("clear=false 第二次仍应读到答案（不能读一次就删），实际 %q", got)
	}
	if got := store.Get("id-1", true); got != "1234" {
		t.Fatalf("clear=true 仍应返回本次读到的答案，实际 %q", got)
	}
	if got := store.Get("id-1", false); got != "" {
		t.Fatalf("clear=true 之后应读不到，实际 %q", got)
	}
}

// Verify 忽略大小写与首尾空白，成功即作废。
//
// 口径必须与 base64Captcha 自带的 MemoryStore 一致（它也是 ToLower 比较）：
// 字符集里是大写字母，用户习惯敲小写；换了存储实现不该悄悄改变"大小写敏不敏感"。
// 一次性同样重要：答对后这张图必须立刻作废，否则可以被多次重放。
func TestVerifyIgnoresCaseAndIsOneShot(t *testing.T) {
	store, _ := newTestRedisStore(t, nil)
	if err := store.Set("id-case", "AbCd"); err != nil {
		t.Fatalf("写入验证码失败: %v", err)
	}

	if !store.Verify("id-case", "  abcd  ", true) {
		t.Fatal("小写 + 空白的答案应校验通过（与库的 MemoryStore 同口径）")
	}
	if store.Verify("id-case", "abcd", true) {
		t.Fatal("校验成功后必须作废，同一答案不能二次通过（防重放）")
	}
}

// 查不到 id 时 Verify 返回 false、Get 返回空串，不 panic 也不报错。
//
// 这是 Store 的接口契约（库的 MemoryStore 同款）：验证码过期、被别处清掉、
// 或者用户拿着旧页面提交，都是最正常的"查不到"。把查不到变成错误值会让
// 中间件把一次普通失败打成服务异常，前端就会以为后端坏了。
func TestMissingIDIsNotAnError(t *testing.T) {
	store, _ := newTestRedisStore(t, nil)

	if got := store.Get("no-such-id", false); got != "" {
		t.Fatalf("查不到时应返回空串，实际 %q", got)
	}
	if store.Verify("no-such-id", "1234", true) {
		t.Fatal("查不到的 id 不能校验通过")
	}
}

// 没注入 Redis 客户端时退化成"校验不过"，而不是 panic/500。
//
// 装配顺序出错（先起 HTTP、后接 Redis）时，验证码是"锦上添花"的安全手段：
// 宁可让这张图填不对、请用户重试，也不能让整个登录接口 500 —— 那会把
// "验证码功能没就绪"升级成"站点登录不可用"。
func TestStoreWithoutClientDegradesInsteadOfPanic(t *testing.T) {
	store := NewRedisStore(nil, nil, nil)

	if err := store.Set("id", "1234"); err == nil {
		t.Fatal("没有 Redis 客户端时 Set 应返回错误（生成侧据此失败，而不是以为写成功了）")
	}
	if got := store.Get("id", false); got != "" {
		t.Fatalf("没有 Redis 客户端时 Get 应返回空串，实际 %q", got)
	}
	if store.Verify("id", "1234", true) {
		t.Fatal("没有 Redis 客户端时 Verify 必须为 false")
	}
}

// 编译期确认 RedisStore 仍然是 base64Captcha 的 Store（接口改动时这里先红）。
var _ base64Captcha.Store = NewRedisStore(nil, nil, nil)
