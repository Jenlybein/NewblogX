package river

// canal 事件回调与位点主循环。
//
// # 这个包现在只做一件事
//
// 把 MySQL 的行变更翻译成"文章搜索投影要重算哪些文档"，交给 articlesync 去拼文档、写 ES。
//
// # 原来的第二条路径去哪了
//
// 2026-10-26 之前这里还有一条"通用规则"路径：配置里写 `表 → 索引 + 字段白名单`，
// 一行一行搬进 ES。它带着一整套批量合并、重试退避、死信证据，占了本包近一半代码。
//
// 删掉它的依据（详见 ADR-008）：
//  1. 配置里 `source` 的 6 张表**全部**由投影分支拦截并 return，通用路径对它们不可达；
//  2. 唯一一条 `rule` 指的也是 `article_models`（同样被拦截）；
//  3. 就算可达，它的白名单只有 8 个列，拼不出搜索文档（缺作者名/分类名/标签）；
//  4. 它的失败证据（完整 ES bulk body）对应的问题，投影这边已经用"行主键 + 按主键重算"解决了。
//
// 代价写在这里，避免以后有人以为是漏删：**加一张不需要 join 的表进 ES，现在要写 handler，
// 不能只改配置。** 真到那天，参照 article_search_projection.go 加一个分派即可。

import (
	"fmt"
	"time"

	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/log/writer"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
)

// posSaver 待落盘的 binlog 位点。
//
// force=true 用于 rotate/DDL/事务提交这些"位点必须立刻记住"的时刻。
type posSaver struct {
	pos   mysql.Position
	force bool
}

// eventHandler 事件处理器
type eventHandler struct {
	r *River
}

// OnRotate 处理 binlog 切换事件：位点换了文件，必须立刻记住。
func (h *eventHandler) OnRotate(header *replication.EventHeader, e *replication.RotateEvent) error {
	h.r.syncCh <- posSaver{
		pos:   mysql.Position{Name: string(e.NextLogName), Pos: uint32(e.Position)},
		force: true,
	}
	return h.r.ctx.Err()
}

// OnTableChanged 处理表结构变更。
//
// 以前它要刷新 rule 里的 TableInfo（通用路径靠列元数据拼文档）。通用路径删掉之后，
// 投影路径完全按列名从行里取值（`collectRowsIDs` 之类），不依赖缓存的表结构，
// 所以这里只剩一个空实现 —— 但接口方法不能省，canal 会回调它。
func (h *eventHandler) OnTableChanged(header *replication.EventHeader, schema, table string) error {
	return nil
}

// OnDDL 处理 DDL 语句事件
func (h *eventHandler) OnDDL(header *replication.EventHeader, nextPos mysql.Position, queryEvent *replication.QueryEvent) error {
	h.r.syncCh <- posSaver{pos: nextPos, force: true}
	return h.r.ctx.Err()
}

// OnXID 处理事务提交事件
func (h *eventHandler) OnXID(header *replication.EventHeader, nextPos mysql.Position) error {
	h.r.syncCh <- posSaver{pos: nextPos}
	return h.r.ctx.Err()
}

// OnRow 处理行事件。
//
// 分派顺序是有讲究的：
//  1. outbox 最先判断 —— 它是领域事件，与 ES 无关，必须命中后直接返回，
//     否则会被后面的投影分支当成业务数据；
//  2. 投影分支处理 6 张搜索相关表；
//  3. 其余表直接忽略（本服务只关心搜索投影）。
func (h *eventHandler) OnRow(e *canal.RowsEvent) error {
	if h.r.handleOutboxEvent(e) {
		return h.r.ctx.Err()
	}

	if handled, err := h.r.handleArticleSearchProjectionEvent(e); handled {
		if err != nil {
			// 记死信后**继续跑**，不 cancel、也不把 error 抛给 canal。
			//
			// 以前是"失败即停"：因为投影死信里没有主键、补不回来，停下来是当时唯一能
			// 保证"这次变更不丢"的办法。代价是一篇文章失败就让整条同步永久停摆
			// （River.Run 只在装配时调用一次，没有重启监督），而 API 照常服务、没人会知道。
			//
			// 现在死信带着行主键、可以按主键重算（ProjectionReplayer），取舍就变了：
			// 从"全站搜索停止更新"降到"这几篇文档晚几分钟才正确"。
			//
			// 两个细节都不能省：
			//   1. 必须返回 nil —— OnRow 返回 error 会让 canal 整体退出，和 cancel 一样是停摆；
			//   2. 必须留一条 error 日志 —— 死信页是"事后"看的，日志是"当时"看的。
			h.r.saveProjectionErrorToDLQ(e, err)
			if h.r.log != nil {
				h.r.log.Errorf("文章搜索投影失败，已写入 DLQ 并继续同步: 表=%s 动作=%s 错误=%v",
					e.Table.Name, e.Action, err)
			}
		}
		return h.r.ctx.Err()
	}

	return nil
}

// OnGTID 处理 GTID 事件（当前未使用）
func (h *eventHandler) OnGTID(header *replication.EventHeader, gtidEvent mysql.BinlogGTIDEvent) error {
	return nil
}

// OnPosSynced 处理位置同步事件（位点由 OnXID/OnDDL 负责，这里不需要）
func (h *eventHandler) OnPosSynced(header *replication.EventHeader, pos mysql.Position, set mysql.GTIDSet, force bool) error {
	return nil
}

// OnRowsQueryEvent 处理行查询事件（当前未使用）
func (h *eventHandler) OnRowsQueryEvent(e *replication.RowsQueryEvent) error {
	return nil
}

// String 返回事件处理器的字符串表示
func (h *eventHandler) String() string {
	return "ArticleSearchProjectionEventHandler"
}

// syncLoop 位点落盘循环。
//
// 通用路径删掉之后，同步通道里只剩 posSaver —— 真正写 ES 的动作由投影分支同步完成。
// 保留"攒一下再写"的行为：每次事务提交都落一次盘没有必要，3 秒一次足够
// （进程崩溃最多重放 3 秒的 binlog，而整条链路是幂等的）。
func (r *River) syncLoop() {
	defer r.wg.Done()

	const flushInterval = 3 * time.Second

	var (
		lastSavedTime = time.Now()
		pending       mysql.Position
		hasPending    bool
	)

	for {
		select {
		case v := <-r.syncCh:
			now := time.Now()
			if v.force || now.Sub(lastSavedTime) > flushInterval {
				pending, hasPending = v.pos, true
			}
		case <-r.ctx.Done():
			// 退出前把最后一个位点落盘：否则重启会重放一段已经同步过的 binlog
			// （幂等，但白干一遍）。
			if hasPending && r.master != nil {
				_ = r.master.Save(pending)
			}
			return
		}

		if !hasPending {
			continue
		}
		if err := r.master.Save(pending); err != nil {
			if r.log != nil {
				r.log.Errorf("保存同步位点失败，停止同步: 位点=%s 错误=%v", pending, err)
			}
			r.cancel()
			return
		}
		lastSavedTime = time.Now()
		hasPending = false
	}
}

// buildCDCJobID 生成"一次 CDC 作业"的稳定标识。
//
// 形如 `es_projection:blogx.article_models:mysql-bin.000123:4567:0`，
// 位点 + 行内序号保证同一事件重放两次得到同一个 ID —— 死信表的唯一键靠它去重。
func buildCDCJobID(stream, schema, table string, pos mysql.Position, rowIndex int) string {
	return fmt.Sprintf("%s:%s.%s:%s:%d:%d", stream, schema, table, pos.Name, pos.Pos, rowIndex)
}

// saveProjectionErrorToDLQ 把一次投影失败写进死信表。
//
// 这是"失败可恢复"的唯一证据来源，所以 payload 必须能让重投**定位到行**，
// 而不只是"知道哪张表出过错"。
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

	// ids 是**重投的定位依据**：没有它，死信只知道"这张表出过错"，不知道是哪几行，
	// 于是只能全量重建索引（几分钟、期间新旧文档混杂）。
	//
	// 为什么在落死信时就记、而不是重投时再猜：binlog 事件此刻才在手，
	// 重投发生在几小时后，那时再从"表名+动作+时间"反推是不可能可靠的。
	ids := collectRowsIDs(e, projectionIDColumn(tableName))

	payload := map[string]any{
		"schema":   schemaName,
		"table":    tableName,
		"action":   e.Action,
		"rowCount": len(e.Rows),
		"ids":      ids,
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
