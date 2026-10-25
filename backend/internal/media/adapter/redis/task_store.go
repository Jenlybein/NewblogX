// Package redis 是媒体模块的 Redis 适配器（上传任务缓存 + 分布式锁）。
package redis

import (
	"context"
	"encoding/json"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/media/domain"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_image"

	goredis "github.com/go-redis/redis/v8"
)

// TaskStore 实现 domain.TaskStore。
type TaskStore struct {
	deps redis_service.Options
}

var _ domain.TaskStore = (*TaskStore)(nil)

// NewTaskStore 构造任务缓存。
func NewTaskStore(deps redis_service.Options) *TaskStore { return &TaskStore{deps: deps} }

// Save 保存任务。
func (s *TaskStore) Save(_ context.Context, task *domain.Task, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return redis_image.StoreTask(s.deps, ctype.ID(task.ID), task.ObjectKey, data, ttl)
}

// Get 按任务 ID 取任务。
func (s *TaskStore) Get(_ context.Context, id uint64) (*domain.Task, error) {
	data, err := redis_image.GetTaskDataByID(s.deps, ctype.ID(id))
	if err != nil {
		if err == goredis.Nil {
			return nil, apperr.New(apperr.CodeImageTaskNotFound)
		}
		return nil, err
	}
	var task domain.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetIDByObjectKey 按对象 key 反查任务 ID。
func (s *TaskStore) GetIDByObjectKey(_ context.Context, objectKey string) (uint64, error) {
	id, err := redis_image.GetTaskIDByObjectKey(s.deps, objectKey)
	if err != nil {
		if err == goredis.Nil {
			return 0, apperr.New(apperr.CodeImageTaskNotFound)
		}
		return 0, err
	}
	return uint64(id), nil
}

// Lock 对任务加锁。
func (s *TaskStore) Lock(_ context.Context, id uint64, ttl time.Duration) (func(), bool, error) {
	return redis_image.LockTask(s.deps, ctype.ID(id), ttl)
}

// AuditStore 实现 domain.AuditStore。
type AuditStore struct {
	deps redis_service.Options
}

var _ domain.AuditStore = (*AuditStore)(nil)

// NewAuditStore 构造审核状态暂存。
func NewAuditStore(deps redis_service.Options) *AuditStore { return &AuditStore{deps: deps} }

// Store 暂存审核结论。
func (s *AuditStore) Store(_ context.Context, objectKey, status string, ttl time.Duration) error {
	return redis_image.StoreAuditStatus(s.deps, objectKey, status, ttl)
}

// Consume 读取并删除暂存结论。
func (s *AuditStore) Consume(_ context.Context, objectKey string) (string, bool, error) {
	value, err := redis_image.ConsumeAuditStatus(s.deps, objectKey)
	if err != nil {
		if err == goredis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}
