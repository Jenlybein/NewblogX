package river

// es_river 死信的重投执行器
//
// 这条链路的死信 payload 是**完整的 ES bulk body**（见 sync_bulk.go 的 dlqItems）：
//
//	{"index":"...","type":"...","id":"...","parent":"...","pipeline":"...","data":{...}}
//
// 所以重投不需要回 MySQL 重新读数据，把同一份 body 再打一次即可。
// 幂等性来自 ES 的文档 ID：index 覆盖写、delete 删除不存在返回 404（我们视为成功）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"myblogx/internal/platform/cdc/deadletter"
	es_service "myblogx/internal/platform/es"

	"github.com/elastic/go-elasticsearch/v7"
)

// ESStream 是 river 直投 ES 时写进死信表的 stream 值。
const ESStream = "es_river"

// ESReplayer 把 es_river 的死信重新投给 Elasticsearch。
type ESReplayer struct {
	client *elasticsearch.Client
}

// NewESReplayer 构造执行器。client 为 nil 时每次重投都会明确报错（而不是静默成功）。
func NewESReplayer(client *elasticsearch.Client) *ESReplayer {
	return &ESReplayer{client: client}
}

// Streams 实现 dlq.Replayer。
func (r *ESReplayer) Streams() []string { return []string{ESStream} }

// Replay 实现 dlq.Replayer。
func (r *ESReplayer) Replay(ctx context.Context, record deadletter.Record) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if r == nil || r.client == nil {
		return fmt.Errorf("river: ES 客户端未初始化，无法重投")
	}
	req, err := buildReplayRequest(record)
	if err != nil {
		return err
	}

	resp := es_service.Bulk(r.client, []*es_service.BulkRequest{req})
	if !resp.Success {
		msg := strings.TrimSpace(resp.Msg)
		if msg == "" {
			msg = "bulk 请求失败"
		}
		return fmt.Errorf("river: 重投失败: %s", msg)
	}

	// 与 doBulk 的判定口径一致：bulk 接口 200 不代表逐条成功，要看 items 里的 status。
	// 这里不抽公共函数是有意的：doBulk 要返回"哪些 task 失败了"供批量重试，
	// 而这里只需要"这一条成没成"。共用会逼出一个两头都不顺手的签名。
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return fmt.Errorf("river: bulk 响应无法解析: %w", err)
	}
	var parsed es_service.BulkResponse
	if err = json.Unmarshal(dataBytes, &parsed); err != nil {
		return fmt.Errorf("river: bulk 响应结构不合法: %w", err)
	}
	if !parsed.Errors {
		return nil
	}
	for _, item := range parsed.Items {
		for action, result := range item {
			if result == nil {
				return fmt.Errorf("river: 重投结果缺失（%s）", action)
			}
			// 删除一个已经不存在的文档返回 404，语义上就是"目标状态已达成"。
			if result.Status == 404 && req.Action == es_service.ActionDelete {
				continue
			}
			if result.Status < 200 || result.Status >= 300 {
				return fmt.Errorf("river: 重投被 ES 拒绝（%s status=%d）", action, result.Status)
			}
		}
	}
	return nil
}

// esReplayPayload 是 es_river 死信 payload 的形状。
type esReplayPayload struct {
	Index    string         `json:"index"`
	Type     string         `json:"type"`
	ID       string         `json:"id"`
	Parent   string         `json:"parent"`
	Pipeline string         `json:"pipeline"`
	Data     map[string]any `json:"data"`
}

// buildReplayRequest 把死信 payload 还原成一条 ES bulk 请求。
//
// 动作从**死信记录的 Action 列**还原，因为 payload 里没有 action：
// canal 的 delete 对应 ES 的 delete，insert/update 都走 index（覆盖写，幂等）。
func buildReplayRequest(record deadletter.Record) (*es_service.BulkRequest, error) {
	var payload esReplayPayload
	raw := strings.TrimSpace(record.PayloadJSON)
	if raw == "" || raw == "{}" {
		return nil, fmt.Errorf("river: 死信 payload 为空，无法重投（cdc_job_id=%s）", record.CdcJobID)
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("river: 死信 payload 不是合法 JSON: %w", err)
	}
	if strings.TrimSpace(payload.Index) == "" {
		return nil, fmt.Errorf("river: 死信 payload 缺少 index，无法重投")
	}
	if strings.TrimSpace(payload.ID) == "" {
		return nil, fmt.Errorf("river: 死信 payload 缺少文档 ID，无法重投")
	}

	action := es_service.ActionIndex
	if strings.EqualFold(strings.TrimSpace(record.Action), "delete") {
		action = es_service.ActionDelete
	}
	if action != es_service.ActionDelete && len(payload.Data) == 0 {
		return nil, fmt.Errorf("river: 死信 payload 缺少 data，无法重建文档")
	}

	return &es_service.BulkRequest{
		Action:   action,
		Index:    payload.Index,
		Type:     payload.Type,
		ID:       payload.ID,
		Parent:   payload.Parent,
		Pipeline: payload.Pipeline,
		Data:     payload.Data,
	}, nil
}
