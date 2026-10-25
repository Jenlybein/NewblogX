package http

import (
	"database/sql"

	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/page"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
)

type RuntimeLogListRequest struct {
	page.PageInfo
	StartAt         string   `form:"start_at"`
	EndAt           string   `form:"end_at"`
	Service         string   `form:"service"`
	Level           string   `form:"level"`
	Host            string   `form:"host"`
	Method          string   `form:"method"`
	Path            string   `form:"path"`
	UserID          ctype.ID `form:"user_id"`
	RequestID       string   `form:"request_id"`
	RequestIDPrefix bool     `form:"request_id_prefix"`
	TraceID         string   `form:"trace_id"`
	TraceIDPrefix   bool     `form:"trace_id_prefix"`
	ErrorCode       string   `form:"error_code"`
	ErrorMessage    string   `form:"error_message"`
	EventName       string   `form:"event_name"`
	Module          string   `form:"module"`
}

type LoginLogListRequest struct {
	page.PageInfo
	StartAt         string   `form:"start_at"`
	EndAt           string   `form:"end_at"`
	UserID          ctype.ID `form:"user_id"`
	IP              string   `form:"ip"`
	Username        string   `form:"username"`
	LoginType       string   `form:"login_type"`
	EventName       string   `form:"event_name"`
	EventNameExact  string   `form:"event_name_exact"`
	Success         *bool    `form:"success"`
	RequestID       string   `form:"request_id"`
	RequestIDPrefix bool     `form:"request_id_prefix"`
	TraceID         string   `form:"trace_id"`
	TraceIDPrefix   bool     `form:"trace_id_prefix"`
	ErrorCode       string   `form:"error_code"`
	ErrorMessage    string   `form:"error_message"`
	Module          string   `form:"module"`
}

type ActionAuditListRequest struct {
	page.PageInfo
	StartAt         string   `form:"start_at"`
	EndAt           string   `form:"end_at"`
	UserID          ctype.ID `form:"user_id"`
	IP              string   `form:"ip"`
	ActionName      string   `form:"action_name"`
	TargetType      string   `form:"target_type"`
	TargetID        string   `form:"target_id"`
	Success         *bool    `form:"success"`
	Path            string   `form:"path"`
	RequestID       string   `form:"request_id"`
	RequestIDPrefix bool     `form:"request_id_prefix"`
	TraceID         string   `form:"trace_id"`
	TraceIDPrefix   bool     `form:"trace_id_prefix"`
	ErrorCode       string   `form:"error_code"`
	ErrorMessage    string   `form:"error_message"`
	EventName       string   `form:"event_name"`
	EventNameLike   string   `form:"event_name_like"`
	Module          string   `form:"module"`
}

// RuntimeLogListView 运行日志列表（管理员）。
// @Summary 运行日志列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param service query string false "服务名"
// @Param level query string false "日志级别"
// @Param host query string false "主机"
// @Param method query string false "HTTP 方法"
// @Param path query string false "请求路径"
// @Param user_id query int false "用户 ID"
// @Param request_id query string false "请求 ID"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id query string false "链路 ID"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Param error_code query string false "错误码"
// @Param error_message query string false "错误信息"
// @Param event_name query string false "事件名"
// @Param module query string false "模块"
// @Success 200 {object} object{code=int,data=object{list=[]clickhouse.RuntimeLogRecord,count=int},msg=string}
// @Router /logs/runtime [get]
func (h *Handler) RuntimeLogListView(c *gin.Context) {
	cr := middleware.GetBindQuery[RuntimeLogListRequest](c)
	list, count, err := clickhouse.ListRuntimeLogs(h.logs, clickhouse.RuntimeLogQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
			Key:   cr.Key,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		Service:         cr.Service,
		Level:           cr.Level,
		Host:            cr.Host,
		Method:          cr.Method,
		Path:            cr.Path,
		UserID:          cr.UserID,
		Key:             cr.Key,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
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

// RuntimeLogDetailView 运行日志详情（管理员）。
// @Summary 运行日志详情
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} object{code=int,data=clickhouse.RuntimeLogRecord,msg=string}
// @Router /logs/runtime/{id} [get]
func (h *Handler) RuntimeLogDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[IDRequest](c)
	item, err := clickhouse.GetRuntimeLog(h.logs, uint64(cr.ID))
	if err != nil {
		if err == sql.ErrNoRows {
			res.FailWithMsg("运行日志不存在", c)
			return
		}
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(item, c)
}

// LoginLogListView 登录日志列表（管理员）。
// @Summary 登录日志列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param user_id query int false "用户 ID"
// @Param ip query string false "IP 地址"
// @Param username query string false "用户名"
// @Param login_type query string false "登录方式"
// @Param event_name query string false "事件名（模糊匹配）"
// @Param event_name_exact query string false "事件名（精确匹配）"
// @Param success query bool false "是否成功"
// @Param request_id query string false "请求 ID"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id query string false "链路 ID"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Param error_code query string false "错误码"
// @Param error_message query string false "错误信息"
// @Param module query string false "模块"
// @Success 200 {object} object{code=int,data=object{list=[]clickhouse.LoginEventRecord,count=int},msg=string}
// @Router /logs/login [get]
func (h *Handler) LoginLogListView(c *gin.Context) {
	cr := middleware.GetBindQuery[LoginLogListRequest](c)
	eventNameLike := cr.EventName
	list, count, err := clickhouse.ListLoginEvents(h.logs, clickhouse.LoginEventQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		UserID:          cr.UserID,
		IP:              cr.IP,
		Username:        cr.Username,
		LoginType:       cr.LoginType,
		EventName:       cr.EventNameExact,
		EventNameLike:   eventNameLike,
		Success:         cr.Success,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		ErrorCode:       cr.ErrorCode,
		ErrorMessage:    cr.ErrorMessage,
		Module:          cr.Module,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, int(count), c)
}

// LoginLogDetailView 登录日志详情（管理员）。
// @Summary 登录日志详情
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} object{code=int,data=clickhouse.LoginEventRecord,msg=string}
// @Router /logs/login/{id} [get]
func (h *Handler) LoginLogDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[IDRequest](c)
	item, err := clickhouse.GetLoginEvent(h.logs, uint64(cr.ID))
	if err != nil {
		if err == sql.ErrNoRows {
			res.FailWithMsg("登录事件不存在", c)
			return
		}
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(item, c)
}

// ActionAuditListView 操作审计日志列表（管理员）。
// @Summary 操作审计日志列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param start_at query string false "开始时间"
// @Param end_at query string false "结束时间"
// @Param user_id query int false "用户 ID"
// @Param ip query string false "IP 地址"
// @Param action_name query string false "操作名"
// @Param target_type query string false "目标类型"
// @Param target_id query string false "目标 ID"
// @Param success query bool false "是否成功"
// @Param path query string false "请求路径"
// @Param request_id query string false "请求 ID"
// @Param request_id_prefix query bool false "请求 ID 前缀匹配"
// @Param trace_id query string false "链路 ID"
// @Param trace_id_prefix query bool false "链路 ID 前缀匹配"
// @Param error_code query string false "错误码"
// @Param error_message query string false "错误信息"
// @Param event_name query string false "事件名（模糊匹配）"
// @Param event_name_like query string false "事件名（模糊匹配，event_name 为空时生效）"
// @Param module query string false "模块"
// @Success 200 {object} object{code=int,data=object{list=[]clickhouse.ActionAuditRecord,count=int},msg=string}
// @Router /logs/action [get]
func (h *Handler) ActionAuditListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ActionAuditListRequest](c)
	eventNameLike := cr.EventName
	if eventNameLike == "" {
		eventNameLike = cr.EventNameLike
	}
	list, count, err := clickhouse.ListActionAudits(h.logs, clickhouse.ActionAuditQuery{
		PageInfo: page.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: clickhouse.LogTimeRange{
			StartAt: cr.StartAt,
			EndAt:   cr.EndAt,
		},
		UserID:          cr.UserID,
		IP:              cr.IP,
		ActionName:      cr.ActionName,
		TargetType:      cr.TargetType,
		TargetID:        cr.TargetID,
		Success:         cr.Success,
		Path:            cr.Path,
		RequestID:       cr.RequestID,
		RequestIDPrefix: cr.RequestIDPrefix,
		TraceID:         cr.TraceID,
		TraceIDPrefix:   cr.TraceIDPrefix,
		ErrorCode:       cr.ErrorCode,
		ErrorMessage:    cr.ErrorMessage,
		EventNameLike:   eventNameLike,
		Module:          cr.Module,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithList(list, int(count), c)
}

// ActionAuditDetailView 操作审计日志详情（管理员）。
// @Summary 操作审计日志详情
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} object{code=int,data=clickhouse.ActionAuditRecord,msg=string}
// @Router /logs/action/{id} [get]
func (h *Handler) ActionAuditDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[IDRequest](c)
	item, err := clickhouse.GetActionAudit(h.logs, uint64(cr.ID))
	if err != nil {
		if err == sql.ErrNoRows {
			res.FailWithMsg("操作审计日志不存在", c)
			return
		}
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(item, c)
}
