package core

import (
	"fmt"

	"myblogx/conf"
	"myblogx/service/db_service"
)

const (
	maxSnowflakeNodeID uint32 = 1023 // 10位机器ID最大值
)

// InitSnowflake 在应用启动阶段显式初始化雪花生成器。
// 生产环境必须配置 system.server_id，避免多个实例使用同一机器号。
func InitSnowflake(config *conf.Config) error {
	if config == nil {
		return fmt.Errorf("配置未初始化，无法初始化雪花 ID 生成器")
	}

	workerID := config.System.ServerID

	if workerID == 0 || workerID > maxSnowflakeNodeID {
		return fmt.Errorf("雪花机器ID必须在 1-%d 之间，当前: %d", maxSnowflakeNodeID, workerID)
	}

	if err := db_service.InitSnowflake(workerID); err != nil {
		return fmt.Errorf("初始化雪花 ID 生成器失败: %w", err)
	}

	return nil
}
