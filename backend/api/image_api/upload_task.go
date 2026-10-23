package image_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/image_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// CreateUploadTaskView 创建图片直传上传任务
// 前端调用：获取上传凭证、判断是否需要上传（秒传）
func (h ImageApi) CreateUploadTaskView(c *gin.Context) {
	app := h.App
	imageDeps := image_service.NewDeps(app.QiNiu, app.Upload, app.DB, app.Redis, app.Logger)
	cr := middleware.GetBindJson[CreateImageUploadTaskRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	// 调用服务层创建上传任务
	result, err := image_service.CreateUploadTask(imageDeps, claims.UserID, cr.FileName, cr.Size, cr.MimeType, cr.Hash)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	// 文件已存在，直接返回图片信息，无需上传
	if result.SkipUpload && result.Image != nil {
		res.OkWithData(CreateImageUploadTaskResponse{
			SkipUpload: true,
			ImageID:    result.Image.ID,
			Status:     result.Image.Status.String(),
			URL:        result.Image.URL,
			Hash:       result.Image.Hash,
		}, c)
		return
	}

	// 正常上传：返回上传任务信息 + 七牛云上传凭证
	res.OkWithData(CreateImageUploadTaskResponse{
		SkipUpload:  false,
		UploadID:    result.Task.ID,                                  // 上传任务ID
		Provider:    string(result.Task.Provider),                    // 存储服务商（七牛）
		Bucket:      result.UploadInfo.Bucket,                        // 七牛存储空间
		ObjectKey:   result.UploadInfo.ObjectKey,                     // 文件存储key
		UploadToken: result.UploadInfo.Token,                         // 七牛上传凭证
		Region:      strings.TrimSpace(app.QiNiu.Region),             // 存储区域
		ExpireAt:    result.UploadInfo.ExpireAt.Format(time.RFC3339), // 凭证过期时间
		MaxSize:     int64(app.QiNiu.Size) * 1024 * 1024,             // 最大上传大小
		Hash:        result.Task.Hash,                                // 文件哈希值
	}, c)
}

// CompleteUploadTaskView 手动完成上传任务
// 作用：前端上传完成后，主动通知后端确认文件上传成功
// 备注：正式环境优先使用七牛回调，此接口用于本地调试/兜底
func (h ImageApi) CompleteUploadTaskView(c *gin.Context) {
	imageDeps := image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger)
	cr := middleware.GetBindJson[CompleteImageUploadTaskRequest](c)

	claims := jwts.MustGetClaimsByGin(c)

	// 调用服务层确认上传任务完成
	result, err := image_service.ConfirmUploadTaskByUser(imageDeps, cr.UploadID, claims.UserID, cr.ObjectKey)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	// 返回任务完成结果（图片信息）
	res.OkWithData(CompleteImageUploadTaskResponse{
		UploadID: result.Task.ID,
		ImageID:  result.Image.ID,
		Status:   string(result.Task.Status),
		URL:      result.Image.URL,
	}, c)
}

// UploadTaskStatusView 查询上传任务状态
// 前端轮询使用：实时获取任务是否完成、失败、成功
func (h ImageApi) UploadTaskStatusView(c *gin.Context) {
	imageDeps := image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger)
	claims := jwts.MustGetClaimsByGin(c)
	cr := middleware.GetBindUri[models.IDRequest](c)

	// 查询任务状态
	result, err := image_service.GetUploadTaskStatusByUser(imageDeps, cr.ID, claims.UserID)
	if err != nil {
		// 任务不存在
		if errors.Is(err, image_service.ErrUploadTaskNotFound) {
			res.FailWithMsg("上传任务不存在", c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}

	// 组装响应数据
	resp := UploadTaskStatusResponse{
		UploadID: result.Task.ID,
		Status:   string(result.Task.Status),
		ErrorMsg: result.Task.ErrorMsg,
		Hash:     result.Task.Hash,
	}
	// 图片生成成功则返回图片ID和访问地址
	if result.Image != nil {
		resp.ImageID = result.Image.ID
		resp.URL = result.Image.URL
	}
	res.OkWithData(resp, c)
}

// QiniuCallbackView 七牛云上传成功回调接口
// 功能：七牛云文件上传完成后，自动回调该接口，后端完成任务确认
// 配置：七牛后台填写回调地址：https://你的后端域名/api/images/qiniu/callback
func (h ImageApi) QiniuCallbackView(c *gin.Context) {
	// 记录并重置请求体（用于后续重复读取）
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	// 校验七牛回调签名合法性（防止伪造请求）
	ok, err := image_service.VerifyQiniuCallback(image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger), c.Request)
	if err != nil || !ok {
		res.FailWithMsg(fmt.Sprintf("校验七牛回调失败: %v", err), c)
		return
	}

	// 再次重置请求体，解析回调JSON数据
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var payload qiniuCallbackRequest
	if err = json.Unmarshal(body, &payload); err != nil {
		res.FailWithError(err, c)
		return
	}
	// 校验回调必须携带文件存储key
	if payload.Key == "" {
		res.FailWithMsg("七牛回调缺少对象 key", c)
		return
	}

	// 根据文件key自动完成上传任务
	result, err := image_service.ConfirmUploadTaskByCallback(image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger), payload.Key, payload.Bucket, payload.Hash, payload.Fsize)
	if err != nil {
		if errors.Is(err, image_service.ErrUploadTaskNotFound) {
			res.FailWithMsg("上传任务不存在", c)
			return
		}
		res.FailWithMsg(err.Error(), c)
		return
	}
	// 返回回调处理结果
	res.OkWithData(CompleteImageUploadTaskResponse{
		UploadID: result.Task.ID,
		ImageID:  result.Image.ID,
		Status:   string(result.Task.Status),
		URL:      result.Image.URL,
		ErrorMsg: result.Task.ErrorMsg,
	}, c)
}

// QiniuAuditCallbackView 七牛内容审核回调接口。
// 七牛完成内容审核后回调该接口，服务端根据审核结论更新图片状态。
func (h ImageApi) QiniuAuditCallbackView(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	ok, err := image_service.VerifyQiniuCallback(image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger), c.Request)
	if err != nil || !ok {
		res.FailWithMsg(fmt.Sprintf("校验七牛审核回调失败: %v", err), c)
		return
	}

	if err = image_service.HandleQiniuAuditCallback(image_service.NewDeps(h.App.QiNiu, h.App.Upload, h.App.DB, h.App.Redis, h.App.Logger), body); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("七牛审核回调处理成功", c)
}
