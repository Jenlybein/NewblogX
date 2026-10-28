// Package apptest 提供媒体模块的内存替身，供用例测试与路由表测试使用。
package apptest

import (
	"context"
	"myblogx/internal/kernel/ctype"
	"net/http"
	"time"

	"myblogx/internal/media/app"
	"myblogx/internal/media/domain"
)

// Repo 是内存图片仓储。
type Repo struct {
	byID   map[domain.ImageID]domain.Image
	byHash map[string]domain.ImageID
	next   uint64
}

// NewRepo 构造。
func NewRepo() *Repo {
	return &Repo{byID: map[domain.ImageID]domain.Image{}, byHash: map[string]domain.ImageID{}}
}

// FindByHash 实现 domain.Repository。
func (r *Repo) FindByHash(_ context.Context, hash string) (domain.Image, bool, error) {
	id, ok := r.byHash[hash]
	if !ok {
		return domain.Image{}, false, nil
	}
	return r.byID[id], true, nil
}

// FindByObjectKey 实现 domain.Repository。
func (r *Repo) FindByObjectKey(_ context.Context, key string) (domain.Image, bool, error) {
	for _, img := range r.byID {
		if img.ObjectKey == key {
			return img, true, nil
		}
	}
	return domain.Image{}, false, nil
}

// FindByID 实现 domain.Repository。
func (r *Repo) FindByID(_ context.Context, id domain.ImageID) (domain.Image, error) {
	return r.byID[id], nil
}

// FindByIDs 实现 domain.Repository。
func (r *Repo) FindByIDs(_ context.Context, ids []domain.ImageID) ([]domain.Image, error) {
	out := make([]domain.Image, 0, len(ids))
	for _, id := range ids {
		if img, ok := r.byID[id]; ok {
			out = append(out, img)
		}
	}
	return out, nil
}

// Create 实现 domain.Repository。
func (r *Repo) Create(_ context.Context, img *domain.Image) error {
	r.next++
	img.ID = ctype.NewID(r.next)
	r.byID[img.ID] = *img
	r.byHash[img.Hash] = img.ID
	return nil
}

// UpdateStatus 实现 domain.Repository。
func (r *Repo) UpdateStatus(_ context.Context, id domain.ImageID, status domain.Status) error {
	img := r.byID[id]
	img.Status = status
	r.byID[id] = img
	return nil
}

// List 实现 domain.Repository。
func (r *Repo) List(_ context.Context, _ domain.Query) ([]domain.Image, int64, error) {
	out := make([]domain.Image, 0, len(r.byID))
	for _, img := range r.byID {
		out = append(out, img)
	}
	return out, int64(len(out)), nil
}

// SoftDeleteMany 实现 domain.Repository。
func (r *Repo) SoftDeleteMany(_ context.Context, ids []domain.ImageID) error {
	for _, id := range ids {
		delete(r.byID, id)
	}
	return nil
}

// DeleteWithRefs 实现 domain.Repository。
func (r *Repo) DeleteWithRefs(ctx context.Context, ids []domain.ImageID) error {
	return r.SoftDeleteMany(ctx, ids)
}

// TaskStore 是内存任务缓存。
type TaskStore struct{ byID map[uint64]*domain.Task }

// NewTaskStore 构造。
func NewTaskStore() *TaskStore { return &TaskStore{byID: map[uint64]*domain.Task{}} }

// Save 实现 domain.TaskStore。
func (s *TaskStore) Save(_ context.Context, task *domain.Task, _ time.Duration) error {
	copied := *task
	s.byID[task.ID.Uint64()] = &copied
	return nil
}

// Get 实现 domain.TaskStore。
func (s *TaskStore) Get(_ context.Context, id uint64) (*domain.Task, error) {
	task, ok := s.byID[id]
	if !ok {
		return nil, nil
	}
	copied := *task
	return &copied, nil
}

// GetIDByObjectKey 实现 domain.TaskStore。
func (s *TaskStore) GetIDByObjectKey(_ context.Context, key string) (uint64, error) {
	for id, task := range s.byID {
		if task.ObjectKey == key {
			return id, nil
		}
	}
	return 0, nil
}

// Lock 实现 domain.TaskStore（测试里永远加锁成功）。
func (s *TaskStore) Lock(_ context.Context, _ uint64, _ time.Duration) (func(), bool, error) {
	return func() {}, true, nil
}

// AuditStore 是内存审核暂存。
type AuditStore struct{ byKey map[string]string }

// NewAuditStore 构造。
func NewAuditStore() *AuditStore { return &AuditStore{byKey: map[string]string{}} }

// Store 实现 domain.AuditStore。
func (s *AuditStore) Store(_ context.Context, key, status string, _ time.Duration) error {
	s.byKey[key] = status
	return nil
}

// Consume 实现 domain.AuditStore。
func (s *AuditStore) Consume(_ context.Context, key string) (string, bool, error) {
	v, ok := s.byKey[key]
	delete(s.byKey, key)
	return v, ok, nil
}

// Storage 是无副作用的对象存储替身。
type Storage struct{}

// Token 实现 domain.ObjectStorage。
func (Storage) Token(p domain.Policy) (domain.Token, error) {
	return domain.Token{Token: "token", Bucket: p.Bucket, ObjectKey: p.ObjectKey, ExpireAt: p.ExpireAt}, nil
}

// VerifyCallback 实现 domain.ObjectStorage。
func (Storage) VerifyCallback(_ *http.Request) (bool, error) { return true, nil }

// Stat 实现 domain.ObjectStorage。
func (Storage) Stat(_, _ string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Hash: "hash", Size: 1}, nil
}

// Delete 实现 domain.ObjectStorage。
func (Storage) Delete(_, _ string) error { return nil }

// ImageInfo 实现 domain.ObjectStorage。
func (Storage) ImageInfo(_ string) (domain.ImageInfo, error) {
	return domain.ImageInfo{Format: "png", Width: 10, Height: 10, Size: 1}, nil
}

// ObjectURL 实现 domain.ObjectStorage。
func (Storage) ObjectURL(key string) string { return "https://example.com/" + key }

// IDs 是递增 ID 生成器。
type IDs struct{ n uint64 }

// NextImageID 实现 domain.IDGenerator。
func (i *IDs) NextImageID() domain.ImageID { i.n++; return ctype.NewID(i.n) }

// NextTaskID 实现 domain.IDGenerator。
func (i *IDs) NextTaskID() uint64 { i.n++; return i.n }

// Clock 是固定时钟。
type Clock struct{}

// Now 实现 domain.Clock。
func (Clock) Now() time.Time { return time.Unix(1700000000, 0) }

// NewService 组装一个可用的内存服务。
//
// 配置包成函数是为了对齐生产装配：媒体服务每次用时现读配置，
// 后台改完立即生效。测试里这个函数始终返回同一份值。
func NewService(cfg domain.Config) *app.Service {
	return app.New(NewRepo(), NewTaskStore(), NewAuditStore(), Storage{}, &IDs{}, Clock{}, nil,
		func() domain.Config { return cfg })
}

// NewDefaultService 用空配置组装服务（路由表测试用）。
func NewDefaultService() *app.Service { return NewService(domain.Config{}) }
