package image_service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/redis_service/redis_image"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// 审核状态缓存过期时间：24小时
const auditStatusCacheTTL = 24 * time.Hour

// HandleQiniuAuditCallback
// 处理七牛云内容审核回调（核心入口）
// 逻辑：
//  1. 解析回调内容，获取文件key + 审核结果
//  2. 映射为系统内部图片状态（正常/审核中/违规）
//  3. 图片已入库 → 直接更新数据库状态
//  4. 图片未入库 → 把审核结果暂存Redis，等待图片入库后消费
func HandleQiniuAuditCallback(deps Deps, body []byte) error {
	// 解析回调数据，获取文件key和审核建议
	objectKey, suggestion, err := parseQiniuAuditCallback(body)
	if err != nil {
		return err
	}

	// 将七牛的审核建议映射为系统图片状态
	status := enum.ImageStatusMapString(suggestion)
	if status == enum.ImageStatusUnknown {
		return fmt.Errorf("未知的七牛审核结论: %s", suggestion)
	}

	// 根据文件key查询数据库中是否已存在图片记录
	var image models.ImageModel
	err = deps.DB.Where("object_key = ?", objectKey).Take(&image).Error
	switch {
	case err == nil:
		// 图片已存在：状态相同则无需更新
		if image.Status == status {
			return nil
		}
		// 更新图片审核状态
		return deps.DB.Model(&models.ImageModel{}).
			Where("id = ?", image.ID).
			Update("status", status).Error

	case errors.Is(err, gorm.ErrRecordNotFound):
		// 图片还未入库：将审核状态暂存Redis，等待后续消费
		return redis_image.StoreAuditStatus(deps.RedisDeps(), objectKey, status.String(), auditStatusCacheTTL)

	default:
		// 数据库查询异常
		return err
	}
}

// applyPendingAuditStatusIfAny
// 图片正式入库后，尝试消费Redis中暂存的审核结果
// 作用：解决“审核回调比图片入库更快”的时序问题
func applyPendingAuditStatusIfAny(deps Deps, image *models.ImageModel) error {
	// 从Redis消费该文件的预存审核状态
	statusName, err := redis_image.ConsumeAuditStatus(deps.RedisDeps(), image.ObjectKey)
	if err != nil {
		// 无预存状态 → 正常返回
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}

	// 将字符串状态映射为枚举
	status := enum.ImageStatusMapString(statusName)
	if status == enum.ImageStatusUnknown {
		return fmt.Errorf("未知的七牛审核结论: %s", statusName)
	}

	// 状态一致则无需更新
	if image.Status == status {
		return nil
	}

	// 执行数据库状态更新
	if err = deps.DB.Model(&models.ImageModel{}).
		Where("id = ?", image.ID).
		Update("status", status).Error; err != nil {
		return err
	}

	// 同步更新内存对象状态
	image.Status = status
	return nil
}

// parseQiniuAuditCallback
// 兼容解析七牛审核回调JSON（适配七牛多种回调格式）
// 从多层嵌套结构中安全提取：文件key + 审核建议
// 返回：objectKey, suggestion, error
func parseQiniuAuditCallback(body []byte) (objectKey string, suggestion string, err error) {
	var payload qiniuAuditCallbackPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}

	choose := func(values ...string) string {
		for _, value := range values {
			if value != "" {
				return value
			}
		}
		return ""
	}

	objectKey = choose(
		payload.InputKey,
		payload.Key,
		payload.ObjectKey,
	)
	if objectKey == "" {
		return "", "", errors.New("七牛审核回调缺少对象 key")
	}

	if len(payload.Items) > 0 {
		suggestion = choose(
			payload.Items[0].Result.Result.Suggestion,
			payload.Items[0].Result.Suggestion,
		)
	}
	if suggestion == "" {
		suggestion = choose(
			payload.Result.Result.Suggestion,
			payload.Result.Suggestion,
			payload.Suggestion,
		)
	}
	if suggestion == "" {
		return "", "", errors.New("七牛审核回调缺少审核结论")
	}

	return objectKey, suggestion, nil
}
