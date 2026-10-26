package imageref

// image_ref_river 死信的重投执行器
//
// 这条链路只有**删除类**死信能重投：
//
//	删除：payload = {owner_id, ref_type}          → 重新清理该对象的所有图片引用
//	新增：payload = {table, row}                  → 行数据是 binlog 原始 []any
//	更新：payload = {table, before, after}        → 同上
//
// 为什么新增/更新不能重投：`row` 里混着数字/字符串/时间，经 JSON 往返后数字变 float64、
// 时间变字符串，重建出来的行快照和真实类型对不上，`rebuildByRow` 会静默算错候选图。
// 这种"看起来成功了但引用错了"比重投失败可怕得多，所以宁可不支持，让人去重新保存对象
// （那会走正常的 binlog 路径，类型是准的）。
//
// 删除类之所以能支持，正因为它的 payload 里没有行数据，只有 owner_id + ref_type —— 自洽。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/platform/cdc/deadletter"

	"gorm.io/gorm"
)

// ReplayStream 是图片引用链路写进死信表的 stream 值。
const ReplayStream = "image_ref_river"

// Replayer 重投 image_ref_river 的删除类死信。
type Replayer struct {
	db *gorm.DB
}

// NewReplayer 构造执行器。
func NewReplayer(db *gorm.DB) *Replayer {
	return &Replayer{db: db}
}

// Streams 实现 dlq.Replayer。
func (r *Replayer) Streams() []string { return []string{ReplayStream} }

// imageRefReplayPayload 是 image_ref_river 死信 payload 的形状（三种动作的字段并集）。
type imageRefReplayPayload struct {
	OwnerID string          `json:"owner_id"`
	RefType string          `json:"ref_type"`
	Table   string          `json:"table"`
	Row     json.RawMessage `json:"row"`
	Before  json.RawMessage `json:"before"`
	After   json.RawMessage `json:"after"`
}

// Replay 实现 dlq.Replayer。
func (r *Replayer) Replay(ctx context.Context, record deadletter.Record) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("imageref: 数据库未初始化，无法重投")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	var payload imageRefReplayPayload
	raw := strings.TrimSpace(record.PayloadJSON)
	if raw == "" || raw == "{}" {
		return fmt.Errorf("imageref: 死信 payload 为空，无法重投（cdc_job_id=%s）", record.CdcJobID)
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("imageref: 死信 payload 不是合法 JSON: %w", err)
	}

	if payload.OwnerID == "" || payload.RefType == "" {
		return fmt.Errorf("imageref: 只支持删除类死信的重投（需要 owner_id 与 ref_type）；" +
			"新增/更新类的行数据经 JSON 往返后类型无法可靠还原，请重新保存该业务对象触发重建")
	}

	refType, err := image_ref_enum.ParseRefType(payload.RefType)
	if err != nil {
		return err
	}
	ownerID, err := ctype.ParseID(payload.OwnerID)
	if err != nil || ownerID.IsZero() {
		return fmt.Errorf("imageref: owner_id 不合法: %q", payload.OwnerID)
	}

	return DeleteOwnerRefs(r.db.WithContext(ctx), refType, ownerID)
}
