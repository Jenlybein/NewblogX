// Package http 是媒体模块的 HTTP 适配器。
package http

import (
	"bytes"
	"encoding/json"
	"io"
	"myblogx/internal/kernel/ctype"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/imagedisplay"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 是媒体 HTTP 处理器。
type Handler struct {
	svc *app.Service
	// display 是图片展示派生器（ADR-0034）。为 nil 时原样返回库里的地址，
	// 只在本模块被单独装配（测试）时出现。
	display *imagedisplay.Resolver
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service, display *imagedisplay.Resolver) *Handler {
	if svc == nil {
		panic("media/http: 用例服务未注入")
	}
	return &Handler{svc: svc, display: display}
}

// uploadVisibleURL 上传链路返回地址前过闸门。
//
// 口径与全站一致：只有「审核通过」才给地址。上传者本人也不给 —— 规格里明确
// 未过审的图对作者同样不可见（作者看到的是「审核中」占位），这样"作者能用"
// 与"读者能看到"永远不是两套规则。
func uploadVisibleURL(status domain.Status, raw string) string {
	if !status.Displayable() {
		return ""
	}
	return raw
}

// displayURL 派生单条图片地址（后台图片列表用）。
//
// 后台看的是"审核面"，所以按管理员身份派生：未过审的图也要能预览，
// 否则管理员没法判断该放行还是驳回。
func (h *Handler) displayURL(c *gin.Context, purpose imagedisplay.Purpose, raw string) string {
	if raw == "" {
		return ""
	}
	out := imagedisplay.MustResolve(c.Request.Context(), h.display,
		imagedisplay.Viewer{IsAdmin: true},
		[]imagedisplay.Item{{URL: raw, Purpose: purpose}})
	return out[0]
}

// CreateTaskRequest 创建上传任务。
type CreateTaskRequest struct {
	FileName string `json:"file_name" binding:"required,max=255"`
	Size     int64  `json:"size" binding:"required"`
	MimeType string `json:"mime_type" binding:"required,max=128"`
	Hash     string `json:"hash" binding:"required,max=64"`
	// Purpose 声明用途：cover（16:9）/ avatar（1:1）。nil 表示请求体不带
	// 该字段（例如私信发图），确认时不校验比例；一旦出现（含空串）必须合法，
	// 非法值由用例层按明确错误码拒绝，不静默当作 cover。
	// 刻意不用 binding:"oneof"：那样非法值会被 gin 挡在适配器里，只回一句
	// 通用「参数错误」，调用方拿不到可判分支的错误码。
	Purpose *string `json:"purpose,omitempty" enums:"cover,avatar"`
}

// CreateTaskResponse 上传任务信息。
type CreateTaskResponse struct {
	SkipUpload  bool     `json:"skip_upload"`
	UploadID    ctype.ID `json:"upload_id"`
	Provider    string   `json:"provider"`
	Bucket      string   `json:"bucket"`
	ObjectKey   string   `json:"object_key"`
	UploadToken string   `json:"upload_token"`
	Region      string   `json:"region"`
	ExpireAt    string   `json:"expire_at"`
	MaxSize     int64    `json:"max_size"`
	ImageID     ctype.ID `json:"image_id"`
	// Status 是**任务**状态（pending / ready / failed）；秒传命中时任务即刻 ready。
	Status string `json:"status"`
	URL    string `json:"url"`
	Hash   string `json:"hash"`
	// AuditStatus 是**图片审核**状态（pending / pass / review / block …）。
	//
	// 秒传分支必须带上：命中的是"这张图以前传过"，它可能早已被封禁或待审核，
	// 而那时 URL 是空的。只有任务状态的话，客户端拿不到"为什么没有地址"，
	// 只能报一句"未返回 URL"（2026-11-27 线上就是这样漏的）。
	AuditStatus string `json:"audit_status" enums:"pending,pass,review,block,deleted,orphaned,unknown"`
}

// CompleteTaskRequest 手动完成上传。
type CompleteTaskRequest struct {
	UploadID  ctype.ID `json:"upload_id" binding:"required"`
	ObjectKey string   `json:"object_key" binding:"required"`
}

// CompleteTaskResponse 完成结果。
type CompleteTaskResponse struct {
	UploadID ctype.ID `json:"upload_id"`
	ImageID  ctype.ID `json:"image_id"`
	// Status 是**任务**状态（pending / ready / failed）。
	Status string `json:"status"`
	// URL 是可用展示地址：只有审核通过时才有值，否则为空（前端画「审核中」占位）。
	URL string `json:"url"`
	// AuditStatus 是**图片审核**状态（pending / pass / review / block …），
	// 与 Status 是两件事：任务是"文件传完了没"，审核是"这张图能不能给人看"。
	AuditStatus string `json:"audit_status" enums:"pending,pass,review,block,deleted,orphaned,unknown"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}

// TaskStatusResponse 任务状态。
type TaskStatusResponse struct {
	UploadID ctype.ID `json:"upload_id"`
	ImageID  ctype.ID `json:"image_id"`
	Status   string   `json:"status"`
	// URL 同 CompleteTaskResponse：只有审核通过才有值。
	URL string `json:"url"`
	// AuditStatus 是图片审核状态，前端据此显示「审核中 / 已通过 / 已屏蔽」。
	AuditStatus string `json:"audit_status" enums:"pending,pass,review,block,deleted,orphaned,unknown"`
	ErrorMsg    string `json:"error_msg,omitempty"`
	Hash        string `json:"hash"`
}

// callbackRequest 七牛上传回调。
type callbackRequest struct {
	Key    string `json:"key"`
	Hash   string `json:"hash"`
	Bucket string `json:"bucket"`
	Fsize  int64  `json:"fsize"`
}

// ImageItem 图片列表项（字段名与旧契约一致）。
type ImageItem struct {
	ID        ctype.ID  `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    ctype.ID  `json:"user_id"`
	Provider  string    `json:"provider"`
	Bucket    string    `json:"bucket"`
	ObjectKey string    `json:"object_key"`
	FileName  string    `json:"file_name"`
	URL       string    `json:"url"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Hash      string    `json:"hash"`
	Status    uint      `json:"status"`
	// AuditStatus 是机读短名（pending / pass / review / block …），前端据此分支。
	AuditStatus string `json:"audit_status" enums:"pending,pass,review,block,deleted,orphaned,unknown"`
	// AuditLabel 是给人看的中文短名（待审核 / 已通过 / 违规 …），后台表格直接显示。
	AuditLabel string `json:"audit_label"`
}

// ListRequest 图片列表查询。
type ListRequest struct {
	Page  int    `form:"page"`
	Limit int    `form:"limit"`
	Key   string `form:"key"`
	// Status 按审核状态筛选：pending / review / pass / block / deleted / orphaned。
	// 空串表示不筛；**支持逗号分隔多值**（待审队列要一次拿 pending,review）。
	Status string `form:"status"`
	// StartAt / EndAt 是创建时间范围（RFC3339 或 YYYY-MM-DD；EndAt 按"当天结束"算）。
	StartAt string `form:"start_at"`
	EndAt   string `form:"end_at"`
}

// StyleProbeRequest 样式探活请求。
type StyleProbeRequest struct {
	// ObjectKey 留空时用图库里最新一张图，省得运营去翻 object key。
	ObjectKey string `json:"object_key,omitempty"`
}

// StyleProbeItem 一条探活结果。
type StyleProbeItem struct {
	Tier      string `json:"tier" enums:"original,cover,avatar,content,chat"`
	StyleName string `json:"style_name"`
	URL       string `json:"url"`
	Status    int    `json:"status"`
	Size      int64  `json:"size"`
	Error     string `json:"error"`
}

// StyleProbeResponse 样式探活结果。
type StyleProbeResponse struct {
	ObjectKey string           `json:"object_key"`
	Items     []StyleProbeItem `json:"items"`
}

// AuditRequest 人工审核一张图。
type AuditRequest struct {
	// Action 是处置动作：approve（放行）/ reject（驳回并删对象）。
	Action string `json:"action" binding:"required" enums:"approve,reject"`
}

// AuditResponse 人工审核结果。
type AuditResponse struct {
	ID          ctype.ID `json:"id"`
	AuditStatus string   `json:"audit_status"`
	AuditLabel  string   `json:"audit_label"`
	URL         string   `json:"url"`
}

// RemoveRequest 批量删除。
type RemoveRequest struct {
	// IDList 用 ctype.ID 而不是 uint64：图片 ID 是雪花 ID（3.8e17 量级），
	// 超过 JS 的 2^53，前端只能按字符串传；写成 []uint64 会让 JSON 绑定直接失败，
	// 而这条错误被前端吞成"参数错误"——2026-11-25 的"删除没反应"就是这么来的。
	// ADR-0007：ID 在协议上一律是字符串。
	IDList []ctype.ID `json:"id_list" binding:"required"`
}

// CreateUploadTask 创建图片直传任务。
// @Summary 创建图片上传任务
// @Tags image
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateTaskRequest true "请求体"
// @Success 200 {object} kernel.Envelope[CreateTaskResponse]
// @Router /images/upload-tasks [post]
func (h *Handler) CreateUploadTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	userID, ok := userIDFrom(c)
	if !ok {
		return
	}
	// 原样转发指针：nil=请求体没带 purpose，非 nil=带了（含空串，由用例层判合法）。
	result, err := h.svc.CreateTask(c.Request.Context(), userID, req.FileName, req.Size, req.MimeType, req.Hash, req.Purpose)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	if result.SkipUpload && result.Image != nil {
		// 秒传命中也要过闸门：库里那张图可能是待审核甚至已封禁的。
		res.OkWithData(CreateTaskResponse{
			SkipUpload: true,
			ImageID:    result.Image.ID,
			// 秒传没有真正建任务：库里那张图已经是终态，任务等价于 ready。
			// Status 固定是任务状态，审核状态走 AuditStatus —— 两者不可互相顶替。
			Status:      string(domain.TaskReady),
			AuditStatus: result.Image.Status.String(),
			URL:         uploadVisibleURL(result.Image.Status, result.Image.URL),
			Hash:        result.Image.Hash,
		}, c)
		return
	}
	if result.Task == nil || result.Token == nil {
		res.FailWithMsg("创建上传任务失败", c)
		return
	}
	res.OkWithData(CreateTaskResponse{
		SkipUpload:  false,
		UploadID:    result.Task.ID,
		Provider:    string(result.Task.Provider),
		Bucket:      result.Token.Bucket,
		ObjectKey:   result.Token.ObjectKey,
		UploadToken: result.Token.Token,
		Region:      result.Region,
		ExpireAt:    result.Token.ExpireAt.Format("2006-01-02T15:04:05Z07:00"),
		MaxSize:     result.MaxSize,
		Hash:        result.Task.Hash,
	}, c)
}

// CompleteUploadTask 手动完成上传。
// @Summary 完成图片上传任务
// @Tags image
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CompleteTaskRequest true "请求体"
// @Success 200 {object} kernel.Envelope[CompleteTaskResponse]
// @Router /images/upload-tasks/complete [post]
func (h *Handler) CompleteUploadTask(c *gin.Context) {
	var req CompleteTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	userID, ok := userIDFrom(c)
	if !ok {
		return
	}
	result, err := h.svc.ConfirmByUser(c.Request.Context(), req.UploadID.Uint64(), userID, req.ObjectKey)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(CompleteTaskResponse{
		UploadID:    result.Task.ID,
		ImageID:     result.Image.ID,
		Status:      string(result.Task.Status),
		URL:         uploadVisibleURL(result.Image.Status, result.Image.URL),
		AuditStatus: result.Image.Status.String(),
	}, c)
}

// UploadTaskStatus 查询任务状态。
// @Summary 查询上传任务状态
// @Tags image
// @Produce json
// @Security BearerAuth
// @Param id path string true "上传任务 ID"
// @Success 200 {object} kernel.Envelope[TaskStatusResponse]
// @Router /images/upload-tasks/{id} [get]
func (h *Handler) UploadTaskStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	userID, ok := userIDFrom(c)
	if !ok {
		return
	}
	result, err := h.svc.GetStatus(c.Request.Context(), id, userID)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	resp := TaskStatusResponse{
		UploadID: result.Task.ID,
		Status:   string(result.Task.Status),
		ErrorMsg: result.Task.ErrorMsg,
		Hash:     result.Task.Hash,
	}
	if result.Image != nil {
		resp.ImageID = result.Image.ID
		resp.URL = uploadVisibleURL(result.Image.Status, result.Image.URL)
		resp.AuditStatus = result.Image.Status.String()
	}
	res.OkWithData(resp, c)
}

// QiniuCallback 七牛上传回调。
// @Summary 七牛云上传回调
// @Tags image
// @Accept json
// @Produce json
// @Success 200 {object} kernel.Envelope[CompleteTaskResponse]
// @Router /images/qiniu/callback [post]
func (h *Handler) QiniuCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	ok, err := h.svc.VerifyCallback(c.Request)
	if err != nil || !ok {
		res.FailWithError(err, c)
		return
	}

	var payload callbackRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		res.FailWithError(err, c)
		return
	}
	if payload.Key == "" {
		res.FailWithError(apperrObjectKeyMissing(), c)
		return
	}

	result, err := h.svc.ConfirmByCallback(c.Request.Context(), payload.Key, payload.Bucket, payload.Hash, payload.Fsize)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(CompleteTaskResponse{
		UploadID:    result.Task.ID,
		ImageID:     result.Image.ID,
		Status:      string(result.Task.Status),
		URL:         uploadVisibleURL(result.Image.Status, result.Image.URL),
		AuditStatus: result.Image.Status.String(),
		ErrorMsg:    result.Task.ErrorMsg,
	}, c)
}

// QiniuAuditCallback 七牛内容审核回调。
// @Summary 七牛内容审核回调
// @Tags image
// @Accept json
// @Produce json
// @Success 200 {object} kernel.Envelope[kernel.EmptyData]
// @Router /images/qiniu/audit/callback [post]
func (h *Handler) QiniuAuditCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	ok, err := h.svc.VerifyCallback(c.Request)
	if err != nil || !ok {
		res.FailWithError(err, c)
		return
	}
	if err := h.svc.HandleAuditCallback(c.Request.Context(), body); err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithMsg("七牛审核回调处理成功", c)
}

// Audit 人工审核一张图（管理员）。
//
// 只对"待审核 / 疑似"有意义：放行会把状态推到通过并在存储侧解禁，
// 驳回会把状态推到违规（终局）并按开关删除对象。违规图不允许放行 ——
// 对象已经删了，"看起来放行了但打不开"比直接拒绝更糟。
// @Summary 图片人工审核
// @Tags image
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "图片 ID"
// @Param body body AuditRequest true "审核动作"
// @Success 200 {object} kernel.Envelope[AuditResponse]
// @Router /images/{id}/audit [post]
func (h *Handler) Audit(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req AuditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}

	var (
		image *domain.Image
		err   error
	)
	switch strings.TrimSpace(strings.ToLower(req.Action)) {
	case "approve":
		image, err = h.svc.ApproveAudit(c.Request.Context(), domain.ImageID(ctype.NewID(id)))
	case "reject":
		image, err = h.svc.RejectAudit(c.Request.Context(), domain.ImageID(ctype.NewID(id)))
	default:
		res.FailWithMsg("审核动作不存在（可选：approve / reject）", c)
		return
	}
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(AuditResponse{
		ID:          image.ID,
		AuditStatus: image.Status.String(),
		AuditLabel:  image.Status.Label(),
		URL:         h.displayURL(c, imagedisplay.PurposeCover, image.URL),
	}, c)
}

// ProbeStyles 图片样式探活（管理员）。
//
// 为什么需要这个端点：七牛对"样式名不存在"的响应是 404，**不会回退原图**。
// 后台把样式名填错一个字，站内那一档的图片会全部裂掉，而七牛不会报任何错。
// 这个接口对同一张图分别请求原图与四档样式，把状态码与字节数并排返回，
// 保存后点一下就能看出配置对不对。
// @Summary 图片样式探活
// @Tags image
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body StyleProbeRequest false "探活参数"
// @Success 200 {object} kernel.Envelope[StyleProbeResponse]
// @Router /images/style-probe [post]
func (h *Handler) ProbeStyles(c *gin.Context) {
	var req StyleProbeRequest
	// 允许空请求体：运营想直接探"最新一张图"时不该被迫构造 body。
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			res.FailWithMsg("参数错误", c)
			return
		}
	}
	report, err := h.svc.ProbeImageStyles(c.Request.Context(), req.ObjectKey)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	items := make([]StyleProbeItem, 0, len(report.Items))
	for _, row := range report.Items {
		items = append(items, StyleProbeItem{
			Tier:      row.Tier,
			StyleName: row.StyleName,
			URL:       row.URL,
			Status:    row.Status,
			Size:      row.Size,
			Error:     row.Error,
		})
	}
	res.OkWithData(StyleProbeResponse{ObjectKey: report.ObjectKey, Items: items}, c)
}

// List 图片列表（管理员）。
// @Summary 图片列表
// @Tags image
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[ImageItem]]
// @Router /images [get]
func (h *Handler) List(c *gin.Context) {
	var req ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	query := domain.Query{Page: req.Page, Limit: req.Limit, Key: req.Key}
	for _, raw := range strings.Split(req.Status, ",") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		status := domain.StatusFromSuggestion(trimmed)
		if status == domain.StatusUnknown {
			res.FailWithMsg("状态筛选值不存在（可选：pending / review / pass / block / deleted / orphaned）", c)
			return
		}
		query.Statuses = append(query.Statuses, status)
	}
	if start, ok := parseDayBoundary(req.StartAt, false); ok {
		query.StartAt = &start
	}
	if end, ok := parseDayBoundary(req.EndAt, true); ok {
		query.EndAt = &end
	}

	images, total, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	items := make([]ImageItem, 0, len(images))
	for _, img := range images {
		items = append(items, ImageItem{
			ID:          img.ID,
			CreatedAt:   img.CreatedAt,
			UserID:      img.UserID,
			Provider:    string(img.Provider),
			Bucket:      img.Bucket,
			ObjectKey:   img.ObjectKey,
			FileName:    img.FileName,
			URL:         h.displayURL(c, imagedisplay.PurposeCover, img.URL),
			MimeType:    img.MimeType,
			Size:        img.Size,
			Width:       img.Width,
			Height:      img.Height,
			Hash:        img.Hash,
			Status:      uint(img.Status),
			AuditStatus: img.Status.String(),
			AuditLabel:  img.Status.Label(),
		})
	}
	res.OkWithList(items, int(total), c)
}

// Remove 批量删除图片（管理员）。
// @Summary 删除图片
// @Tags image
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body RemoveRequest true "图片 ID 列表"
// @Success 200 {object} kernel.Envelope[string]
// @Router /images [delete]
func (h *Handler) Remove(c *gin.Context) {
	var req RemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}
	count, err := h.svc.Remove(c.Request.Context(), req.IDList)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(sprintfDelete(count), c)
}

// parseDayBoundary 把后台传的日期解析成时间边界。
//
// 接受两种写法：`YYYY-MM-DD`（按天）与 RFC3339（按时刻）。`end=true` 表示这是
// 右边界，按**次日零点**算 —— 否则选"11-25"会漏掉 11-25 当天的图，运营会以为
// 那天的图丢了。解析失败返回 false（当作没传，不报错：日期是可选筛选项）。
func parseDayBoundary(raw string, end bool) (time.Time, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}, false
	}
	if day, err := time.ParseInLocation("2006-01-02", text, time.Local); err == nil {
		if end {
			return day.AddDate(0, 0, 1), true
		}
		return day, true
	}
	if at, err := time.Parse(time.RFC3339, text); err == nil {
		return at, true
	}
	return time.Time{}, false
}
