//go:build integration

// Package mysqlcontainer 用 testcontainers 起一个 MySQL 8，供仓储契约测试使用。
//
// 为什么不复用内存 sqlite：本项目大量使用 `ON CONFLICT`、`SELECT ... FOR UPDATE`、
// 唯一键冲突与 `AUTO_INCREMENT`，这些在 sqlite 与 MySQL 上行为并不一致
// （见 docs/architecture/baseline.md「这是假安全」一节）。契约测试必须打真库。
//
// 用法：
//
//	db := mysqlcontainer.DB(t, &models.ArticleModel{})   // 独立 schema + 自动建表
//	mysqlcontainer.Terminate(ctx)                        // 可选：TestMain 里统一收尾
//
// 优先级：BLOGX_MYSQL_DSN 环境变量 > testcontainers 自动启动。
// 没有 Docker 时 t.Skip，不阻断本地普通开发。
package mysqlcontainer

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcmsql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"myblogx/internal/platform/idgen"
)

const (
	// 容器内的 root 凭据。仅测试用，不涉及任何真实环境。
	rootUser = "root"
	rootPass = "root"
	// baseSchema 是容器启动时创建的库，仅用于执行 CREATE DATABASE。
	baseSchema = "blogx_it"
)

var (
	once      sync.Once
	addr      string // host:port
	container *tcmsql.MySQLContainer
	startErr  error
	seq       atomic.Uint64
)

// DSN 返回指向**本测试独立 schema** 的 root DSN。
//
// 每个调用创建一个新库（test_<序号>_<测试名>），因此 t.Parallel() 的子测试
// 互不可见，不会像共享库那样相互污染。
func DSN(t *testing.T) string {
	t.Helper()

	if custom := os.Getenv("BLOGX_MYSQL_DSN"); custom != "" {
		return custom
	}

	host := ensureAddr(t)
	name := schemaName(t)
	if err := createSchema(host, name); err != nil {
		t.Fatalf("mysqlcontainer: 建库失败: %v", err)
	}
	return buildDSN(host, name)
}

// DB 返回一个已连接、已建表（传入 models 时）的 gorm 实例。
//
// 连接在 t.Cleanup 时关闭。
func DB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(mysql.Open(DSN(t)), &gorm.Config{
		Logger: logger.Discard, // 契约测试只看结果，不要 SQL 噪音
	})
	if err != nil {
		t.Fatalf("mysqlcontainer: 连接 MySQL 失败: %v", err)
	}

	// models.Model.BeforeCreate 依赖雪花生成器；不初始化的话
	// 任何 Insert（即使没显式设 ID）都会直接报错。
	// 这属于"测试环境开箱可用"的职责，不该让每个测试自己记得初始化。
	if err := idgen.Init(1); err != nil {
		t.Fatalf("mysqlcontainer: 初始化雪花失败: %v", err)
	}

	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("mysqlcontainer: AutoMigrate 失败: %v", err)
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("mysqlcontainer: 取底层 *sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	return db
}

// Terminate 终止容器；建议在 TestMain 里 defer 调用。
// 未启动容器（例如用了 BLOGX_MYSQL_DSN）时是 no-op。
func Terminate(_ context.Context) {
	if container == nil {
		return
	}
	_ = testcontainers.TerminateContainer(container)
}

// ensureAddr 保证容器已启动，返回 host:port。
func ensureAddr(t *testing.T) string {
	t.Helper()

	once.Do(func() {
		addr, container, startErr = start()
	})
	if startErr != nil {
		t.Skipf("Docker/testcontainers 不可用，跳过集成测试: %v", startErr)
	}
	return addr
}

func start() (string, *tcmsql.MySQLContainer, error) {
	ctx := context.Background()

	ctr, err := tcmsql.Run(ctx, "mysql:8.0",
		tcmsql.WithDatabase(baseSchema),
		tcmsql.WithUsername(rootUser),
		tcmsql.WithPassword(rootPass),
		// 让服务器 SYSTEM 时区跟随 +08，种子 SQL 里用 NOW() 也稳定。
		testcontainers.WithEnv(map[string]string{"TZ": "Asia/Shanghai"}),
	)
	if err != nil {
		return "", nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", ctr, err
	}
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return "", ctr, err
	}
	return net.JoinHostPort(host, port.Port()), ctr, nil
}

// createSchema 建一个空库。用 baseSchema 作为入口，避免依赖容器初始化时机。
func createSchema(host, name string) error {
	db, err := gorm.Open(mysql.Open(buildDSN(host, baseSchema)), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	return db.Exec("CREATE DATABASE IF NOT EXISTS `" + name + "`").Error
}

// buildDSN 拼 DSN。
//
// loc 固定为 +08:00 而不是 Local：golden 要求"同一请求重构前后逐字节一致"，
// 若跟随运行机器的时区，换一台机器就会红。这里把时区钉死，生产配置仍用 Local。
func buildDSN(host, schema string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai",
		rootUser, rootPass, host, schema)
}

// schemaName 生成合法且唯一的库名（MySQL 库名上限 64 字符）。
func schemaName(t *testing.T) string {
	base := sanitize(t.Name())
	name := fmt.Sprintf("test_%d_%s", seq.Add(1), base)
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

func sanitize(name string) string {
	const maxLen = 40
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
