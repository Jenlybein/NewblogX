package db_service

import (
	"fmt"

	"github.com/bwmarrin/snowflake"

	"myblogx/models/ctype"
)

const (
	snowflakeEpochMillis int64  = 1704067200000 // 2024-01-01 00:00:00 UTC（上线后禁止修改）
	maxSnowflakeNodeID   uint32 = 1023
)

var (
	defaultSnowflake    *snowflake.Node
	initializedWorkerID uint32
)

// InitSnowflake 显式初始化雪花生成器。
// 初始化成功后，重复传入相同机器号会直接复用；传入不同机器号会返回错误。
func InitSnowflake(workerID uint32) error {
	if workerID == 0 || workerID > maxSnowflakeNodeID {
		return fmt.Errorf("雪花机器ID必须在 1-%d 之间，当前: %d", maxSnowflakeNodeID, workerID)
	}

	if defaultSnowflake != nil {
		if initializedWorkerID == workerID {
			return nil
		}
		return fmt.Errorf("雪花ID生成器已使用机器号 %d 初始化，不能再改为 %d", initializedWorkerID, workerID)
	}

	// 第三方包使用全局纪元，这里只在首次初始化时设置。
	snowflake.Epoch = snowflakeEpochMillis

	node, err := snowflake.NewNode(int64(workerID))
	if err != nil {
		return err
	}

	defaultSnowflake = node
	initializedWorkerID = workerID
	return nil
}

// NextSnowflakeID 生成全局唯一雪花ID。
func NextSnowflakeID() (ctype.ID, error) {
	if defaultSnowflake == nil {
		return 0, fmt.Errorf("雪花ID生成器未初始化，请先调用 core.InitSnowflake 或 db_service.InitSnowflake")
	}
	return ctype.ID(defaultSnowflake.Generate().Int64()), nil
}
