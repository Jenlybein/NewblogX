package log_api

import (
	"database/sql"

	"myblogx/common"
	"myblogx/common/res"
	"myblogx/conf"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/log_service"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	Log              conf.Logrus
	System           conf.System
	ClickHouseConfig conf.ClickHouse
	Logger           *logrus.Logger
	ClickHouse       *sql.DB
	DB               *gorm.DB
}

type LogApi struct {
	App Deps
}

func New(deps Deps) LogApi {
	return LogApi{App: deps}
}

type RuntimeLogListRequest struct {
	common.PageInfo
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
	common.PageInfo
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
	common.PageInfo
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

func (h *LogApi) RuntimeLogListView(c *gin.Context) {
	cr := middleware.GetBindQuery[RuntimeLogListRequest](c)
	list, count, err := log_service.ListRuntimeLogs(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), log_service.RuntimeLogQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
			Key:   cr.Key,
		},
		LogTimeRange: log_service.LogTimeRange{
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

func (h *LogApi) RuntimeLogDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	item, err := log_service.GetRuntimeLog(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), uint64(cr.ID))
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

func (h *LogApi) LoginLogListView(c *gin.Context) {
	cr := middleware.GetBindQuery[LoginLogListRequest](c)
	eventNameLike := cr.EventName
	list, count, err := log_service.ListLoginEvents(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), log_service.LoginEventQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: log_service.LogTimeRange{
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

func (h *LogApi) LoginLogDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	item, err := log_service.GetLoginEvent(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), uint64(cr.ID))
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

func (h *LogApi) ActionAuditListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ActionAuditListRequest](c)
	eventNameLike := cr.EventName
	if eventNameLike == "" {
		eventNameLike = cr.EventNameLike
	}
	list, count, err := log_service.ListActionAudits(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), log_service.ActionAuditQuery{
		PageInfo: common.PageInfo{
			Limit: cr.Limit,
			Page:  cr.Page,
		},
		LogTimeRange: log_service.LogTimeRange{
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

func (h *LogApi) ActionAuditDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	item, err := log_service.GetActionAudit(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), uint64(cr.ID))
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
