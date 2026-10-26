package http

import (
	"database/sql"
	"strings"

	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/analytics/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/page"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

type CdcEventListRequest struct {
	page.PageInfo
	StartAt         string `form:"start_at"`
	EndAt           string `form:"end_at"`
	Service         string `form:"service"`
	Level           string `form:"level"`
	Stream          string `form:"stream"`
	SourceTable     string `form:"source_table"`
	Action          string `form:"action"`
	Result          string `form:"result"`
	RetryCount      *int   `form:"retry_count"`
	RequestID       string `form:"request_id"`
	RequestIDPrefix bool   `form:"request_id_prefix"`
	TraceID         string `form:"trace_id"`
	TraceIDPrefix   bool   `form:"trace_id_prefix"`
	CdcJobID        string `form:"cdc_job_id"`
	CdcJobIDPrefix  bool   `form:"cdc_job_id_prefix"`
	ErrorCode       string `form:"error_code"`
	ErrorMessage    string `form:"error_message"`
	EventName       string `form:"event_name"`
	Module          string `form:"module"`
}

type ReplayEventListRequest struct {
	page.PageInfo
	StartAt         string `form:"start_at"`
	EndAt           string `form:"end_at"`
	Service         string `form:"service"`
	Level           string `form:"level"`
	Stream          string `form:"stream"`
	SourceTable     string `form:"source_table"`
	Action          string `form:"action"`
	Result          string `form:"result"`
	RetryCount      *int   `form:"retry_count"`
	RequestID       string `form:"request_id"`
	RequestIDPrefix bool   `form:"request_id_prefix"`
	TraceID         string `form:"trace_id"`
	TraceIDPrefix   bool   `form:"trace_id_prefix"`
	CdcJobID        string `form:"cdc_job_id"`
	CdcJobIDPrefix  bool   `form:"cdc_job_id_prefix"`
	ErrorCode       string `form:"error_code"`
	ErrorMessage    string `form:"error_message"`
	EventName       string `form:"event_name"`
	Module          string `form:"module"`
}

type ChainQueryRequest struct {
	RequestID       string `form:"request_id"`
	TraceID         string `form:"trace_id"`
	CdcJobID        string `form:"cdc_job_id"`
	CdcJobIDPrefix  bool   `form:"cdc_job_id_prefix"`
	Stream          string `form:"stream"`
	Status          string `form:"status"`
	StartAt         string `form:"start_at"`
	EndAt           string `form:"end_at"`
	Limit           int    `form:"limit"`
	RequestIDPrefix bool   `form:"request_id_prefix"`
	TraceIDPrefix   bool   `form:"trace_id_prefix"`
}

type ChainDeadLetterRecord struct {
	ID          ctype.ID `json:"id"`
	Stream      string   `json:"stream"`
	CdcJobID    string   `json:"cdc_job_id"`
	SourceTable string   `json:"source_table"`
	Action      string   `json:"action"`
	TargetKey   string   `json:"target_key"`
	RetryCount  int      `json:"retry_count"`
	Status      string   `json:"status"`
	ErrorCode   string   `json:"error_code"`
	ErrorMsg    string   `json:"error_msg"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type ChainQueryResponse struct {
	RuntimeLogs []clickhouse.RuntimeLogRecord  `json:"runtime_logs"`
	LoginLogs   []clickhouse.LoginEventRecord  `json:"login_logs"`
	ActionLogs  []clickhouse.ActionAuditRecord `json:"action_logs"`
	CdcLogs     []clickhouse.CdcEventRecord    `json:"cdc_logs"`
	ReplayLogs  []clickhouse.ReplayEventRecord `json:"replay_logs"`
	DLQ         []ChainDeadLetterRecord        `json:"dlq"`
}

// CdcEventListView CDC 执行日志列表（管理员）。
// @Summary CDC 执行日志列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param service query string false "服务名"
// @Param level query string false "日志级别"
// @Param stream query string false "Stream 名称"
// @Param source_table query string false "源表"
// @Param action query string false "操作类型"
// @Param result query string false "执行结果"
// @Param retry_count query int false "重试次数"
// @Param request_id query string false "请求 ID"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id query string false "链路 ID"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Param cdc_job_id query string false "CDC 任务 ID"
// @Param cdc_job_id_prefix query bool false "CDC 任务 ID 前缀匹配"
// @Param error_code query string false "错误码"
// @Param error_message query string false "错误信息"
// @Param event_name query string false "事件名"
// @Param module query string false "模块"
// @Success 200 {object} kernel.Envelope[kernel.ListData[clickhouse.CdcEventRecord]]
// @Router /logs/cdc [get]
func (h *Handler) CdcEventListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CdcEventListRequest](c)
	list, count, err := clickhouse.ListCdcEvents(h.logs, clickhouse.CdcEventQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		Service:         cr.Service,
		Level:           cr.Level,
		Stream:          cr.Stream,
		SourceTable:     cr.SourceTable,
		Action:          cr.Action,
		Result:          cr.Result,
		RetryCount:      cr.RetryCount,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		CdcJobID:        cr.CdcJobID,
		CdcJobIDPrefix:  cr.CdcJobIDPrefix,
		ErrorCode:       cr.ErrorCode,
		ErrorMessage:    cr.ErrorMessage,
		EventName:       cr.EventName,
		Module:          cr.Module,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, int(count), c)
}

// CdcEventDetailView CDC 执行日志详情（管理员）。
// @Summary CDC 执行日志详情
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param id path string true "日志 ID"
// @Success 200 {object} kernel.Envelope[clickhouse.CdcEventRecord]
// @Router /logs/cdc/{id} [get]
func (h *Handler) CdcEventDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[IDRequest](c)
	item, err := clickhouse.GetCdcEvent(h.logs, cr.ID.Uint64())
	if err != nil {
		if err == sql.ErrNoRows {
			res.FailWithMsg("CDC 执行日志不存在", c)
			return
		}
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(item, c)
}

// ReplayEventListView 回放日志列表（管理员）。
// @Summary 回放日志列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param service query string false "服务名"
// @Param level query string false "日志级别"
// @Param stream query string false "Stream 名称"
// @Param source_table query string false "源表"
// @Param action query string false "操作类型"
// @Param result query string false "执行结果"
// @Param retry_count query int false "重试次数"
// @Param request_id query string false "请求 ID"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id query string false "链路 ID"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Param cdc_job_id query string false "CDC 任务 ID"
// @Param cdc_job_id_prefix query bool false "CDC 任务 ID 前缀匹配"
// @Param error_code query string false "错误码"
// @Param error_message query string false "错误信息"
// @Param event_name query string false "事件名"
// @Param module query string false "模块"
// @Success 200 {object} kernel.Envelope[kernel.ListData[clickhouse.ReplayEventRecord]]
// @Router /logs/replay [get]
func (h *Handler) ReplayEventListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ReplayEventListRequest](c)
	list, count, err := clickhouse.ListReplayEvents(h.logs, clickhouse.ReplayEventQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		Service:         cr.Service,
		Level:           cr.Level,
		Stream:          cr.Stream,
		SourceTable:     cr.SourceTable,
		Action:          cr.Action,
		Result:          cr.Result,
		RetryCount:      cr.RetryCount,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		CdcJobID:        cr.CdcJobID,
		CdcJobIDPrefix:  cr.CdcJobIDPrefix,
		ErrorCode:       cr.ErrorCode,
		ErrorMessage:    cr.ErrorMessage,
		EventName:       cr.EventName,
		Module:          cr.Module,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, int(count), c)
}

// ReplayEventDetailView 回放日志详情（管理员）。
// @Summary 回放日志详情
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param id path string true "日志 ID"
// @Success 200 {object} kernel.Envelope[clickhouse.ReplayEventRecord]
// @Router /logs/replay/{id} [get]
func (h *Handler) ReplayEventDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[IDRequest](c)
	item, err := clickhouse.GetReplayEvent(h.logs, cr.ID.Uint64())
	if err != nil {
		if err == sql.ErrNoRows {
			res.FailWithMsg("回放日志不存在", c)
			return
		}
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(item, c)
}

// ChainQueryView 全链路日志聚合查询（管理员）。
// @Summary 全链路日志聚合查询
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param request_id query string false "请求 ID"
// @Param trace_id query string false "链路 ID"
// @Param cdc_job_id query string false "CDC 任务 ID"
// @Param cdc_job_id_prefix query bool false "CDC 任务 ID 前缀匹配"
// @Param stream query string false "Stream 名称"
// @Param status query string false "死信状态"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param limit query int false "返回条数上限（默认 50，最大 200）"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Success 200 {object} kernel.Envelope[ChainQueryResponse]
// @Router /logs/chain [get]
func (h *Handler) ChainQueryView(c *gin.Context) {
	cr := middleware.GetBindQuery[ChainQueryRequest](c)
	if strings.TrimSpace(cr.RequestID) == "" && strings.TrimSpace(cr.TraceID) == "" && strings.TrimSpace(cr.CdcJobID) == "" {
		res.FailWithMsg("请至少传 request_id、trace_id、cdc_job_id 其中一个", c)
		return
	}

	limit := cr.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	opts := h.logs
	page := page.PageInfo{Limit: limit, Page: 1}
	timeRange := clickhouse.LogTimeRange{StartAt: cr.StartAt, EndAt: cr.EndAt}

	runtimeLogs, _, err := clickhouse.ListRuntimeLogs(opts, clickhouse.RuntimeLogQuery{
		PageInfo:        page,
		LogTimeRange:    timeRange,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	loginLogs, _, err := clickhouse.ListLoginEvents(opts, clickhouse.LoginEventQuery{
		PageInfo:        page,
		LogTimeRange:    timeRange,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	actionLogs, _, err := clickhouse.ListActionAudits(opts, clickhouse.ActionAuditQuery{
		PageInfo:        page,
		LogTimeRange:    timeRange,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	cdcLogs, _, err := clickhouse.ListCdcEvents(opts, clickhouse.CdcEventQuery{
		PageInfo:        page,
		LogTimeRange:    timeRange,
		Stream:          cr.Stream,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		CdcJobID:        cr.CdcJobID,
		CdcJobIDPrefix:  cr.CdcJobIDPrefix,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	replayLogs, _, err := clickhouse.ListReplayEvents(opts, clickhouse.ReplayEventQuery{
		PageInfo:        page,
		LogTimeRange:    timeRange,
		Stream:          cr.Stream,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		CdcJobID:        cr.CdcJobID,
		CdcJobIDPrefix:  cr.CdcJobIDPrefix,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	cdcJobIDSet := make(map[string]struct{})
	if strings.TrimSpace(cr.CdcJobID) != "" && !cr.CdcJobIDPrefix {
		cdcJobIDSet[strings.TrimSpace(cr.CdcJobID)] = struct{}{}
	}
	for _, item := range cdcLogs {
		if item.CdcJobID != "" {
			cdcJobIDSet[item.CdcJobID] = struct{}{}
		}
	}
	for _, item := range replayLogs {
		if item.CdcJobID != "" {
			cdcJobIDSet[item.CdcJobID] = struct{}{}
		}
	}
	cdcJobIDs := make([]string, 0, len(cdcJobIDSet))
	for cdcJobID := range cdcJobIDSet {
		cdcJobIDs = append(cdcJobIDs, cdcJobID)
	}

	if h.deadLetters == nil {
		res.OkWithData(ChainQueryResponse{
			RuntimeLogs: runtimeLogs,
			LoginLogs:   loginLogs,
			ActionLogs:  actionLogs,
			CdcLogs:     cdcLogs,
			ReplayLogs:  replayLogs,
			DLQ:         []ChainDeadLetterRecord{},
		}, c)
		return
	}
	dlqList, err := h.deadLetters.Query(reqCtx(c), domain.DeadLetterQuery{
		Stream:         cr.Stream,
		Status:         cr.Status,
		CdcJobID:       cr.CdcJobID,
		CdcJobIDPrefix: cr.CdcJobIDPrefix,
		CdcJobIDs:      cdcJobIDs,
		Limit:          limit,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	dlq := make([]ChainDeadLetterRecord, 0, len(dlqList))
	for _, item := range dlqList {
		dlq = append(dlq, ChainDeadLetterRecord{
			ID:          item.ID,
			Stream:      item.Stream,
			CdcJobID:    item.CdcJobID,
			SourceTable: item.SourceTable,
			Action:      item.Action,
			TargetKey:   item.TargetKey,
			RetryCount:  item.RetryCount,
			Status:      item.Status,
			ErrorCode:   item.ErrorCode,
			ErrorMsg:    item.ErrorMsg,
			CreatedAt:   item.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:   item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	res.OkWithData(ChainQueryResponse{
		RuntimeLogs: runtimeLogs,
		LoginLogs:   loginLogs,
		ActionLogs:  actionLogs,
		CdcLogs:     cdcLogs,
		ReplayLogs:  replayLogs,
		DLQ:         dlq,
	}, c)
}
