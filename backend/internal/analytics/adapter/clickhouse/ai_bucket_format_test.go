package clickhouse

import (
	"strings"
	"testing"
	"time"
)

// 本文件是 #357 的无 Docker 守卫：锁住"Go 侧桶键 == ClickHouse 侧桶键"这条契约。
//
// 为什么需要一条纯字符串测试：真正的回归测试要真 ClickHouse（见
// ai_usage_ch_integration_test.go），但格式串写错时 SQL 不会报错 ——
// ClickHouse 把 %M 当成月份英文名，查得到数据也对不上，整页静默补 0。
// 一条不用容器的断言能在本地随手跑、也能在 CI 的普通 job 里挡住这类笔误。

// TestAIBucketCHFormatEncodesMinutes 锁住分钟 token。
//
// ClickHouse 的 %M 是**月份英文名**（January..December），分钟是 %i。
// 写成 %M 时桶键会变成 "10:November:00" 这种形状，而 Go 侧是 "10:00:00"，
// 于是所有桶都查不到 —— 首页 AI 用量恒为 0 的直接原因。
func TestAIBucketCHFormatEncodesMinutes(t *testing.T) {
	if strings.Contains(aiBucketCHFormat, "%M") {
		t.Fatalf("aiBucketCHFormat = %q 含 %%M（ClickHouse 里是月份英文名），分钟必须用 %%i",
			aiBucketCHFormat)
	}
	if !strings.Contains(aiBucketCHFormat, "%i") {
		t.Fatalf("aiBucketCHFormat = %q 缺少 %%i（分钟），桶键无法与 Go 侧对齐",
			aiBucketCHFormat)
	}
}

// TestAIBucketFormatsDescribeSameInstant 用同一个时间点对账两套格式串。
//
// 期望值来自 Go 参考布局（"2006-01-02 15:04:05"）本身，不来自被测常量；
// 再按 ClickHouse formatDateTime 的语义把 CH 格式串渲染一遍，两边必须逐字节相同。
// 只断言"包含 %i"挡不住顺序写反或漏掉秒这类错误，这一条才真正对账。
func TestAIBucketFormatsDescribeSameInstant(t *testing.T) {
	at := time.Date(2026, 11, 11, 10, 44, 24, 0, cst)

	want := at.Format(aiBucketGoFormat)
	if want != "2026-11-11 10:44:24" {
		t.Fatalf("Go 侧桶键 = %q，期望 2026-11-11 10:44:24（参考布局被人改过？）", want)
	}
	if got := chFormatSample(t, aiBucketCHFormat, at); got != want {
		t.Fatalf("ClickHouse 格式串 %q 渲染成 %q，与 Go 侧桶键 %q 不一致",
			aiBucketCHFormat, got, want)
	}
}

// chFormatSample 按 ClickHouse formatDateTime 的语义渲染格式串。
//
// 只实现本包常量用得到的 token；碰到未知 token 直接让测试失败而不是悄悄忽略 ——
// 忽略的话，将来有人写错 token 这条对账会变成假绿。
func chFormatSample(t *testing.T, format string, at time.Time) string {
	t.Helper()

	var b strings.Builder
	for index := 0; index < len(format); index++ {
		if format[index] != '%' {
			b.WriteByte(format[index])
			continue
		}
		index++
		if index >= len(format) {
			t.Fatalf("格式串 %q 以未转义的 %% 结尾", format)
		}
		switch format[index] {
		case 'Y':
			b.WriteString(at.Format("2006"))
		case 'm':
			b.WriteString(at.Format("01"))
		case 'd':
			b.WriteString(at.Format("02"))
		case 'H':
			b.WriteString(at.Format("15"))
		case 'i':
			b.WriteString(at.Format("04"))
		case 'S':
			b.WriteString(at.Format("05"))
		case 'M':
			// ClickHouse 的 %M 是月份英文名（January..December）。
			b.WriteString(at.Format("January"))
		default:
			t.Fatalf("格式串 %q 含本测试未实现的 token %%%c", format, format[index])
		}
	}
	return b.String()
}
