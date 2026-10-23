package image_service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/db_service"

	"gorm.io/gorm"
)

var (
	ErrInvalidUploadConfig = errors.New("七牛上传配置不完整")
	ErrUploadTaskNotFound  = errors.New("上传任务不存在")
	ErrUploadTaskFailed    = errors.New("上传任务校验失败")
)

// CreateUploadTask 创建图片上传任务（核心入口）
// 入参：用户ID、文件名、文件大小、MIME类型、文件哈希
// 出参：任务结果（含秒传/上传凭证）、错误
func CreateUploadTask(deps Deps, userID ctype.ID, fileName string, size int64, mimeType string, hash string) (*CreateUploadTaskResult, error) {
	q := deps.QiNiu
	// 校验七牛上传配置
	if !q.Enable || q.Size <= 0 || q.Expiry <= 0 || strings.TrimSpace(q.Bucket) == "" {
		return nil, ErrInvalidUploadConfig
	}

	hash = strings.TrimSpace(hash)
	maxSize := int64(q.Size) * 1024 * 1024
	suffix := GetImageSuffix(fileName)

	if size <= 0 {
		return nil, errors.New("图片大小必须大于 0")
	}
	if hash == "" {
		return nil, errors.New("图片内容哈希不能为空")
	}
	if size > maxSize {
		return nil, fmt.Errorf("图片大小不能超过 %dMB", q.Size)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return nil, errors.New("仅支持图片上传")
	}
	if !containsString(deps.Upload.Whitelist, suffix) {
		return nil, fmt.Errorf("图片后缀 %s 不在服务器允许上传的图片格式白名单中", suffix)
	}

	// 根据文件哈希查询数据库是否已存在相同图片
	var existing models.ImageModel
	if err := deps.DB.Where("hash = ?", hash).Take(&existing).Error; err == nil {
		// 图片已存在，直接返回结果
		return &CreateUploadTaskResult{
			Image:      &existing,
			SkipUpload: true,
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 生成雪花ID作为上传任务ID
	taskID, err := db_service.NextSnowflakeID()
	if err != nil {
		return nil, err
	}

	// 构建七牛对象存储Key（路径+哈希）
	objectKey := buildObjectKey(deps, hash)
	// 计算上传凭证过期时间
	expireAt := time.Now().Add(time.Duration(q.Expiry) * time.Second)
	// 构造上传任务结构体
	task := &ImageUploadTask{
		ID:           taskID,
		UserID:       userID,
		Provider:     enum.ImageProviderQiNiu,
		Status:       enum.ImageUploadTaskPending, // 待上传状态
		Bucket:       q.Bucket,
		ObjectKey:    objectKey,
		OriginalName: fileName,
		DeclaredMime: mimeType,
		DeclaredSize: size,
		Hash:         hash,
		ExpiresAt:    expireAt,
	}

	// 调用七牛SDK生成上传凭证与上传信息
	uploadInfo, err := CreateUploadToken(deps, UploadPolicy{
		Bucket:      q.Bucket,
		ObjectKey:   objectKey,
		CallbackURL: q.CallbackURL, // 七牛回调地址
		ExpireAt:    expireAt,
		MaxSize:     maxSize,
		EndUser:     userID.String(),
	})
	if err != nil {
		return nil, err
	}

	// 保存上传任务到缓存（设置过期时间）
	if err = saveUploadTask(deps, task, taskPendingTTL(expireAt)); err != nil {
		return nil, err
	}
	// 返回任务信息+七牛上传凭证
	return &CreateUploadTaskResult{Task: task, UploadInfo: uploadInfo}, nil
}

// ConfirmUploadTaskByUser 用户手动确认上传完成（前端调用）
// 校验任务归属用户，校验ObjectKey，然后执行任务确认
func ConfirmUploadTaskByUser(deps Deps, taskID, userID ctype.ID, objectKey string) (*ConfirmUploadTaskResult, error) {
	return confirmUploadTask(deps, taskID, nil, func(task *ImageUploadTask) error {
		if task.UserID != userID {
			return ErrUploadTaskNotFound
		}
		if objectKey != "" && task.ObjectKey != objectKey {
			return errors.New("上传对象 key 不匹配")
		}
		return nil
	})
}

// ConfirmUploadTaskByCallback 七牛上传回调确认上传。
// 直接复用七牛回调已返回的对象元信息，避免再额外调用一次 StatObject。
func ConfirmUploadTaskByCallback(deps Deps, objectKey, bucket, hash string, size int64) (*ConfirmUploadTaskResult, error) {
	taskID, err := getUploadTaskIDByObjectKey(deps, objectKey)
	if err != nil {
		return nil, err
	}
	return confirmUploadTask(deps, taskID, &uploadedObjectMeta{
		Bucket: bucket,
		Hash:   hash,
		Size:   size,
	}, func(task *ImageUploadTask) error {
		if task.ObjectKey != objectKey {
			return ErrUploadTaskNotFound
		}
		return nil
	})
}

// GetUploadTaskStatusByUser 查询上传任务状态（前端轮询）
// 校验任务归属，返回任务状态+关联图片信息
func GetUploadTaskStatusByUser(deps Deps, taskID, userID ctype.ID) (*ConfirmUploadTaskResult, error) {
	// 根据ID查询任务
	task, err := getUploadTaskByID(deps, taskID)
	if err != nil {
		return nil, err
	}
	// 校验任务归属
	if task.UserID != userID {
		return nil, ErrUploadTaskNotFound
	}

	// 构造响应结果
	result := &ConfirmUploadTaskResult{Task: task}
	// 如果任务已关联图片ID，查询图片信息并返回
	if task.ImageID != nil {
		if task.ImageURL != "" {
			result.Image = &models.ImageModel{
				Model: models.Model{
					ID: *task.ImageID,
				},
				URL: task.ImageURL,
			}
			return result, nil
		}
		var image models.ImageModel
		if err = deps.DB.Take(&image, "id = ?", *task.ImageID).Error; err == nil {
			result.Image = &image
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return result, nil
}

// confirmUploadTask 统一确认上传任务（核心逻辑）
// 处理任务状态、加锁防并发、校验文件、入库保存。
func confirmUploadTask(deps Deps, taskID ctype.ID, objectMeta *uploadedObjectMeta, validate func(*ImageUploadTask) error) (*ConfirmUploadTaskResult, error) {
	// 分布式锁：防止并发重复确认
	unlock, locked, err := lockUploadTask(deps, taskID, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if locked {
		defer unlock()
	}

	// 加锁后重新查询最新任务状态，防止状态已变更
	task, err := getUploadTaskByID(deps, taskID)
	if err != nil {
		return nil, err
	}
	if validate != nil {
		if err = validate(task); err != nil {
			return nil, err
		}
	}
	// 校验：任务已完成
	if task.Status == enum.ImageUploadTaskReady && task.ImageID != nil {
		var image models.ImageModel
		if err = deps.DB.Take(&image, "id = ?", *task.ImageID).Error; err != nil {
			return nil, err
		}
		return &ConfirmUploadTaskResult{Task: task, Image: &image}, nil
	}

	if task.Status == enum.ImageUploadTaskFailed {
		return nil, ErrUploadTaskFailed
	}
	// 未获取到锁：说明其他请求正在处理，当前请求只能读取状态，不能继续执行确认。
	if !locked {
		return nil, errors.New("上传任务处理中，请稍后重试")
	}

	// 核心：校验七牛上的真实文件
	verified, err := verifyUploadedObject(deps, task, objectMeta)
	if err != nil {
		// 校验失败：标记任务为失败状态
		task.Status = enum.ImageUploadTaskFailed
		task.ErrorMsg = err.Error()
		// 保存失败状态
		if saveErr := saveUploadTask(deps, task, finalizedTaskKeepAlive); saveErr != nil && deps.Logger != nil {
			deps.Logger.Warnf("保存失败的图片上传任务状态失败: 任务ID=%s 错误=%v", task.ID.String(), saveErr)
		}
		return nil, err
	}

	// 事务：保存图片信息+更新任务状态
	result, err := persistConfirmedTask(deps, task, verified)
	if err != nil {
		return nil, err
	}

	// 如果是重复文件，删除七牛上多余的对象
	if verified.ShouldDeleteUpload {
		if delErr := DeleteObject(deps, task.Bucket, task.ObjectKey); delErr != nil && deps.Logger != nil {
			deps.Logger.Warnf("删除重复上传的七牛对象失败: 对象键=%s 错误=%v", task.ObjectKey, delErr)
		}
	}
	return result, nil
}

// verifyUploadedObject 校验七牛云存储的真实文件
// 校验哈希、大小、格式、图片信息，返回校验结果
func verifyUploadedObject(deps Deps, task *ImageUploadTask, objectMeta *uploadedObjectMeta) (*verifiedImage, error) {
	var (
		objectHash string
		objectSize int64
	)
	if objectMeta != nil {
		if objectMeta.Bucket != "" && objectMeta.Bucket != task.Bucket {
			return nil, errors.New("上传对象 bucket 与任务不匹配")
		}
		objectHash = objectMeta.Hash
		objectSize = objectMeta.Size
	}
	if objectHash == "" || objectSize <= 0 {
		fileInfo, err := StatObject(deps, task.Bucket, task.ObjectKey)
		if err != nil {
			return nil, err
		}
		objectHash = fileInfo.Hash
		objectSize = fileInfo.Fsize
	}
	// 校验文件哈希存在
	if objectHash == "" {
		return nil, errors.New("七牛对象缺少哈希信息")
	}
	// 校验哈希与任务一致
	if task.Hash != "" && task.Hash != objectHash {
		return nil, errors.New("上传对象哈希与任务不匹配")
	}
	// 校验文件大小与声明一致
	if task.DeclaredSize > 0 && objectSize != task.DeclaredSize {
		return nil, errors.New("上传对象大小与任务不匹配")
	}

	// 获取图片宽高信息
	imageInfo, err := ImageInfoObject(deps, task.Bucket, task.ObjectKey)
	if err != nil {
		return nil, err
	}
	// 校验图片格式在白名单内
	format := strings.ToLower(strings.TrimSpace(imageInfo.Format))
	if !containsString(deps.Upload.Whitelist, format) {
		return nil, fmt.Errorf("图片后缀 %s 不在服务器允许上传的图片格式白名单中", format)
	}

	// 构造校验通过的图片信息
	return &verifiedImage{
		TaskID:    task.ID,
		UserID:    task.UserID,
		Bucket:    task.Bucket,
		ObjectKey: task.ObjectKey,
		FileName:  task.OriginalName,
		Hash:      objectHash,
		MimeType:  chooseVerifiedMime(task.DeclaredMime, formatToMime(format)),
		Size:      objectSize,
		Width:     imageInfo.Width,
		Height:    imageInfo.Height,
	}, nil
}

// persistConfirmedTask 数据库事务：保存确认后的任务与图片
// 处理秒传、新建图片、更新任务状态
func persistConfirmedTask(deps Deps, task *ImageUploadTask, verified *verifiedImage) (*ConfirmUploadTaskResult, error) {
	var result ConfirmUploadTaskResult

	// 开启数据库事务
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// 根据哈希查询图片
		var image models.ImageModel
		err := tx.Where("hash = ?", verified.Hash).Take(&image).Error
		switch {
		case err == nil:
			// 图片已存在：标记需要删除七牛重复文件
			verified.ShouldDeleteUpload = (image.ObjectKey != verified.ObjectKey)
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 图片不存在：创建新图片记录
			image = models.ImageModel{
				UserID:    verified.UserID,
				Provider:  enum.ImageProviderQiNiu,
				Bucket:    verified.Bucket,
				ObjectKey: verified.ObjectKey,
				FileName:  verified.FileName,
				URL:       ObjectURL(deps, verified.ObjectKey),
				MimeType:  verified.MimeType,
				Size:      verified.Size,
				Width:     verified.Width,
				Height:    verified.Height,
				Hash:      verified.Hash,
				Status:    enum.ImageStatusPass,
			}
			// 创建图片记录
			if err = tx.Create(&image).Error; err != nil {
				// 处理并发重复创建：再次查询
				if !errors.Is(err, gorm.ErrDuplicatedKey) {
					return err
				}
				if err = tx.Where("hash = ?", verified.Hash).Take(&image).Error; err != nil {
					return err
				}
				verified.ShouldDeleteUpload = image.ObjectKey != verified.ObjectKey
			}
		default:
			return err
		}

		// 更新上传任务状态为已完成
		task.Status = enum.ImageUploadTaskReady
		task.VerifiedMime = verified.MimeType
		task.VerifiedSize = verified.Size
		task.Width = verified.Width
		task.Height = verified.Height
		task.Hash = verified.Hash
		task.ConfirmedAt = &now
		task.ImageID = &image.ID
		task.ImageURL = image.URL
		result.Task = task
		result.Image = &image
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 保存完成状态的任务到缓存
	if saveErr := saveUploadTask(deps, task, finalizedTaskKeepAlive); saveErr != nil && deps.Logger != nil {
		deps.Logger.Warnf("保存成功的图片上传任务状态失败: 任务ID=%s 错误=%v", task.ID.String(), saveErr)
	}
	if err = applyPendingAuditStatusIfAny(deps, result.Image); err != nil && deps.Logger != nil {
		deps.Logger.Warnf("应用七牛审核结果失败: 图片ID=%s 对象键=%s 错误=%v", result.Image.ID.String(), result.Image.ObjectKey, err)
	}
	return &result, nil
}

// buildObjectKey 构建七牛存储对象Key
// 格式：前缀/日期/文件哈希
func buildObjectKey(deps Deps, hash string) string {
	prefix := strings.Trim(deps.QiNiu.Prefix, "/")
	if prefix == "" {
		prefix = "images"
	}
	dateDir := time.Now().Format("20060102")
	return fmt.Sprintf("%s/images/%s/%s", prefix, dateDir, hash)
}

// containsString 判断字符串是否在切片中（忽略大小写、空格）
func containsString(list []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, item := range list {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

// chooseVerifiedMime 选择最终使用的MIME类型
// 优先使用文件统计信息，否则使用格式推导值
func chooseVerifiedMime(statMime, verifiedMime string) string {
	if strings.TrimSpace(statMime) != "" {
		return statMime
	}
	return verifiedMime
}

// formatToMime 将图片格式（如jpg）转为标准MIME类型（image/jpeg）
func formatToMime(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "jpg":
		format = "jpeg"
	}
	if format == "" {
		return ""
	}
	return "image/" + format
}
