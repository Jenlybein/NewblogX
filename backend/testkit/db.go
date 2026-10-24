package testkit

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sqliteSeq atomic.Uint64

// NewSQLite 打开一个测试专属的内存 sqlite 连接，并在 t.Cleanup 时关闭。
//
// 每次调用使用**独立的库名**，因此多个 t.Parallel() 子测试互不可见，
// 不会像共享 testutil.InitGlobals() 那样产生跨测试污染。
// 传入 models 时会自动建表。
func NewSQLite(t testing.TB, models ...any) *gorm.DB {
	t.Helper()

	name := fmt.Sprintf("testkit_%d_%s", sqliteSeq.Add(1), sanitize(t.Name()))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Discard, // 测试里不需要 SQL 日志噪音
	})
	if err != nil {
		t.Fatalf("testkit: 打开内存 sqlite 失败: %v", err)
	}

	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("testkit: AutoMigrate 失败: %v", err)
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("testkit: 取底层 *sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	return db
}

// sanitize 把测试名里的非法字符替换掉，保证能作为 sqlite 库名的一部分。
func sanitize(name string) string {
	const maxLen = 48
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= maxLen {
			break
		}
	}
	if b.Len() == 0 {
		return "t"
	}
	return b.String()
}
