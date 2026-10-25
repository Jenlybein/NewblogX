//go:build integration

package mysqlcontainer_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

func TestMain(m *testing.M) {
	code := m.Run()
	mysqlcontainer.Terminate(context.Background())
	os.Exit(code)
}

// TestIsolatedSchema 证明每次调用拿到的是不同的库，互不可见。
func TestIsolatedSchema(t *testing.T) {
	t.Parallel()

	a := mysqlcontainer.DB(t)
	b := mysqlcontainer.DB(t)

	if err := a.Exec("CREATE TABLE only_in_a (id INT PRIMARY KEY)").Error; err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	var count int64
	if err := b.Raw(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'only_in_a'",
	).Scan(&count).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("两个测试应各自独立建库，但 b 看到了 a 的表")
	}
}

// TestAutoMigrateAndUniqueKey 同时验证两件事：
//  1. AutoMigrate 在真 MySQL 上能建出项目的表；
//  2. 唯一键冲突是**真冲突**——这正是 sqlite 上测不出来的那类行为。
func TestAutoMigrateAndUniqueKey(t *testing.T) {
	t.Parallel()

	db := mysqlcontainer.DB(t, &schema.ArticleRow{}, &schema.TagRow{})

	if !db.Migrator().HasTable(&schema.ArticleRow{}) {
		t.Fatal("AutoMigrate 后应存在 article 表")
	}

	if err := db.Exec("CREATE TABLE uniq_probe (id BIGINT PRIMARY KEY, k VARCHAR(64), UNIQUE KEY uk_k (k))").Error; err != nil {
		t.Fatalf("建探针表失败: %v", err)
	}
	if err := db.Exec("INSERT INTO uniq_probe (id, k) VALUES (1, 'x')").Error; err != nil {
		t.Fatalf("首次插入应成功: %v", err)
	}

	err := db.Exec("INSERT INTO uniq_probe (id, k) VALUES (2, 'x')").Error
	if err == nil {
		t.Fatal("重复键应报错，说明唯一约束没生效")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("期望 duplicate entry 错误，实际: %v", err)
	}
}

// TestTimeRoundTripIsStable 验证 golden 真正依赖的性质：
// 同一个时刻写进去、读出来必须是同一个，且解析出的偏移固定为 +08:00。
//
// 只断言服务器 NOW() 不够——loc 参数管的是客户端解析，不是服务器时区。
func TestTimeRoundTripIsStable(t *testing.T) {
	t.Parallel()

	db := mysqlcontainer.DB(t)

	if err := db.Exec("CREATE TABLE tz_probe (id BIGINT PRIMARY KEY, at DATETIME)").Error; err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if err := db.Exec("INSERT INTO tz_probe (id, at) VALUES (1, ?)", want).Error; err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	var got time.Time
	if err := db.Raw("SELECT at FROM tz_probe WHERE id = 1").Scan(&got).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if !got.Equal(want) {
		t.Fatalf("时间往返不一致: 写入 %s，读出 %s", want, got)
	}
	if _, offset := got.Zone(); offset != 8*3600 {
		t.Fatalf("解析偏移应为 +08:00，实际 %d 秒（golden 会随机器时区漂移）", offset)
	}
}
