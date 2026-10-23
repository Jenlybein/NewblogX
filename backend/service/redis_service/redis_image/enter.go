// 提供图片上传任务相关的Redis操作封装，包含任务存储、查询、分布式锁等核心功能
package redis_image

import (
	"context"
	"fmt"
	"time"

	"myblogx/models/ctype"
	"myblogx/service/redis_service"

	"github.com/go-redis/redis/v8"
)

// releaseUploadTaskLockScript Redis Lua脚本：安全释放分布式锁
// 逻辑：判断锁值是否一致，一致则原子性删除锁
var releaseUploadTaskLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// StoreTask 存储图片上传任务数据（事务操作，保证原子性）
func StoreTask(deps redis_service.Deps, taskID ctype.ID, objectKey string, data []byte, ttl time.Duration) error {
	//	taskID - 任务唯一ID
	//	objectKey - 对象存储的key
	//	data - 任务数据（字节数组）
	//	ttl - 数据过期时间，<=0时默认1分钟

	ctx := context.Background()
	// 创建Redis事务管道
	if deps.Client == nil {
		return fmt.Errorf("redis 未初始化")
	}
	pipe := deps.Client.TxPipeline()

	// 1. 根据taskID存储任务原始数据
	pipe.Set(ctx, uploadTaskIDKey(taskID), data, ttl)
	// 2. 根据objectKey存储taskID，建立反向索引
	pipe.Set(ctx, uploadTaskObjectKey(objectKey), taskID.String(), ttl)

	// 执行事务
	_, err := pipe.Exec(ctx)
	return err
}

// GetTaskDataByID 根据任务ID获取任务原始数据
func GetTaskDataByID(deps redis_service.Deps, taskID ctype.ID) ([]byte, error) {
	// 从Redis获取字节数据
	if deps.Client == nil {
		return nil, fmt.Errorf("redis 未初始化")
	}
	return deps.Client.Get(context.Background(), uploadTaskIDKey(taskID)).Bytes()
}

// GetTaskIDByObjectKey 根据对象key反向查询对应的任务ID
// 返回：任务ID、查询/解析错误
func GetTaskIDByObjectKey(deps redis_service.Deps, objectKey string) (ctype.ID, error) {
	client := deps.Client
	if client == nil {
		return 0, fmt.Errorf("redis 未初始化")
	}

	// 获取字符串格式的taskID
	rawID, err := client.Get(context.Background(), uploadTaskObjectKey(objectKey)).Result()
	if err != nil {
		return 0, err
	}

	// 反序列化为自定义ID类型
	var taskID ctype.ID
	if err = taskID.UnmarshalText([]byte(rawID)); err != nil {
		return 0, err
	}
	return taskID, nil
}

// LockTask 对图片上传任务加分布式锁（防并发冲突）
func LockTask(deps redis_service.Deps, taskID ctype.ID, ttl time.Duration) (unlock func(), locked bool, err error) {
	//	taskID - 要加锁的任务ID
	//	ttl - 锁过期时间，<=0时默认30秒
	//	unlock - 解锁函数
	//	locked - 是否加锁成功

	// 默认锁过期时间30秒
	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	ctx := context.Background()
	token := fmt.Sprintf("%d", time.Now().UnixNano())

	// SETNX：不存在则设置，实现加锁
	client := deps.Client
	locked, err = client.SetNX(ctx, uploadTaskLockKey(taskID), token, ttl).Result()
	if err != nil || !locked {
		return nil, locked, err
	}

	// 返回闭包解锁函数，内部使用Lua脚本安全释放锁
	return func() {
		// 执行Lua脚本释放锁
		if _, releaseErr := releaseUploadTaskLockScript.Run(ctx, client, []string{uploadTaskLockKey(taskID)}, token).Result(); releaseErr != nil && deps.Logger != nil {
			deps.Logger.Warnf("释放图片上传任务锁失败: 任务ID=%s 错误=%v", taskID.String(), releaseErr)
		}
	}, true, nil
}

// StoreAuditStatus 暂存图片审核状态，兜住“审核回调先到、图片记录后建”的情况。
func StoreAuditStatus(deps redis_service.Deps, objectKey string, status string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if deps.Client == nil {
		return fmt.Errorf("redis 未初始化")
	}
	return deps.Client.Set(context.Background(), imageAuditKey(objectKey), status, ttl).Err()
}

// ConsumeAuditStatus 读取并删除暂存的图片审核状态。
func ConsumeAuditStatus(deps redis_service.Deps, objectKey string) (string, error) {
	if deps.Client == nil {
		return "", fmt.Errorf("redis 未初始化")
	}
	return deps.Client.GetDel(context.Background(), imageAuditKey(objectKey)).Result()
}
