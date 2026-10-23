package log_api

import (
	"database/sql"
	"strings"

	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/cdc_dead_letter_service"
	"myblogx/service/log_service"

	"github.com/gin-gonic/gin"
)

type CdcEventListRequest struct {
	common.PageInfo
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
	common.PageInfo
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
	RuntimeLogs []log_service.RuntimeLogRecord  `json:"runtime_logs"`
	LoginLogs   []log_service.LoginEventRecord  `json:"login_logs"`
	ActionLogs  []log_service.ActionAuditRecord `json:"action_logs"`
	CdcLogs     []log_service.CdcEventRecord    `json:"cdc_logs"`
	ReplayLogs  []log_service.ReplayEventRecord `json:"replay_logs"`
	DLQ         []ChainDeadLetterRecord         `json:"dlq"`
}

func (h *LogApi) logDeps() log_service.Deps {
	return log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse)
}

func (h *LogApi) CdcEventListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CdcEventListRequest](c)
	list, count, err := log_service.ListCdcEvents(h.logDeps(), log_service.CdcEventQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: log_service.LogTimeRange{
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

func (h *LogApi) CdcEventDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	item, err := log_service.GetCdcEvent(h.logDeps(), uint64(cr.ID))
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

func (h *LogApi) ReplayEventListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ReplayEventListRequest](c)
	list, count, err := log_service.ListReplayEvents(h.logDeps(), log_service.ReplayEventQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: log_service.LogTimeRange{
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

func (h *LogApi) ReplayEventDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	item, err := log_service.GetReplayEvent(h.logDeps(), uint64(cr.ID))
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

func (h *LogApi) ChainQueryView(c *gin.Context) {
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

	deps := h.logDeps()
	page := common.PageInfo{Limit: limit, Page: 1}
	timeRange := log_service.LogTimeRange{StartAt: cr.StartAt, EndAt: cr.EndAt}

	runtimeLogs, _, err := log_service.ListRuntimeLogs(deps, log_service.RuntimeLogQuery{
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
	loginLogs, _, err := log_service.ListLoginEvents(deps, log_service.LoginEventQuery{
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
	actionLogs, _, err := log_service.ListActionAudits(deps, log_service.ActionAuditQuery{
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

	cdcLogs, _, err := log_service.ListCdcEvents(deps, log_service.CdcEventQuery{
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
	replayLogs, _, err := log_service.ListReplayEvents(deps, log_service.ReplayEventQuery{
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

	dlqList, err := cdc_dead_letter_service.Query(h.App.DB, cdc_dead_letter_service.QueryOption{
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
