package image_service

import (
	"encoding/json"
	"errors"
	"time"

	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_image"

	"github.com/go-redis/redis/v8"
)

const (
	uploadTaskTTLBuffer    = 10 * time.Minute // 任务过期缓冲时间，防止任务提前失效
	finalizedTaskKeepAlive = 24 * time.Hour   // 已完成/失败任务保留缓存时长
)

// saveUploadTask 保存上传任务到 Redis
func saveUploadTask(deps Deps, task *ImageUploadTask, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	if err = redis_image.StoreTask(deps.RedisDeps(), task.ID, task.ObjectKey, data, ttl); err != nil {
		return err
	}
	return nil
}

// getUploadTaskByID 根据任务ID从Redis获取上传任务
func getUploadTaskByID(deps Deps, taskID ctype.ID) (*ImageUploadTask, error) {
	// 从Redis获取任务原始数据
	data, err := redis_image.GetTaskDataByID(deps.RedisDeps(), taskID)
	if err != nil {
		// Redis不存在该任务，返回任务不存在错误
		if errors.Is(err, redis.Nil) {
			return nil, ErrUploadTaskNotFound
		}
		return nil, err
	}

	// 反序列化为任务结构体
	var task ImageUploadTask
	if err = json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// getUploadTaskIDByObjectKey 根据七牛对象 key 查询上传任务 ID。
func getUploadTaskIDByObjectKey(deps Deps, objectKey string) (ctype.ID, error) {
	// 根据对象key查询任务ID
	taskID, err := redis_image.GetTaskIDByObjectKey(deps.RedisDeps(), objectKey)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrUploadTaskNotFound
		}
		return 0, err
	}
	return taskID, nil
}

// getUploadTaskByObjectKey 根据七牛对象key获取上传任务
// 先通过objectKey查找到taskID，再通过taskID获取任务
func getUploadTaskByObjectKey(deps Deps, objectKey string) (*ImageUploadTask, error) {
	taskID, err := getUploadTaskIDByObjectKey(deps, objectKey)
	if err != nil {
		return nil, err
	}
	return getUploadTaskByID(deps, taskID)
}

// lockUploadTask 对上传任务加分布式锁
// 防止并发重复确认上传任务，返回解锁函数、是否加锁成功、错误
func lockUploadTask(deps Deps, taskID ctype.ID, ttl time.Duration) (unlock func(), locked bool, err error) {
	unlock, locked, err = redis_image.LockTask(deps.RedisDeps(), taskID, ttl)
	return unlock, locked, err
}

// taskPendingTTL 计算待上传任务的缓存过期时间
// 过期时间 = 上传凭证过期时间 + 缓冲时间，确保凭证有效期内任务不失效
func taskPendingTTL(expireAt time.Time) time.Duration {
	ttl := time.Until(expireAt) + uploadTaskTTLBuffer
	// 兜底最小过期时间
	if ttl <= 0 {
		return time.Minute
	}
	return ttl
}
