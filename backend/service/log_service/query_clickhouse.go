package log_service

import (
	"context"
	"database/sql"
	"fmt"
)

// clickhouseEnabled 判断当前环境是否启用了 ClickHouse 日志查询能力。
func clickhouseEnabled(deps Deps) bool {
	return deps.ClickHouse != nil && deps.ClickHouseEnable
}

// queryCount 执行 count 查询并返回结果，统一处理 ClickHouse 未启用场景。
func queryCount(deps Deps, ctx context.Context, query string, args ...any) (int64, error) {
	if !clickhouseEnabled(deps) {
		return 0, fmt.Errorf("ClickHouse 未启用")
	}
	var count int64
	if err := deps.ClickHouse.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// queryRowExists 执行单行查询，供详情接口复用。
func queryRowExists(deps Deps, ctx context.Context, query string, args ...any) *sql.Row {
	return deps.ClickHouse.QueryRowContext(ctx, query, args...)
}
