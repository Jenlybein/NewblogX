package clickhouse

import (
	"strings"
	"testing"
)

// 热力图取数语句的形状约束。
//
// 背景：2026-11-14 之前热力图一直是 0。数据在 ClickHouse 里、SQL 手跑也出得来，
// 真因是 `toDate(ts, tz)` 返回 **Date** 列，而 clickhouse-go 的 Date 列只支持扫进
// time.Time / sql.Scanner；扫进 *string 直接返回 ColumnConverterError，
// 这个错误又被上层当成"热力图留空"吞掉。
//
// 所以这里锁两件事：day 必须是字符串（toString 包裹），以及过滤口径不被悄悄放宽。
func TestStudioActivitySQLSelectsStringDay(t *testing.T) {
	if !strings.Contains(studioActivitySQL, "toString(toDate(ts, ?))") {
		t.Fatalf("day 必须 toString 包裹，否则 Date→*string 扫描会失败：\n%s", studioActivitySQL)
	}
	if strings.Contains(studioActivitySQL, "SELECT toDate(") {
		t.Fatalf("不要选 Date 列，driver 扫不进 string：\n%s", studioActivitySQL)
	}
}

func TestStudioActivitySQLFilters(t *testing.T) {
	if !strings.Contains(studioActivitySQL, "success = 1") {
		t.Fatal("只数成功的动作：失败动作没有产出内容")
	}
	for _, action := range []string{"article_create", "article_update"} {
		if !strings.Contains(studioActivitySQL, action) {
			t.Fatalf("动作名必须是确切值（审计里带实体前缀），缺 %s", action)
		}
	}
	if strings.Contains(studioActivitySQL, "LIKE 'create%'") || strings.Contains(studioActivitySQL, "LIKE 'update%'") {
		t.Fatal("不要再退回前缀匹配：审计动作名带实体前缀，前缀一行都匹配不到")
	}
	if !strings.Contains(studioActivitySQL, "GROUP BY day") {
		t.Fatal("一天一行：用 GROUP BY 而不是把明细拉回来求和")
	}
}
