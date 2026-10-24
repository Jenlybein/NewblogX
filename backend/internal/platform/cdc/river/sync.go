package river

// canal 事件回调与同步主循环
import (
	"time"

	"myblogx/internal/platform/es"
	"myblogx/service/log_service"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/pingcap/errors"
)

const (
	fieldTypeList = "list"
	// 用于mysql int类型到es日期类型的转换
	// 设置 [rule.field] created_time = ",date"
	fieldTypeDate = "date"
	// 用于将 mysql 的 1/0、true/false 等值写入 ES boolean 字段
	fieldTypeBool = "bool"
)

const mysqlDateFormat = "2006-01-02"

// posSaver 保存位置信息的结构
type posSaver struct {
	pos   mysql.Position // MySQL位置
	force bool           // 是否强制保存
}

type cdcTask struct {
	Request     *es_service.BulkRequest
	CdcJobID    string
	Stream      string
	SourceTable string
	Action      string
	TargetKey   string
}

// eventHandler 事件处理器
type eventHandler struct {
	r *River // River实例
}

// OnRotate 处理旋转事件
func (h *eventHandler) OnRotate(header *replication.EventHeader, e *replication.RotateEvent) error {
	pos := mysql.Position{
		Name: string(e.NextLogName),
		Pos:  uint32(e.Position),
	}

	h.r.syncCh <- posSaver{pos, true}

	return h.r.ctx.Err()
}

// OnTableChanged 处理表变更事件
func (h *eventHandler) OnTableChanged(header *replication.EventHeader, schema, table string) error {
	err := h.r.updateRule(schema, table)
	if err != nil && err != ErrRuleNotExist {
		return errors.Trace(err)
	}
	return nil
}

// OnDDL 处理DDL语句事件
func (h *eventHandler) OnDDL(header *replication.EventHeader, nextPos mysql.Position, queryEvent *replication.QueryEvent) error {
	h.r.syncCh <- posSaver{nextPos, true}
	return h.r.ctx.Err()
}

// OnXID 处理事务提交事件
func (h *eventHandler) OnXID(header *replication.EventHeader, nextPos mysql.Position) error {
	h.r.syncCh <- posSaver{nextPos, false}
	return h.r.ctx.Err()
}

// OnRow 处理行事件
func (h *eventHandler) OnRow(e *canal.RowsEvent) error {
	// outbox（领域事件）最先判断：它与 ES 无关，
	// 且必须命中后直接返回，否则会被当成业务数据同步进 ES。
	if h.r.handleOutboxEvent(e) {
		return h.r.ctx.Err()
	}

	if handled, err := h.r.handleArticleSearchProjectionEvent(e); handled {
		if err != nil {
			h.r.saveProjectionErrorToDLQ(e, err)
			h.r.cancel()
			return errors.Errorf("处理文章搜索读模型事件失败: 表=%s 动作=%s 错误=%v", e.Table.Name, e.Action, err)
		}
		return h.r.ctx.Err()
	}

	rule, ok := h.r.rules[ruleKey(e.Table.Schema, e.Table.Name)]
	if !ok {
		return nil
	}

	var reqs []*es_service.BulkRequest
	var err error
	switch e.Action {
	case canal.InsertAction:
		reqs, err = h.r.makeInsertRequest(rule, e.Rows)
	case canal.DeleteAction:
		reqs, err = h.r.makeDeleteRequest(rule, e.Rows)
	case canal.UpdateAction:
		reqs, err = h.r.makeUpdateRequest(rule, e.Rows)
	default:
		err = errors.Errorf("无效的行事件操作: %s", e.Action)
	}

	if err != nil {
		pos := h.r.canal.SyncedPosition()
		log_service.EmitCDCEvent(h.r.logDeps, log_service.CdcEventInput{
			Level:        "error",
			Message:      "river 构建批量请求失败",
			ErrorCode:    "RIVER_BUILD_FAILED",
			ErrorMessage: err.Error(),
			ErrorType:    "river_error",
			CdcJobID:     buildCDCJobID("es_river", e.Table.Schema, e.Table.Name, pos, 0),
			Stream:       "es_river",
			SourceTable:  e.Table.Schema + "." + e.Table.Name,
			Action:       e.Action,
			RetryCount:   0,
			Result:       "failed",
		})
		h.r.cancel()
		return errors.Errorf("构建 %s 的 ES 请求失败，停止同步: %v", e.Action, err)
	}

	pos := h.r.canal.SyncedPosition()
	tasks := wrapCDCReqs("es_river", e.Table.Schema, e.Table.Name, e.Action, pos, reqs)
	h.r.syncCh <- tasks

	return h.r.ctx.Err()
}

// OnGTID 处理GTID事件
func (h *eventHandler) OnGTID(header *replication.EventHeader, gtidEvent mysql.BinlogGTIDEvent) error {
	return nil
}

// OnPosSynced 处理位置同步事件
func (h *eventHandler) OnPosSynced(header *replication.EventHeader, pos mysql.Position, set mysql.GTIDSet, force bool) error {
	return nil
}

// OnRowsQueryEvent 处理行查询事件
func (h *eventHandler) OnRowsQueryEvent(e *replication.RowsQueryEvent) error {
	return nil
}

// String 返回事件处理器的字符串表示
func (h *eventHandler) String() string {
	return "ESRiverEventHandler"
}

// syncLoop 同步循环，处理同步请求
func (r *River) syncLoop() {
	bulkSize := r.cfg.BulkSize
	if bulkSize == 0 {
		bulkSize = 128
	}

	interval := time.Duration(r.cfg.FlushBulkTime) * time.Millisecond
	if interval == 0 {
		interval = 200 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer r.wg.Done()

	lastSavedTime := time.Now()
	reqs := make([]cdcTask, 0, 1024)

	var pos mysql.Position

	for {
		needFlush := false
		needSavePos := false

		select {
		case v := <-r.syncCh:
			switch v := v.(type) {
			case posSaver:
				now := time.Now()
				if v.force || now.Sub(lastSavedTime) > 3*time.Second {
					lastSavedTime = now
					needFlush = true
					needSavePos = true
					pos = v.pos
				}
			case []cdcTask:
				reqs = append(reqs, v...)
				needFlush = len(reqs) >= bulkSize
			}
		case <-ticker.C:
			needFlush = true
		case <-r.ctx.Done():
			return
		}

		if needFlush {
			if err := r.doBulkWithRetry(reqs); err != nil {
				if r.log != nil {
					r.log.Errorf("执行 ES 批量同步失败，已写入 DLQ: %v", err)
				}
			}
			reqs = reqs[0:0]
		}

		if needSavePos {
			if err := r.master.Save(pos); err != nil {
				if r.log != nil {
					r.log.Errorf("保存同步位点失败，停止同步: 位点=%s 错误=%v", pos, err)
				}
				r.cancel()
				return
			}
		}
	}
}

// makeRequest 为插入和删除操作创建请求
