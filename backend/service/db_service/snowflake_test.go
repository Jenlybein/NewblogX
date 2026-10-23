package db_service

import (
	"testing"
)

func resetSnowflakeStateForTest() {
	defaultSnowflake = nil
	initializedWorkerID = 0
}

func TestInitSnowflake(t *testing.T) {
	t.Cleanup(resetSnowflakeStateForTest)

	t.Run("机器号为 0 时失败", func(t *testing.T) {
		resetSnowflakeStateForTest()
		err := InitSnowflake(0)
		if err == nil {
			t.Fatal("机器号为 0 时应初始化失败")
		}
	})

	t.Run("配置合法机器号时成功", func(t *testing.T) {
		resetSnowflakeStateForTest()
		if err := InitSnowflake(7); err != nil {
			t.Fatalf("显式初始化雪花生成器失败: %v", err)
		}

		id, err := NextSnowflakeID()
		if err != nil {
			t.Fatalf("生成雪花 ID 失败: %v", err)
		}
		if id == 0 {
			t.Fatal("生成的雪花 ID 不应为 0")
		}
	})

	t.Run("重复相同机器号初始化可直接复用", func(t *testing.T) {
		resetSnowflakeStateForTest()
		if err := InitSnowflake(9); err != nil {
			t.Fatalf("首次初始化失败: %v", err)
		}
		if err := InitSnowflake(9); err != nil {
			t.Fatalf("相同机器号重复初始化不应失败: %v", err)
		}
	})

	t.Run("重复不同机器号初始化应失败", func(t *testing.T) {
		resetSnowflakeStateForTest()
		if err := InitSnowflake(10); err != nil {
			t.Fatalf("首次初始化失败: %v", err)
		}
		if err := InitSnowflake(11); err == nil {
			t.Fatal("不同机器号重复初始化应失败")
		}
	})
}

func TestNextSnowflakeIDRequiresInit(t *testing.T) {
	t.Cleanup(resetSnowflakeStateForTest)

	resetSnowflakeStateForTest()

	if _, err := NextSnowflakeID(); err == nil {
		t.Fatal("未初始化时生成雪花 ID 应返回错误")
	}
}
