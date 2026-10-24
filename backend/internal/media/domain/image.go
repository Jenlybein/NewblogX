// Package domain 是媒体（图片）模块的领域层。
package domain

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ImageID 是图片标识。
type ImageID uint64

// Uint64 返回底层值。
func (id ImageID) Uint64() uint64 { return uint64(id) }

// IsZero 判断是否为空。
func (id ImageID) IsZero() bool { return id == 0 }

// Status 是图片审核状态。
type Status uint

const (
	// StatusUnknown 未知。
	StatusUnknown Status = iota
	// StatusPass 审核通过，可对外展示。
	StatusPass
	// StatusDeleted 已删除。
	StatusDeleted
	// StatusOrphaned 无引用。
	StatusOrphaned
	// StatusReviewing 审核中。
	StatusReviewing
	// StatusBlocked 违规封禁。
	StatusBlocked
)

// String 返回状态短名（与旧契约一致）。
func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusDeleted:
		return "deleted"
	case StatusOrphaned:
		return "orphaned"
	case StatusReviewing:
		return "review"
	case StatusBlocked:
		return "block"
	default:
		return "unknown"
	}
}

// StatusFromSuggestion 把七牛审核建议映射成内部状态。
func StatusFromSuggestion(suggestion string) Status {
	switch strings.ToLower(strings.TrimSpace(suggestion)) {
	case "pass":
		return StatusPass
	case "review":
		return StatusReviewing
	case "block":
		return StatusBlocked
	case "deleted":
		return StatusDeleted
	case "orphaned":
		return StatusOrphaned
	default:
		return StatusUnknown
	}
}

// Provider 是存储服务商。
type Provider string

// ProviderQiniu 七牛。
const ProviderQiniu Provider = "qiniu"

// TaskStatus 是上传任务状态。
type TaskStatus string

const (
	// TaskPending 待上传。
	TaskPending TaskStatus = "pending"
	// TaskReady 已完成。
	TaskReady TaskStatus = "ready"
	// TaskFailed 校验失败。
	TaskFailed TaskStatus = "failed"
)

// Image 是一张已验收的图片。
type Image struct {
	ID        ImageID
	CreatedAt time.Time
	UserID    uint64
	Provider  Provider
	Bucket    string
	ObjectKey string
	FileName  string
	URL       string
	MimeType  string
	Size      int64
	Width     int
	Height    int
	Hash      string
	Status    Status
}

// Task 是一次直传上传任务。
type Task struct {
	ID           uint64
	UserID       uint64
	Provider     Provider
	Status       TaskStatus
	Bucket       string
	ObjectKey    string
	OriginalName string
	DeclaredMime string
	DeclaredSize int64
	VerifiedMime string
	VerifiedSize int64
	Width        int
	Height       int
	Hash         string
	ErrorMsg     string
	ExpiresAt    time.Time
	ConfirmedAt  *time.Time
	ImageID      *uint64
	ImageURL     string
}

// Policy 是一次上传的约束。
type Policy struct {
	Bucket      string
	ObjectKey   string
	CallbackURL string
	ExpireAt    time.Time
	MaxSize     int64
	EndUser     string
}

// Token 是签发出来的上传凭证。
type Token struct {
	Token     string
	Bucket    string
	ObjectKey string
	ExpireAt  time.Time
}

// ObjectInfo 是对象存储上的文件元信息。
type ObjectInfo struct {
	Hash string
	Size int64
}

// ImageInfo 是图片格式与尺寸。
type ImageInfo struct {
	Format string
	Width  int
	Height int
	Size   int64
}

// Config 是媒体的运行配置。
type Config struct {
	Enabled       bool
	Bucket        string
	Prefix        string
	Region        string
	SizeMB        int
	ExpirySeconds int
	CallbackURL   string
	Whitelist     []string
}

// MaxSize 返回字节上限。
func (c Config) MaxSize() int64 { return int64(c.SizeMB) * 1024 * 1024 }

// Query 是图片列表查询。
type Query struct {
	Page  int
	Limit int
	Key   string
}

// Normalize 收敛分页（默认 10、上限 100）。
func (q Query) Normalize() Query {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 10
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	return q
}

// Offset 返回偏移量。
func (q Query) Offset() int { n := q.Normalize(); return (n.Page - 1) * n.Limit }

// Repository 是图片持久化端口。
type Repository interface {
	FindByHash(ctx context.Context, hash string) (Image, bool, error)
	FindByObjectKey(ctx context.Context, objectKey string) (Image, bool, error)
	FindByID(ctx context.Context, id ImageID) (Image, error)
	FindByIDs(ctx context.Context, ids []ImageID) ([]Image, error)
	Create(ctx context.Context, img *Image) error
	UpdateStatus(ctx context.Context, id ImageID, status Status) error
	List(ctx context.Context, q Query) ([]Image, int64, error)
	SoftDeleteMany(ctx context.Context, ids []ImageID) error
	// DeleteWithRefs 在一个事务里删引用再硬删图片。
	DeleteWithRefs(ctx context.Context, ids []ImageID) error
}

// AuditStore 暂存"审核回调先到、图片还没入库"的审核结论。
type AuditStore interface {
	Store(ctx context.Context, objectKey, status string, ttl time.Duration) error
	// Consume 读取并删除；found=false 表示没有暂存结论。
	Consume(ctx context.Context, objectKey string) (status string, found bool, err error)
}

// TaskStore 是上传任务的缓存端口。
type TaskStore interface {
	Save(ctx context.Context, task *Task, ttl time.Duration) error
	Get(ctx context.Context, id uint64) (*Task, error)
	GetIDByObjectKey(ctx context.Context, objectKey string) (uint64, error)
	Lock(ctx context.Context, id uint64, ttl time.Duration) (unlock func(), locked bool, err error)
}

// ObjectStorage 是对象存储端口。
type ObjectStorage interface {
	Token(policy Policy) (Token, error)
	VerifyCallback(req *http.Request) (bool, error)
	Stat(bucket, key string) (ObjectInfo, error)
	Delete(bucket, key string) error
	ImageInfo(key string) (ImageInfo, error)
	ObjectURL(key string) string
}

// IDGenerator 生成图片与任务 ID。
type IDGenerator interface {
	NextImageID() ImageID
	NextTaskID() uint64
}

// Clock 提供当前时间。
type Clock interface {
	Now() time.Time
}

// Logger 是窄日志接口。
type Logger interface {
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}
