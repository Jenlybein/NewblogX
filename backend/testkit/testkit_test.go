package testkit_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"myblogx/testkit"
)

// row 是隔离测试用的最小表结构。
type row struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

// 关键验收：多个 t.Parallel() 子测试各持一个库，互不可见。
func TestNewSQLiteParallelIsolation(t *testing.T) {
	t.Parallel()

	for i := 0; i < 8; i++ {
		i := i
		t.Run(fmt.Sprintf("case%d", i), func(t *testing.T) {
			t.Parallel()

			db := testkit.NewSQLite(t, &row{})
			name := fmt.Sprintf("r%d", i)
			if err := db.Create(&row{Name: name}).Error; err != nil {
				t.Fatalf("插入失败: %v", err)
			}

			var got int64
			if err := db.Model(&row{}).Count(&got).Error; err != nil {
				t.Fatalf("统计失败: %v", err)
			}
			if got != 1 {
				t.Fatalf("库未隔离：期望 1 行，实际 %d 行", got)
			}
		})
	}
}

func TestFakeClock(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := testkit.NewFakeClock(start)

	if !c.Now().Equal(start) {
		t.Fatalf("初始时间不符: %v", c.Now())
	}
	next := c.Advance(90 * time.Minute)
	if !next.Equal(start.Add(90*time.Minute)) || !c.Now().Equal(next) {
		t.Fatalf("Advance 结果不符: %v", c.Now())
	}

	later := start.Add(48 * time.Hour)
	c.Set(later)
	if !c.Now().Equal(later) {
		t.Fatalf("Set 结果不符: %v", c.Now())
	}
}

func TestRealClock(t *testing.T) {
	t.Parallel()

	before := time.Now()
	got := testkit.RealClock{}.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Fatalf("RealClock.Now 不在调用区间内: %v", got)
	}
}

func TestSeqIDMonotonic(t *testing.T) {
	t.Parallel()

	id := testkit.NewSeqID(100)
	if got := id.Next(); got != 101 {
		t.Fatalf("首个 ID 期望 101，实际 %d", got)
	}
	if got := id.Next(); got != 102 {
		t.Fatalf("第二个 ID 期望 102，实际 %d", got)
	}
}

func TestCompareGolden(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	payload := []byte("hello golden\n")

	// 首次以 update 模式写入。
	testkit.CompareGolden(t, dir, "demo.txt", payload, true)

	if _, err := os.Stat(dir + "/demo.txt"); err != nil {
		t.Fatalf("golden 文件未生成: %v", err)
	}

	// 再以比较模式读回，内容一致应通过。
	testkit.CompareGolden(t, dir, "demo.txt", payload, false)
}
