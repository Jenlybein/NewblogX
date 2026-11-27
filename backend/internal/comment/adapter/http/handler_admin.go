package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
	// 供 swag 注解里的 app.AdminCommentItem / app.AdminRemoveManyResult 解析用
	//（代码里不直接引用；可读性上它们只是响应的具名形状）。
	_ "myblogx/internal/comment/app"
)

// 本文件是管理端「评论管理」的 HTTP 适配器（后台「文章管理 → 评论管理」第一期）。
//
// 路由整段挂在 adminGroup 上（见 router/comment_router.go），所以这里不再判
// "是不是管理员"——那已经由 AdminMiddleware 做了一次；用例层还会再判一次，
// 但那是为了挡住绕过 HTTP 的内部调用，不是给这里省事。
//
// 三条写动作都要留操作审计（EmitActionAuditFromGin）：删除 / 禁言 / 封禁是
// 事后唯一能回答"谁、什么时候、对谁做了什么"的线索。写接口的请求体由路由层
// 的 mw.CaptureLog(mw.ReqBody) 采集，只在失败请求时落进审计，不额外把原始报文
// 塞进消息里（L.10 决策：原始报文默认不入库）。

// AdminList 管理端评论列表（含已删除）。
//
// 筛选参数非法时返回明确错误而不是静默退化成"不筛选"：后台拼错一个状态值
// 就该看到报错，而不是拿到一份看起来正常、其实只是全量数据的列表。
// @Summary 管理端评论列表（管理员）
// @Tags comment
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码（默认 1）"
// @Param limit query int false "每页条数（默认 20，上限 100）"
// @Param keyword query string false "评论内容关键词（子串）"
// @Param user_id query string false "评论发布人 ID"
// @Param user_keyword query string false "评论发布人昵称关键词（子串）"
// @Param article_id query string false "文章 ID"
// @Param from query string false "起始日期（YYYY-MM-DD，按 created_at）"
// @Param to query string false "结束日期（YYYY-MM-DD，含当天，按 created_at）"
// @Param status query string false "状态筛选" Enums(all,published,deleted)
// @Success 200 {object} kernel.Envelope[kernel.HasMoreListData[app.AdminCommentItem]]
// @Router /comments/admin/list [get]
func (h *Handler) AdminList(c *gin.Context) {
	var req AdminCommentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	status, err := domain.ParseAdminStatus(req.Status)
	if err != nil {
		res.Fail(err, c)
		return
	}
	from, err := adminDayBoundary(req.From, false)
	if err != nil {
		res.Fail(err, c)
		return
	}
	to, err := adminDayBoundary(req.To, true)
	if err != nil {
		res.Fail(err, c)
		return
	}

	items, hasMore, err := h.svc.AdminList(
		c.Request.Context(),
		viewer(c),
		domain.AdminFilter{
			Keyword:     req.Keyword,
			UserID:      req.UserID,
			UserKeyword: req.UserKeyword,
			ArticleID:   req.ArticleID,
			FromAt:      from,
			ToAt:        to,
			Status:      status,
		},
		domain.AdminPage{Page: req.Page, Limit: req.Limit},
	)
	if err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithData(map[string]any{"list": items, "has_more": hasMore}, c)
}

// AdminRemoveMany 批量删除评论（管理员）。
//
// 语义是**逐条结果**（见 app.Service.AdminRemoveMany 的注释）：请求级非法
// （一条没选 / 超过 100 条 / 混了空 ID）整批拒绝；通过之后逐条走与读者侧
// 删除完全相同的链路，成功的已生效、失败的如实回传，不回滚已删掉的部分。
// @Summary 批量删除评论（管理员）
// @Tags comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body AdminRemoveRequest true "要删除的评论 ID（1–100 条）"
// @Success 200 {object} kernel.Envelope[app.AdminRemoveManyResult]
// @Router /comments/admin [delete]
func (h *Handler) AdminRemoveMany(c *gin.Context) {
	var req AdminRemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	result, err := h.svc.AdminRemoveMany(c.Request.Context(), viewer(c), req.IDList)
	if err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithData(result, c)
	// 有失败条目时 Success 记 false：审计要能回答"这次批量删除到底干净不干净"。
	// 目标 ID 一并记下，否则事后只能看到"删了若干条"，对不上具体评论。
	emitAdminAudit(c, "comment_delete_admin", "comment", joinAdminIDs(req.IDList), len(result.Failed) == 0,
		fmt.Sprintf("批量删除评论 %d 条：成功 %d 条、失败 %d 条",
			len(req.IDList), result.Succeeded, len(result.Failed)))
}

// AdminMute 禁言评论发布人（管理员）。
//
// days 的档位与理由校验都在用例层（domain.NormalizeAdminMuteDays /
// NormalizeAdminReason）：四档之外的数字、空理由都要报参数错误，而不是
// 悄悄按默认值处理 —— "设了 3 天却被按 7 天执行"比直接报错更难发现。
// @Summary 禁言评论发布人（管理员）
// @Tags comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body AdminMuteRequest true "用户 ID、禁言天数与理由"
// @Success 200 {object} kernel.Envelope[AdminMuteResponse]
// @Router /comments/admin/mute [post]
func (h *Handler) AdminMute(c *gin.Context) {
	var req AdminMuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	until, err := h.svc.AdminMute(c.Request.Context(), viewer(c), req.UserID, req.Days, req.Reason)
	if err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithData(AdminMuteResponse{MutedUntil: until.Format(time.RFC3339)}, c)
	// 目标是**用户**（被禁言的人），不是评论：审计按用户查时才找得到这次处置。
	emitAdminAudit(c, "comment_mute_user", "user", req.UserID.String(), true,
		fmt.Sprintf("禁言评论发布人: user=%s days=%d until=%s reason=%s",
			req.UserID.String(), req.Days, until.Format(time.RFC3339), strings.TrimSpace(req.Reason)))
}

// AdminBan 封禁评论发布人（管理员）。
//
// 封禁是账号级终态（status=3），走 user 模块的 AdminUpdate 同一口径：令牌版本
// +1、吊销全部会话 —— 只是"改一列"的话，被封的人手里那张未过期的 access token
// 还能继续发评论。
// @Summary 封禁评论发布人（管理员）
// @Tags comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body AdminBanRequest true "用户 ID 与可选理由"
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /comments/admin/ban [post]
func (h *Handler) AdminBan(c *gin.Context) {
	var req AdminBanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	if err := h.svc.AdminBan(c.Request.Context(), viewer(c), req.UserID, req.Reason); err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithMsg("封禁成功", c)
	emitAdminAudit(c, "comment_ban_user", "user", req.UserID.String(), true,
		fmt.Sprintf("封禁评论发布人: user=%s reason=%s", req.UserID.String(), strings.TrimSpace(req.Reason)))
}

// adminDayBoundary 把日期字符串解析成 created_at 的边界；空串表示"不限"。
//
// end=true 时取"次日零点"，配合 SQL 的 `created_at < ?` 实现"含当天"：
// 用"当日 23:59:59.999"当上界会在 millisecond 精度下漏掉最后一毫秒的评论。
// 除了 YYYY-MM-DD 也接受 RFC3339：后台偶尔会直接贴一个带时刻的值。
func adminDayBoundary(raw string, end bool) (*time.Time, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	day, err := time.ParseInLocation("2006-01-02", text, time.Local)
	if err != nil {
		at, rfcErr := time.Parse(time.RFC3339, text)
		if rfcErr != nil {
			return nil, apperr.New(apperr.CodeCommonInvalidArgument)
		}
		day = at
	}
	if end {
		day = day.AddDate(0, 0, 1)
	}
	return &day, nil
}

// joinAdminIDs 把一批评论 ID 拼成审计里的目标 ID。
//
// 批量删除没有单一目标，全部列出来才能事后对上"到底删了哪几条"；
// 条数上限是 100（domain.MaxAdminBatchIDs），拼出来的长度是可预期的。
func joinAdminIDs(ids []ctype.ID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return strings.Join(parts, ",")
}

// emitAdminAudit 写一条管理端操作审计。
//
// 包一层是为了让三个 handler 的审计字段口径一致（动作名、目标类型、成功位、
// 消息），也避免每处都重复 GinAuditInput 的字段拼装。
// TargetType 必须跟着 TargetID 的实体走：删除记 "comment"、禁言 / 封禁记 "user"
// —— 两者混用会让"按用户查处置记录"时漏掉这些动作。
func emitAdminAudit(c *gin.Context, action, targetType, targetID string, success bool, message string) {
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: action,
		TargetType: targetType,
		TargetID:   targetID,
		Success:    success,
		Message:    message,
	})
}
