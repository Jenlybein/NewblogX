package http

// 死信（DLQ）列表与重投接口。
//
// 背景：三个 CDC 子链路重试耗尽后会把失败证据写进 cdc_dead_letter，但以前没有任何
// 入口去看它、更没法照它补一次。这两个接口补齐"看得见 + 补得上"：
//
//	GET  /logs/dead-letter         列出待处理/已解决的死信
//	POST /logs/dead-letter/replay  勾选若干条重投，逐条返回结果
//
// 重投的语义与支持范围见 docs/architecture/adr/ADR-008-死信重投闭环.md。

import (
	"fmt"
	"time"

	"myblogx/internal/analytics/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/page"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// maxReplayBatch 是单次重投的条数上限。
const maxReplayBatch = 100

// DeadLetterListRequest 是死信列表的查询参数。
type DeadLetterListRequest struct {
	page.PageInfo
	Stream string `form:"stream"`
	Status string `form:"status"`
	// CdcJobID 支持前缀匹配（运维手里通常只有一段任务 ID）。
	CdcJobID string `form:"cdc_job_id"`
}

// DeadLetterItemResponse 是列表里的一条死信。
type DeadLetterItemResponse struct {
	ID          ctype.ID  `json:"id"`
	Stream      string    `json:"stream"`
	CdcJobID    string    `json:"cdc_job_id"`
	SourceTable string    `json:"source_table"`
	Action      string    `json:"action"`
	TargetKey   string    `json:"target_key"`
	RetryCount  int       `json:"retry_count"`
	Status      string    `json:"status"`
	ErrorCode   string    `json:"error_code"`
	ErrorMsg    string    `json:"error_msg"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DeadLetterReplayRequest 是重投请求体。
type DeadLetterReplayRequest struct {
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

// DeadLetterReplayResultResponse 是单条重投结果。
type DeadLetterReplayResultResponse struct {
	ID      ctype.ID `json:"id"`
	Stream  string   `json:"stream"`
	Status  string   `json:"status"`
	Result  string   `json:"result"`
	Message string   `json:"message"`
}

// DeadLetterListView 死信列表（管理员）。
// @Summary 死信列表
// @Tags log
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param limit query int false "每页条数"
// @Param stream query string false "Stream 名称"
// @Param status query string false "状态：pending 待处理 / resolved 已解决"
// @Param cdc_job_id query string false "CDC 任务 ID（前缀匹配）"
// @Success 200 {object} kernel.Envelope[kernel.ListData[DeadLetterItemResponse]]
// @Router /logs/dead-letter [get]
func (h *Handler) DeadLetterListView(c *gin.Context) {
	cr := middleware.GetBindQuery[DeadLetterListRequest](c)
	if h.deadLetters == nil {
		res.FailWithMsg("死信查询未初始化", c)
		return
	}

	query := deadLetterQuery(cr)
	total, err := h.deadLetters.Count(c.Request.Context(), query)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	// 偏移由 GetOffset 统一算：它会顺手把 page=9999 夹到最后一页。
	query.Offset = cr.GetOffset(int(total))
	query.Limit = cr.GetLimit()

	list, err := h.deadLetters.Query(c.Request.Context(), query)
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	items := make([]DeadLetterItemResponse, 0, len(list))
	for _, item := range list {
		items = append(items, DeadLetterItemResponse{
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
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	res.OkWithList(items, int(total), c)
}

// DeadLetterReplayView 重投死信（管理员）。
//
// 逐条返回结果：一条失败不影响同批其他记录。result 取值
// success / failed / skipped / unsupported 与回放日志的 result 列同一套词。
// @Summary 重投死信
// @Tags log
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body DeadLetterReplayRequest true "死信 ID 列表"
// @Success 200 {object} kernel.Envelope[kernel.ListData[DeadLetterReplayResultResponse]]
// @Router /logs/dead-letter/replay [post]
func (h *Handler) DeadLetterReplayView(c *gin.Context) {
	cr := middleware.GetBindJson[DeadLetterReplayRequest](c)
	if h.replayer == nil {
		res.FailWithMsg("死信重投未启用（未注册任何重投执行器）", c)
		return
	}
	// 一次一批的硬上限：重投是同步逐个执行的，一批一千条会把 HTTP 请求挂在那儿，
	// 而且在 ES 抖动时会放大故障。管理端一页 10 条，正常用法远低于这个数。
	if len(cr.IDList) > maxReplayBatch {
		res.FailWithMsg(fmt.Sprintf("一次最多重投 %d 条，请分批操作", maxReplayBatch), c)
		return
	}

	results, err := h.replayer.Replay(c.Request.Context(), cr.IDList)
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	items := make([]DeadLetterReplayResultResponse, 0, len(results))
	for _, item := range results {
		items = append(items, DeadLetterReplayResultResponse{
			ID:      item.ID,
			Stream:  item.Stream,
			Status:  item.Status,
			Result:  item.Result,
			Message: item.Message,
		})
	}
	res.OkWithList(items, len(items), c)
}

// deadLetterQuery 把请求参数翻译成领域查询（分页参数由调用方补，因为 Count 用不到）。
func deadLetterQuery(cr DeadLetterListRequest) domain.DeadLetterQuery {
	q := domain.DeadLetterQuery{
		Stream:   cr.Stream,
		Status:   cr.Status,
		CdcJobID: cr.CdcJobID,
	}
	if cr.CdcJobID != "" {
		// 运维手里通常只有一段任务 ID，按前缀匹配更实用。
		q.CdcJobIDPrefix = true
	}
	return q
}
