// Package http 是媒体模块的 HTTP 适配器。
package http

import (
	"bytes"
	"encoding/json"
	"io"
	"myblogx/internal/kernel/ctype"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/media/app"
	"myblogx/internal/media/domain"
	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 是媒体 HTTP 处理器。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("media/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// CreateTaskRequest 创建上传任务。
type CreateTaskRequest struct {
	FileName string `json:"file_name" binding:"required,max=255"`
	Size     int64  `json:"size" binding:"required"`
	MimeType string `json:"mime_type" binding:"required,max=128"`
	Hash     string `json:"hash" binding:"required,max=64"`
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
	Status      string   `json:"status"`
	URL         string   `json:"url"`
	Hash        string   `json:"hash"`
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
	Status   string   `json:"status"`
	URL      string   `json:"url"`
	ErrorMsg string   `json:"error_msg,omitempty"`
}

// TaskStatusResponse 任务状态。
type TaskStatusResponse struct {
	UploadID ctype.ID `json:"upload_id"`
	ImageID  ctype.ID `json:"image_id"`
	Status   string   `json:"status"`
	URL      string   `json:"url"`
	ErrorMsg string   `json:"error_msg,omitempty"`
	Hash     string   `json:"hash"`
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
}

// ListRequest 图片列表查询。
type ListRequest struct {
	Page  int    `form:"page"`
	Limit int    `form:"limit"`
	Key   string `form:"key"`
}

// RemoveRequest 批量删除。
type RemoveRequest struct {
	IDList []uint64 `json:"id_list" binding:"required"`
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
	result, err := h.svc.CreateTask(c.Request.Context(), userID, req.FileName, req.Size, req.MimeType, req.Hash)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	if result.SkipUpload && result.Image != nil {
		res.OkWithData(CreateTaskResponse{
			SkipUpload: true,
			ImageID:    result.Image.ID,
			Status:     result.Image.Status.String(),
			URL:        result.Image.URL,
			Hash:       result.Image.Hash,
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
		UploadID: result.Task.ID,
		ImageID:  result.Image.ID,
		Status:   string(result.Task.Status),
		URL:      result.Image.URL,
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
		resp.URL = result.Image.URL
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
		UploadID: result.Task.ID,
		ImageID:  result.Image.ID,
		Status:   string(result.Task.Status),
		URL:      result.Image.URL,
		ErrorMsg: result.Task.ErrorMsg,
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
	images, total, err := h.svc.List(c.Request.Context(), domain.Query{Page: req.Page, Limit: req.Limit, Key: req.Key})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	items := make([]ImageItem, 0, len(images))
	for _, img := range images {
		items = append(items, ImageItem{
			ID:        img.ID,
			CreatedAt: img.CreatedAt,
			UserID:    img.UserID,
			Provider:  string(img.Provider),
			Bucket:    img.Bucket,
			ObjectKey: img.ObjectKey,
			FileName:  img.FileName,
			URL:       img.URL,
			MimeType:  img.MimeType,
			Size:      img.Size,
			Width:     img.Width,
			Height:    img.Height,
			Hash:      img.Hash,
			Status:    uint(img.Status),
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
	ids := make([]domain.ImageID, 0, len(req.IDList))
	for _, id := range req.IDList {
		ids = append(ids, ctype.NewID(id))
	}
	count, err := h.svc.Remove(c.Request.Context(), ids)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(sprintfDelete(count), c)
}
