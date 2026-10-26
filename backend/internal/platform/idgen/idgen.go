// Package idgen 提供全局唯一的雪花 ID 生成能力。
//
// 归位自 service/db_service（S4.0）。这里只有"怎么生成 ID"的技术细节，
// 不含业务语义：机器号从启动参数读，纪元固定，生成过程无副作用。
//
// 临时依赖 models/ctype.ID 作为返回类型——ctype 被 186 个文件引用，
// 提前搬迁会淹没 S4.0 的主题。M6 删除 models/ 时，本包与 ctype 一起搬到
// internal/kernel，届时把返回类型换掉即可（调用点形态不变）。
package idgen

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/snowflake"

	"myblogx/internal/kernel/ctype"
)

const (
	// epochMillis 是雪花纪元：2024-01-01 00:00:00 UTC。上线后禁止修改，
	// 改了会让已发出的 ID 与新生 ID 落在不同区间，排序全部错乱。
	epochMillis int64 = 1704067200000
	// MaxWorkerID 是 10 位机器号的上限。
	MaxWorkerID uint32 = 1023
)

var (
	mu            sync.Mutex
	node          *snowflake.Node
	initializedID uint32
)

// Init 显式初始化雪花生成器。
//
// 同一机器号重复调用是幂等的；换成不同机器号返回错误——一个进程内混用两个
// 机器号会生成重复 ID，必须当场失败而不是悄悄生效。
func Init(workerID uint32) error {
	if workerID == 0 || workerID > MaxWorkerID {
		return fmt.Errorf("idgen: worker id must be within 1-%d, got %d", MaxWorkerID, workerID)
	}

	mu.Lock()
	defer mu.Unlock()

	if node != nil {
		if initializedID == workerID {
			return nil
		}
		return fmt.Errorf("idgen: already initialized with worker id %d, cannot switch to %d", initializedID, workerID)
	}

	// 第三方包使用全局纪元，只能在首次初始化时设置，且必须在 NewNode 之前。
	snowflake.Epoch = epochMillis

	created, err := snowflake.NewNode(int64(workerID))
	if err != nil {
		return fmt.Errorf("idgen: create node: %w", err)
	}

	node = created
	initializedID = workerID
	return nil
}

// Next 生成下一个全局唯一 ID。
//
// 未初始化时返回错误而不是自动兜底：自动兜底会让多实例部署悄悄共用机器号 1，
// 直到某天出现主键冲突才暴露。
func Next() (ctype.ID, error) {
	mu.Lock()
	current := node
	mu.Unlock()

	if current == nil {
		return "", fmt.Errorf("idgen: not initialized, call idgen.Init first")
	}
	return ctype.NewID(uint64(current.Generate().Int64())), nil
}

// MustReady 报告生成器是否已就绪，供启动自检使用。
func MustReady() bool {
	mu.Lock()
	defer mu.Unlock()
	return node != nil
}
