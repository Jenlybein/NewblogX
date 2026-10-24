// Package app 是媒体（图片）模块的用例层。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/media/domain"
)

const (
	// uploadTaskTTLBuffer 任务缓存比凭证多留一段时间，避免凭证还有效任务先过期。
	uploadTaskTTLBuffer = 10 * time.Minute
	// finalizedKeepAlive 已完成/失败任务的缓存保留时长。
	finalizedKeepAlive = 24 * time.Hour
	// auditStatusTTL 审核结论暂存时长。
	auditStatusTTL = 24 * time.Hour
)

// CreateResult 是创建上传任务的结果。
type CreateResult struct {
	SkipUpload bool
	Task       *domain.Task
	Token      *domain.Token
	Image      *domain.Image
	Region     string
	MaxSize    int64
}

// ConfirmResult 是确认/查询任务的结果。
type ConfirmResult struct {
	Task  *domain.Task
	Image *domain.Image
}

type uploadedMeta struct {
	Bucket string
	Hash   string
	Size   int64
}

type verifiedImage struct {
	TaskID             uint64
	UserID             uint64
	Bucket             string
	ObjectKey          string
	FileName           string
	Hash               string
	MimeType           string
	Size               int64
	Width              int
	Height             int
	ShouldDeleteUpload bool
}

// Service 是媒体应用服务。
type Service struct {
	repo    domain.Repository
	tasks   domain.TaskStore
	audits  domain.AuditStore
	storage domain.ObjectStorage
	ids     domain.IDGenerator
	clock   domain.Clock
	logger  domain.Logger
	cfg     domain.Config
}

// New 构造媒体服务。
func New(
	repo domain.Repository,
	tasks domain.TaskStore,
	audits domain.AuditStore,
	storage domain.ObjectStorage,
	ids domain.IDGenerator,
	clock domain.Clock,
	logger domain.Logger,
	cfg domain.Config,
) *Service {
	if repo == nil {
		panic("media/app: 图片仓储未注入")
	}
	if tasks == nil {
		panic("media/app: 任务缓存未注入")
	}
	if storage == nil {
		panic("media/app: 对象存储未注入")
	}
	if ids == nil {
		panic("media/app: ID 生成器未注入")
	}
	return &Service{repo: repo, tasks: tasks, audits: audits, storage: storage, ids: ids, clock: clock, logger: logger, cfg: cfg}
}

// CreateTask 创建直传上传任务；相同哈希直接秒传。
func (s *Service) CreateTask(ctx context.Context, userID uint64, fileName string, size int64, mimeType, hash string) (*CreateResult, error) {
	q := s.cfg
	if !q.Enabled || q.Bucket == "" || q.SizeMB <= 0 || q.ExpirySeconds <= 0 {
		return nil, apperr.New(apperr.CodeImageUploadDisabled)
	}

	hash = strings.TrimSpace(hash)
	maxSize := q.MaxSize()
	suffix := imageSuffix(fileName)

	if size <= 0 {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}
	if hash == "" {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}
	if size > maxSize {
		return nil, apperr.New(apperr.CodeImageTooLarge)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}
	if !whitelistContains(q.Whitelist, suffix) {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}

	if image, ok, err := s.repo.FindByHash(ctx, hash); err != nil {
		return nil, err
	} else if ok {
		return &CreateResult{SkipUpload: true, Image: &image, Region: q.Region, MaxSize: maxSize}, nil
	}

	now := s.clock.Now()
	task := &domain.Task{
		ID:           s.ids.NextTaskID(),
		UserID:       userID,
		Provider:     domain.ProviderQiniu,
		Status:       domain.TaskPending,
		Bucket:       q.Bucket,
		ObjectKey:    buildObjectKey(q.Prefix, hash, now),
		OriginalName: fileName,
		DeclaredMime: mimeType,
		DeclaredSize: size,
		Hash:         hash,
		ExpiresAt:    now.Add(time.Duration(q.ExpirySeconds) * time.Second),
	}

	token, err := s.storage.Token(domain.Policy{
		Bucket:      q.Bucket,
		ObjectKey:   task.ObjectKey,
		CallbackURL: q.CallbackURL,
		ExpireAt:    task.ExpiresAt,
		MaxSize:     maxSize,
		EndUser:     fmt.Sprintf("%d", userID),
	})
	if err != nil {
		return nil, err
	}
	if err := s.tasks.Save(ctx, task, taskPendingTTL(task.ExpiresAt)); err != nil {
		return nil, err
	}
	return &CreateResult{Task: task, Token: &token, Region: q.Region, MaxSize: maxSize}, nil
}

// ConfirmByUser 用户手动确认上传完成。
func (s *Service) ConfirmByUser(ctx context.Context, taskID, userID uint64, objectKey string) (*ConfirmResult, error) {
	return s.confirm(ctx, taskID, nil, func(task *domain.Task) error {
		if task.UserID != userID {
			return apperr.New(apperr.CodeImageTaskNotFound)
		}
		if objectKey != "" && task.ObjectKey != objectKey {
			return apperr.New(apperr.CodeImageMismatch)
		}
		return nil
	})
}

// ConfirmByCallback 七牛回调确认上传。
func (s *Service) ConfirmByCallback(ctx context.Context, objectKey, bucket, hash string, size int64) (*ConfirmResult, error) {
	taskID, err := s.tasks.GetIDByObjectKey(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	return s.confirm(ctx, taskID, &uploadedMeta{Bucket: bucket, Hash: hash, Size: size}, func(task *domain.Task) error {
		if task.ObjectKey != objectKey {
			return apperr.New(apperr.CodeImageTaskNotFound)
		}
		return nil
	})
}

// GetStatus 查询任务状态（前端轮询）。
func (s *Service) GetStatus(ctx context.Context, taskID, userID uint64) (*ConfirmResult, error) {
	task, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.UserID != userID {
		return nil, apperr.New(apperr.CodeImageTaskNotFound)
	}
	result := &ConfirmResult{Task: task}
	if task.ImageID != nil {
		if task.ImageURL != "" {
			result.Image = &domain.Image{ID: domain.ImageID(*task.ImageID), URL: task.ImageURL}
			return result, nil
		}
		if image, err := s.repo.FindByID(ctx, domain.ImageID(*task.ImageID)); err == nil {
			result.Image = &image
		}
	}
	return result, nil
}

// confirm 统一确认流程：加锁 → 校验对象 → 入库 → 回写任务。
func (s *Service) confirm(ctx context.Context, taskID uint64, meta *uploadedMeta, validate func(*domain.Task) error) (*ConfirmResult, error) {
	unlock, locked, err := s.tasks.Lock(ctx, taskID, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if locked && unlock != nil {
		defer unlock()
	}

	task, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if validate != nil {
		if err := validate(task); err != nil {
			return nil, err
		}
	}
	if task.Status == domain.TaskReady && task.ImageID != nil {
		image, err := s.repo.FindByID(ctx, domain.ImageID(*task.ImageID))
		if err != nil {
			return nil, err
		}
		return &ConfirmResult{Task: task, Image: &image}, nil
	}
	if task.Status == domain.TaskFailed {
		return nil, apperr.New(apperr.CodeImageTaskFailed)
	}
	if !locked {
		return nil, apperr.New(apperr.CodeImageTaskProcessing)
	}

	verified, err := s.verify(ctx, task, meta)
	if err != nil {
		task.Status = domain.TaskFailed
		task.ErrorMsg = err.Error()
		if saveErr := s.tasks.Save(ctx, task, finalizedKeepAlive); saveErr != nil && s.logger != nil {
			s.logger.Warnf("保存失败的上传任务状态失败: task=%d err=%v", task.ID, saveErr)
		}
		return nil, err
	}

	result, err := s.persist(ctx, task, verified)
	if err != nil {
		return nil, err
	}
	if verified.ShouldDeleteUpload {
		if delErr := s.storage.Delete(task.Bucket, task.ObjectKey); delErr != nil && s.logger != nil {
			s.logger.Warnf("删除重复上传对象失败: key=%s err=%v", task.ObjectKey, delErr)
		}
	}
	return result, nil
}

// verify 校验对象存储上的真实文件。
func (s *Service) verify(ctx context.Context, task *domain.Task, meta *uploadedMeta) (*verifiedImage, error) {
	var objectHash string
	var objectSize int64
	if meta != nil {
		if meta.Bucket != "" && meta.Bucket != task.Bucket {
			return nil, apperr.New(apperr.CodeImageMismatch)
		}
		objectHash = meta.Hash
		objectSize = meta.Size
	}
	if objectHash == "" || objectSize <= 0 {
		info, err := s.storage.Stat(task.Bucket, task.ObjectKey)
		if err != nil {
			return nil, err
		}
		objectHash = info.Hash
		objectSize = info.Size
	}
	if objectHash == "" {
		return nil, apperr.New(apperr.CodeImageMismatch)
	}
	if task.Hash != "" && task.Hash != objectHash {
		return nil, apperr.New(apperr.CodeImageMismatch)
	}
	if task.DeclaredSize > 0 && objectSize != task.DeclaredSize {
		return nil, apperr.New(apperr.CodeImageMismatch)
	}

	imageInfo, err := s.storage.ImageInfo(task.ObjectKey)
	if err != nil {
		return nil, err
	}
	format := strings.ToLower(strings.TrimSpace(imageInfo.Format))
	if !whitelistContains(s.cfg.Whitelist, format) {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}

	return &verifiedImage{
		TaskID:    task.ID,
		UserID:    task.UserID,
		Bucket:    task.Bucket,
		ObjectKey: task.ObjectKey,
		FileName:  task.OriginalName,
		Hash:      objectHash,
		MimeType:  chooseMime(task.DeclaredMime, formatToMime(format)),
		Size:      objectSize,
		Width:     imageInfo.Width,
		Height:    imageInfo.Height,
	}, nil
}

// persist 落图片记录并回写任务状态。
func (s *Service) persist(ctx context.Context, task *domain.Task, verified *verifiedImage) (*ConfirmResult, error) {
	image, ok, err := s.repo.FindByHash(ctx, verified.Hash)
	if err != nil {
		return nil, err
	}
	if ok {
		verified.ShouldDeleteUpload = image.ObjectKey != verified.ObjectKey
	} else {
		created := domain.Image{
			UserID:    verified.UserID,
			Provider:  domain.ProviderQiniu,
			Bucket:    verified.Bucket,
			ObjectKey: verified.ObjectKey,
			FileName:  verified.FileName,
			URL:       s.storage.ObjectURL(verified.ObjectKey),
			MimeType:  verified.MimeType,
			Size:      verified.Size,
			Width:     verified.Width,
			Height:    verified.Height,
			Hash:      verified.Hash,
			Status:    domain.StatusPass,
		}
		if err := s.repo.Create(ctx, &created); err != nil {
			// 并发下另一请求可能刚建好同哈希图片，重查一次。
			got, found, findErr := s.repo.FindByHash(ctx, verified.Hash)
			if findErr != nil || !found {
				return nil, err
			}
			created = got
			verified.ShouldDeleteUpload = created.ObjectKey != verified.ObjectKey
		}
		image = created
	}

	now := s.clock.Now()
	task.Status = domain.TaskReady
	task.VerifiedMime = verified.MimeType
	task.VerifiedSize = verified.Size
	task.Width = verified.Width
	task.Height = verified.Height
	task.Hash = verified.Hash
	task.ConfirmedAt = &now
	imageID := image.ID.Uint64()
	task.ImageID = &imageID
	task.ImageURL = image.URL

	if err := s.tasks.Save(ctx, task, finalizedKeepAlive); err != nil && s.logger != nil {
		s.logger.Warnf("保存完成的上传任务状态失败: task=%d err=%v", task.ID, err)
	}
	s.applyPendingAudit(ctx, &image)
	return &ConfirmResult{Task: task, Image: &image}, nil
}

// applyPendingAudit 消费"审核回调先到"时暂存的结论。
func (s *Service) applyPendingAudit(ctx context.Context, image *domain.Image) {
	if s.audits == nil {
		return
	}
	name, found, err := s.audits.Consume(ctx, image.ObjectKey)
	if err != nil || !found {
		return
	}
	status := domain.StatusFromSuggestion(name)
	if status == domain.StatusUnknown || status == image.Status {
		return
	}
	if err := s.repo.UpdateStatus(ctx, image.ID, status); err != nil {
		if s.logger != nil {
			s.logger.Warnf("应用审核结果失败: image=%d err=%v", image.ID, err)
		}
		return
	}
	image.Status = status
}

// HandleAuditCallback 处理七牛内容审核回调。
func (s *Service) HandleAuditCallback(ctx context.Context, body []byte) error {
	objectKey, suggestion, err := parseAuditCallback(body)
	if err != nil {
		return apperr.New(apperr.CodeImageCallbackInvalid)
	}
	status := domain.StatusFromSuggestion(suggestion)
	if status == domain.StatusUnknown {
		return apperr.New(apperr.CodeImageAuditInvalid)
	}

	image, ok, err := s.repo.FindByObjectKey(ctx, objectKey)
	if err != nil {
		return err
	}
	if !ok {
		if s.audits == nil {
			return nil
		}
		return s.audits.Store(ctx, objectKey, status.String(), auditStatusTTL)
	}
	if image.Status == status {
		return nil
	}
	return s.repo.UpdateStatus(ctx, image.ID, status)
}

// List 图片列表。
func (s *Service) List(ctx context.Context, q domain.Query) ([]domain.Image, int64, error) {
	return s.repo.List(ctx, q)
}

// Remove 批量删除图片：先删对象存储，再删引用与记录。
func (s *Service) Remove(ctx context.Context, ids []domain.ImageID) (int, error) {
	images, err := s.repo.FindByIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	if len(images) == 0 {
		return 0, apperr.New(apperr.CodeImageUnavailable)
	}
	for _, image := range images {
		if err := s.storage.Delete(image.Bucket, image.ObjectKey); err != nil {
			return 0, apperr.Wrap(apperr.CodeImageDeleteFailed, err)
		}
	}
	if err := s.repo.DeleteWithRefs(ctx, ids); err != nil {
		return 0, err
	}
	return len(images), nil
}

// VerifyCallback 校验回调签名。
func (s *Service) VerifyCallback(req *http.Request) (bool, error) {
	return s.storage.VerifyCallback(req)
}

// ConfigRegion 返回存储区域（响应展示用）。
func (s *Service) ConfigRegion() string { return s.cfg.Region }

// ConfigMaxSize 返回单文件大小上限（字节）。
func (s *Service) ConfigMaxSize() int64 { return s.cfg.MaxSize() }

func buildObjectKey(prefix, hash string, now time.Time) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "images"
	}
	return fmt.Sprintf("%s/images/%s/%s", prefix, now.Format("20060102"), hash)
}

func taskPendingTTL(expireAt time.Time) time.Duration {
	ttl := time.Until(expireAt) + uploadTaskTTLBuffer
	if ttl <= 0 {
		return time.Minute
	}
	return ttl
}

func imageSuffix(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 || idx == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[idx+1:])
}

func whitelistContains(list []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, item := range list {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func chooseMime(declared, fallback string) string {
	if strings.TrimSpace(declared) != "" {
		return declared
	}
	return fallback
}

func formatToMime(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "jpg" {
		format = "jpeg"
	}
	if format == "" {
		return ""
	}
	return "image/" + format
}

type auditPayload struct {
	InputKey   string          `json:"inputKey"`
	Key        string          `json:"key"`
	ObjectKey  string          `json:"objectKey"`
	Suggestion string          `json:"suggestion"`
	Result     auditResultL1   `json:"result"`
	Items      []auditCallback `json:"items"`
}

type auditCallback struct {
	Result auditResultL1 `json:"result"`
}

type auditResultL1 struct {
	Suggestion string        `json:"suggestion"`
	Result     auditResultL2 `json:"result"`
}

type auditResultL2 struct {
	Suggestion string `json:"suggestion"`
}

func parseAuditCallback(body []byte) (objectKey, suggestion string, err error) {
	var payload auditPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}
	pick := func(values ...string) string {
		for _, v := range values {
			if v != "" {
				return v
			}
		}
		return ""
	}
	objectKey = pick(payload.InputKey, payload.Key, payload.ObjectKey)
	if objectKey == "" {
		return "", "", fmt.Errorf("missing object key")
	}
	if len(payload.Items) > 0 {
		suggestion = pick(payload.Items[0].Result.Result.Suggestion, payload.Items[0].Result.Suggestion)
	}
	if suggestion == "" {
		suggestion = pick(payload.Result.Result.Suggestion, payload.Result.Suggestion, payload.Suggestion)
	}
	if suggestion == "" {
		return "", "", fmt.Errorf("missing suggestion")
	}
	return objectKey, suggestion, nil
}
