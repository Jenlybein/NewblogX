package river

// bulk 提交、重试与死信落盘
import (
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/es"
	"myblogx/internal/platform/log/writer"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
)

func (r *River) doBulk(tasks []cdcTask) ([]cdcTask, string) {
	if len(tasks) == 0 {
		return nil, ""
	}

	reqs := make([]*es_service.BulkRequest, 0, len(tasks))
	for _, task := range tasks {
		reqs = append(reqs, task.Request)
	}

	resp := es_service.Bulk(r.es, reqs)
	if !resp.Success {
		message := strings.TrimSpace(resp.Msg)
		if message == "" {
			message = "bulk 请求失败"
		}
		return tasks, message
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return tasks, "bulk 响应解析失败"
	}
	var parsed es_service.BulkResponse
	if err = json.Unmarshal(dataBytes, &parsed); err != nil {
		return tasks, "bulk 响应结构解析失败"
	}

	if !parsed.Errors {
		return nil, ""
	}

	failed := make([]cdcTask, 0)
	for index, item := range parsed.Items {
		if index >= len(tasks) {
			break
		}
		for _, result := range item {
			if result == nil {
				failed = append(failed, tasks[index])
				break
			}
			if result.Status < 200 || result.Status >= 300 {
				failed = append(failed, tasks[index])
				break
			}
		}
	}
	if len(parsed.Items) < len(tasks) {
		failed = append(failed, tasks[len(parsed.Items):]...)
	}
	if len(failed) == 0 {
		return nil, ""
	}
	return failed, "bulk 响应包含失败项（errors=true）"
}

func (r *River) doBulkWithRetry(tasks []cdcTask) error {
	if len(tasks) == 0 {
		return nil
	}

	maxAttempts := r.cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	delay := time.Duration(r.cfg.Retry.DelayMS) * time.Millisecond
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}

	pending := tasks
	var lastMsg string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		failed, message := r.doBulk(pending)
		lastMsg = message
		if len(failed) == 0 {
			for _, task := range pending {
				writer.EmitCDCEvent(r.logOptions, writer.CdcEventInput{
					Level:       "info",
					Message:     "river 执行成功",
					CdcJobID:    task.CdcJobID,
					Stream:      task.Stream,
					SourceTable: task.SourceTable,
					Action:      task.Action,
					TargetKey:   task.TargetKey,
					RetryCount:  attempt - 1,
					Result:      "success",
				})
			}
			return nil
		}

		if attempt < maxAttempts {
			for _, task := range failed {
				writer.EmitCDCEvent(r.logOptions, writer.CdcEventInput{
					Level:        "warn",
					Message:      "river 执行失败，准备重试",
					ErrorCode:    "RIVER_BULK_RETRY",
					ErrorMessage: message,
					ErrorType:    "es_error",
					CdcJobID:     task.CdcJobID,
					Stream:       task.Stream,
					SourceTable:  task.SourceTable,
					Action:       task.Action,
					TargetKey:    task.TargetKey,
					RetryCount:   attempt,
					Result:       "retry",
				})
			}
			pending = failed
			time.Sleep(delay)
			continue
		}

		dlqItems := make([]deadletter.Item, 0, len(failed))
		for _, task := range failed {
			// 把**这条 bulk 请求自身的动作**也存下来。
			//
			// 为什么 payload 里必须有 action：一次 update 如果改了文档 ID，会被拆成
			// 「删旧文档（无 data）」+「写新文档（有 data）」两条请求，而它们的
			// cdcTask.Action 都是 canal 的 "update"。重投时只看 cdcTask.Action 的话，
			// 那条删除会被当成索引、然后因为缺 data 而永远失败（死信只能一直躺着）。
			// 以 payload 里的 ES 动作为准，cdcTask.Action 只作旧记录的兜底。
			payload := map[string]any{
				"action":   task.Request.Action,
				"index":    task.Request.Index,
				"type":     task.Request.Type,
				"id":       task.Request.ID,
				"parent":   task.Request.Parent,
				"pipeline": task.Request.Pipeline,
				"data":     task.Request.Data,
			}
			dlqItems = append(dlqItems, deadletter.Item{
				Stream:      task.Stream,
				CdcJobID:    task.CdcJobID,
				SourceTable: task.SourceTable,
				Action:      task.Action,
				TargetKey:   task.TargetKey,
				Payload:     payload,
				RetryCount:  attempt - 1,
				Status:      "pending",
				ErrorCode:   "RIVER_BULK_FAILED",
				ErrorMsg:    message,
			})

			writer.EmitCDCEvent(r.logOptions, writer.CdcEventInput{
				Level:        "error",
				Message:      "river 执行失败，已转入 DLQ",
				ErrorCode:    "RIVER_BULK_DLQ",
				ErrorMessage: message,
				ErrorType:    "es_error",
				CdcJobID:     task.CdcJobID,
				Stream:       task.Stream,
				SourceTable:  task.SourceTable,
				Action:       task.Action,
				TargetKey:    task.TargetKey,
				RetryCount:   attempt - 1,
				Result:       "dlq",
			})
		}
		if err := deadletter.SaveBatch(r.db, dlqItems); err != nil {
			return err
		}
		break
	}

	if strings.TrimSpace(lastMsg) == "" {
		lastMsg = "river 批量执行失败，已写入 DLQ"
	}
	return stdErrors.New(lastMsg)
}

func wrapCDCReqs(stream, schema, table, action string, pos mysql.Position, reqs []*es_service.BulkRequest) []cdcTask {
	sourceTable := schema + "." + table
	list := make([]cdcTask, 0, len(reqs))
	for index, req := range reqs {
		targetKey := ""
		if req != nil && req.ID != "" {
			targetKey = "doc_id=" + req.ID
		}
		list = append(list, cdcTask{
			Request:     req,
			CdcJobID:    buildCDCJobID(stream, schema, table, pos, index),
			Stream:      stream,
			SourceTable: sourceTable,
			Action:      action,
			TargetKey:   targetKey,
		})
	}
	return list
}

func buildCDCJobID(stream, schema, table string, pos mysql.Position, rowIndex int) string {
	return fmt.Sprintf("%s:%s.%s:%s:%d:%d", stream, schema, table, pos.Name, pos.Pos, rowIndex)
}

func (r *River) saveProjectionErrorToDLQ(e *canal.RowsEvent, err error) {
	if e == nil || e.Table == nil || err == nil {
		return
	}

	pos := mysql.Position{}
	if r.canal != nil {
		pos = r.canal.SyncedPosition()
	}
	stream := "es_projection"
	schemaName := e.Table.Schema
	tableName := e.Table.Name
	sourceTable := schemaName + "." + tableName
	cdcJobID := buildCDCJobID(stream, schemaName, tableName, pos, 0)

	payload := map[string]any{
		"schema":   schemaName,
		"table":    tableName,
		"action":   e.Action,
		"rowCount": len(e.Rows),
	}

	saveErr := deadletter.SaveBatch(r.db, []deadletter.Item{{
		Stream:      stream,
		CdcJobID:    cdcJobID,
		SourceTable: sourceTable,
		Action:      e.Action,
		TargetKey:   "",
		Payload:     payload,
		RetryCount:  0,
		Status:      "pending",
		ErrorCode:   "RIVER_PROJECTION_FAILED",
		ErrorMsg:    err.Error(),
	}})

	logInput := writer.CdcEventInput{
		Level:        "error",
		Message:      "projection 同步失败，已转入 DLQ",
		ErrorCode:    "RIVER_PROJECTION_DLQ",
		ErrorMessage: err.Error(),
		ErrorType:    "es_error",
		CdcJobID:     cdcJobID,
		Stream:       stream,
		SourceTable:  sourceTable,
		Action:       e.Action,
		RetryCount:   0,
		Result:       "dlq",
	}
	if saveErr != nil {
		logInput.Message = "projection 同步失败，DLQ 写入失败"
		logInput.ErrorCode = "RIVER_PROJECTION_DLQ_SAVE_FAILED"
		logInput.ErrorMessage = saveErr.Error()
		logInput.Result = "failed"
	}
	writer.EmitCDCEvent(r.logOptions, logInput)
}

// getFieldValue 获取mysql字段值并将其转换为特定的es值
