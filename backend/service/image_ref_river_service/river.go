package image_ref_river_service

import (
	"fmt"
	"strings"
	"time"

	"myblogx/conf"
	"myblogx/models/ctype"
	"myblogx/models/enum/image_ref_enum"
	"myblogx/service/cdc_dead_letter_service"
	"myblogx/service/log_service"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// River 基于 MySQL Binlog 的图片引用关系监听服务
// 作用：监听文章/用户/横幅/收藏表的数据变化，自动维护图片引用关系
type River struct {
	canal   *canal.Canal
	cfg     conf.ImageRefRiver
	qiNiu   conf.QiNiu
	log     *logrus.Logger
	logDeps log_service.Deps
	db      *gorm.DB
}

// NewRiver 初始化图片引用关系监听服务
func NewRiver(config conf.ImageRefRiver, qiNiuConfig conf.QiNiu, logger *logrus.Logger, db *gorm.DB) (*River, error) {
	r := &River{
		cfg:   config,
		qiNiu: qiNiuConfig,
		log:   logger,
		db:    db,
	}
	// 初始化 canal 客户端
	if err := r.newCanal(); err != nil {
		return nil, err
	}
	// 设置 binlog 事件处理器
	r.canal.SetEventHandler(&eventHandler{river: r})
	return r, nil
}

func (r *River) SetLogDeps(deps log_service.Deps) {
	r.logDeps = deps
}

// newCanal 配置并创建 MySQL Binlog 监听客户端
func (r *River) newCanal() error {
	// 创建默认 canal 配置
	cfg := canal.NewDefaultConfig()
	// 绑定日志适配器
	cfg.Logger = logrusToSlogAdapter(r.log)
	// 从全局配置加载 MySQL 连接信息
	cfg.Addr = r.cfg.Mysql.Addr
	cfg.User = r.cfg.Mysql.User
	cfg.Password = r.cfg.Mysql.Password
	cfg.Charset = r.cfg.Charset
	cfg.Flavor = r.cfg.Flavor
	cfg.ServerID = r.cfg.ServerID
	// 全量数据备份相关配置
	cfg.Dump.ExecutionPath = r.cfg.DumpExec
	cfg.Dump.SkipMasterData = r.cfg.SkipMasterData

	// 获取要监听的数据库名
	schema := strings.TrimSpace(r.cfg.Schema)
	// 监听四张核心业务表
	for _, table := range []string{"article_models", "user_models", "banner_models", "favorite_models"} {
		cfg.IncludeTableRegex = append(cfg.IncludeTableRegex, schema+"\\."+table)
	}

	var err error
	// 创建 canal 实例
	r.canal, err = canal.NewCanal(cfg)
	if err != nil {
		return err
	}
	// 添加需要全量转储的表
	r.canal.AddDumpTables(schema, "article_models", "user_models", "banner_models", "favorite_models")
	return nil
}

// Run 启动图片引用监听服务
func (r *River) Run() error {
	return r.canal.Run()
}

// eventHandler 实现 canal.EventHandler 接口，处理数据库行事件
type eventHandler struct{ river *River }

type imageRefTask struct {
	CdcJobID    string
	Stream      string
	SourceTable string
	Action      string
	TargetKey   string
}

// OnRow 处理数据表行变化（增/删/改）
func (h *eventHandler) OnRow(e *canal.RowsEvent) error {
	// 根据表名获取对应的引用类型和重建函数
	refType, rebuildByRow, ok := tableHandler(e.Table.Name)
	if !ok {
		// 非监听表，直接忽略
		return nil
	}

	// 当前批次行的列布局固定，索引映射只构建一次并在本批次复用。
	columnNames := make([]string, 0, len(e.Table.Columns))
	for _, column := range e.Table.Columns {
		columnNames = append(columnNames, column.Name)
	}
	layout := newRowLayout(columnNames)

	// 根据操作类型处理
	switch e.Action {
	case canal.DeleteAction:
		// 删除操作：删除该业务对象关联的所有图片引用
		for index, ownerID := range extractRowIDs(e) {
			task := newImageRefTask(h.river, e, index, fmt.Sprintf("owner_id=%d", ownerID))
			if err := h.river.runTaskWithRetry(task, map[string]any{
				"owner_id": ownerID,
				"ref_type": refType.String(),
			}, func() error {
				return DeleteOwnerRefs(h.river.db, refType, ownerID)
			}); err != nil {
				return err
			}
		}
	case canal.InsertAction:
		// 新增操作：重建该条数据的图片引用关系
		for index, row := range e.Rows {
			rowSnapshot := newRowSnapshot(layout, row)
			targetKey := fmt.Sprintf("owner_id=%s", snapshotIDString(rowSnapshot))
			task := newImageRefTask(h.river, e, index, targetKey)
			if err := h.river.runTaskWithRetry(task, map[string]any{
				"table": e.Table.Name,
				"row":   row,
			}, func() error {
				return rebuildByRow(h.river.db, h.river.qiNiu, rowSnapshot)
			}); err != nil {
				return err
			}
		}
	case canal.UpdateAction:
		// 更新操作：使用新数据重建图片引用（e.Rows[i] 是更新后的数据）
		for i := 1; i < len(e.Rows); i += 2 {
			before := newRowSnapshot(layout, e.Rows[i-1])
			after := newRowSnapshot(layout, e.Rows[i])
			if !shouldRebuildOnUpdate(e.Table.Name, before, after) {
				continue
			}
			rowIndex := i / 2
			targetKey := fmt.Sprintf("owner_id=%s", snapshotIDString(after))
			task := newImageRefTask(h.river, e, rowIndex, targetKey)
			if err := h.river.runTaskWithRetry(task, map[string]any{
				"table":  e.Table.Name,
				"before": e.Rows[i-1],
				"after":  e.Rows[i],
			}, func() error {
				return rebuildByRow(h.river.db, h.river.qiNiu, after)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// 以下方法为 canal.EventHandler 接口必须实现的空实现
func (h *eventHandler) OnRotate(_ *replication.EventHeader, _ *replication.RotateEvent) error {
	return nil
}
func (h *eventHandler) OnTableChanged(_ *replication.EventHeader, _, _ string) error { return nil }
func (h *eventHandler) OnDDL(_ *replication.EventHeader, _ mysql.Position, _ *replication.QueryEvent) error {
	return nil
}
func (h *eventHandler) OnXID(_ *replication.EventHeader, _ mysql.Position) error { return nil }
func (h *eventHandler) OnGTID(_ *replication.EventHeader, _ mysql.BinlogGTIDEvent) error {
	return nil
}
func (h *eventHandler) OnPosSynced(_ *replication.EventHeader, _ mysql.Position, _ mysql.GTIDSet, _ bool) error {
	return nil
}
func (h *eventHandler) OnRowsQueryEvent(_ *replication.RowsQueryEvent) error { return nil }

// String 返回事件处理器名称
func (h *eventHandler) String() string {
	return "ImageRefRiverEventHandler"
}

// tableHandler 根据表名映射：图片引用类型 + 对应行数据处理函数
func tableHandler(table string) (image_ref_enum.RefType, func(*gorm.DB, conf.QiNiu, rowSnapshot) error, bool) {
	switch table {
	case "article_models":
		return image_ref_enum.RefTypeArticle, RebuildArticleRefsByRow, true
	case "user_models":
		return image_ref_enum.RefTypeUser, RebuildUserRefsByRow, true
	case "banner_models":
		return image_ref_enum.RefTypeBanner, RebuildBannerRefsByRow, true
	case "favorite_models":
		return image_ref_enum.RefTypeFavorite, RebuildFavoriteRefsByRow, true
	default:
		return 0, nil, false
	}
}

func shouldRebuildOnUpdate(table string, before rowSnapshot, after rowSnapshot) bool {
	beforeDeleted := before.IsDeleted()
	afterDeleted := after.IsDeleted()
	if beforeDeleted != afterDeleted {
		return true
	}
	if afterDeleted {
		return false
	}
	switch table {
	case "article_models":
		return !before.EqualString(after, "content") || !before.EqualString(after, "cover")
	case "user_models":
		return !before.EqualString(after, "avatar")
	case "banner_models":
		return !before.EqualString(after, "cover")
	case "favorite_models":
		return !before.EqualString(after, "cover")
	default:
		return true
	}
}

// extractRowIDs 从 binlog 行数据中提取所有主键 ID（去重）
func extractRowIDs(e *canal.RowsEvent) []ctype.ID {
	// 找到 id 列的索引位置
	index := findIDColumnIndex(e)
	if index < 0 {
		return nil
	}

	result := make([]ctype.ID, 0, len(e.Rows))
	seen := make(map[ctype.ID]struct{}, len(e.Rows)) // 去重

	// 遍历每一行数据，提取 ID
	for _, row := range e.Rows {
		id, ok := rowValueToID(row[index])
		if !ok {
			continue
		}
		// 已存在则跳过
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// findIDColumnIndex 查找表中 id 列的索引
func findIDColumnIndex(e *canal.RowsEvent) int {
	for index, column := range e.Table.Columns {
		if column.Name == "id" {
			return index
		}
	}
	return -1
}

// rowValueToID 将 binlog 中的字段值转换为业务 ID 类型
func rowValueToID(value any) (ctype.ID, bool) {
	var id ctype.ID
	// 扫描值到自定义 ID 类型
	if err := id.Scan(value); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func buildImageRefCDCJobID(e *canal.RowsEvent, pos mysql.Position, rowIndex int) string {
	return fmt.Sprintf("%s:%s.%s:%s:%d:%d", "image_ref_river", e.Table.Schema, e.Table.Name, pos.Name, pos.Pos, rowIndex)
}

func newImageRefTask(river *River, e *canal.RowsEvent, rowIndex int, targetKey string) imageRefTask {
	pos := mysql.Position{}
	if river != nil && river.canal != nil {
		pos = river.canal.SyncedPosition()
	}
	if e != nil && e.Header != nil && e.Header.LogPos > 0 {
		pos.Pos = uint32(e.Header.LogPos)
	}
	return imageRefTask{
		CdcJobID:    buildImageRefCDCJobID(e, pos, rowIndex),
		Stream:      "image_ref_river",
		SourceTable: e.Table.Schema + "." + e.Table.Name,
		Action:      e.Action,
		TargetKey:   targetKey,
	}
}

func snapshotIDString(snapshot rowSnapshot) string {
	id, err := snapshot.ID()
	if err != nil {
		return "0"
	}
	return fmt.Sprintf("%d", id)
}

func (r *River) runTaskWithRetry(task imageRefTask, payload map[string]any, runner func() error) error {
	maxAttempts := r.cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	delay := time.Duration(r.cfg.Retry.DelayMS) * time.Millisecond
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = runner()
		if lastErr == nil {
			log_service.EmitCDCEvent(r.logDeps, log_service.CdcEventInput{
				Level:       "info",
				Message:     "image_ref_river 同步成功",
				CdcJobID:    task.CdcJobID,
				Stream:      task.Stream,
				SourceTable: task.SourceTable,
				Action:      task.Action,
				TargetKey:   task.TargetKey,
				RetryCount:  attempt - 1,
				Result:      "success",
			})
			return nil
		}
		if attempt >= maxAttempts {
			break
		}
		log_service.EmitCDCEvent(r.logDeps, log_service.CdcEventInput{
			Level:        "warn",
			Message:      "image_ref_river 同步失败，准备重试",
			ErrorCode:    "IMAGE_REF_RIVER_RETRY",
			ErrorMessage: lastErr.Error(),
			ErrorType:    "db_error",
			CdcJobID:     task.CdcJobID,
			Stream:       task.Stream,
			SourceTable:  task.SourceTable,
			Action:       task.Action,
			TargetKey:    task.TargetKey,
			RetryCount:   attempt,
			Result:       "retry",
		})
		time.Sleep(delay)
	}

	if err := cdc_dead_letter_service.SaveBatch(r.db, []cdc_dead_letter_service.Item{{
		Stream:      task.Stream,
		CdcJobID:    task.CdcJobID,
		SourceTable: task.SourceTable,
		Action:      task.Action,
		TargetKey:   task.TargetKey,
		Payload:     payload,
		RetryCount:  maxAttempts - 1,
		Status:      "pending",
		ErrorCode:   "IMAGE_REF_RIVER_FAILED",
		ErrorMsg:    lastErr.Error(),
	}}); err != nil {
		return err
	}

	log_service.EmitCDCEvent(r.logDeps, log_service.CdcEventInput{
		Level:        "error",
		Message:      "image_ref_river 同步失败，已转入 DLQ",
		ErrorCode:    "IMAGE_REF_RIVER_DLQ",
		ErrorMessage: lastErr.Error(),
		ErrorType:    "db_error",
		CdcJobID:     task.CdcJobID,
		Stream:       task.Stream,
		SourceTable:  task.SourceTable,
		Action:       task.Action,
		TargetKey:    task.TargetKey,
		RetryCount:   maxAttempts - 1,
		Result:       "dlq",
	})
	return nil
}
