// Package app 是媒体（图片）模块的用例层。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"myblogx/internal/kernel/ctype"
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
	TaskID             ctype.ID
	UserID             ctype.ID
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
	cfg     func() domain.Config
	// violations 是"判定违规 → 违规计分"的上报口（ADR-0035），可为 nil。
	violations ViolationRecorder
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
	cfg func() domain.Config,
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
	if cfg == nil {
		panic("media/app: 配置未注入")
	}
	return &Service{repo: repo, tasks: tasks, audits: audits, storage: storage, ids: ids, clock: clock, logger: logger, cfg: cfg}
}

// CreateTask 创建直传上传任务；相同哈希直接秒传。
//
// purpose 是请求里声明的用途字面量（cover / avatar）的指针：nil 表示请求体
// 没带该字段（例如私信发图是任意比例照片），这类任务不记录目标比例、确认时
// 不做比例约束；一旦带了字段就必须合法（含空串），非法直接按明确错误码拒绝，
// 不静默当作 cover。请求体与领域规则的边界放在这里一处，适配器只负责转发。
func (s *Service) CreateTask(ctx context.Context, userID uint64, fileName string, size int64, mimeType, hash string, purpose *string) (*CreateResult, error) {
	q := s.cfg()
	if !q.Enabled || q.Bucket == "" || q.SizeMB <= 0 || q.ExpirySeconds <= 0 {
		return nil, apperr.New(apperr.CodeImageUploadDisabled)
	}

	var declaredPurpose domain.Purpose
	var targetRatio domain.Ratio
	if purpose != nil {
		parsed, ok := domain.ParsePurpose(*purpose)
		if !ok {
			return nil, apperr.New(apperr.CodeImagePurposeInvalid)
		}
		ratio, ok := parsed.TargetRatio()
		if !ok {
			return nil, apperr.New(apperr.CodeImagePurposeInvalid)
		}
		declaredPurpose = parsed
		targetRatio = ratio
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
		ID:           ctype.NewID(s.ids.NextTaskID()),
		UserID:       ctype.NewID(userID),
		Provider:     domain.ProviderQiniu,
		Status:       domain.TaskPending,
		Bucket:       q.Bucket,
		ObjectKey:    buildObjectKey(q.Prefix, hash, now),
		OriginalName: fileName,
		DeclaredMime: mimeType,
		DeclaredSize: size,
		Hash:         hash,
		Purpose:      declaredPurpose,
		TargetRatio:  targetRatio,
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
		if task.UserID.Uint64() != userID {
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
	if task.UserID.Uint64() != userID {
		return nil, apperr.New(apperr.CodeImageTaskNotFound)
	}
	result := &ConfirmResult{Task: task}
	if task.ImageID != nil {
		// 必须以图行里的**当前**审核状态为准。任务上的 ImageURL 只是入库那一刻的
		// 快照，而审核（同步 CensorNow / 七牛回调 / 超时扫描）都在任务落盘之后才
		// 有结论；照快照拼一个 Image 出来，状态会一直是 0（unknown），于是客户端
		// 拿 "unknown" 当结论 —— 2026-11-27 上传违规图时弹出「这张图状态异常」。
		image, err := s.repo.FindByID(ctx, ctype.NewID((*task.ImageID).Uint64()))
		if err == nil {
			result.Image = &image
		} else if task.ImageURL != "" {
			// 图行已经不在了（运营物理清理等）：只能回快照，状态如实按未知处理。
			result.Image = &domain.Image{ID: ctype.NewID((*task.ImageID).Uint64()), URL: task.ImageURL}
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
		image, err := s.repo.FindByID(ctx, ctype.NewID((*task.ImageID).Uint64()))
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
		// 对外只回 apperr 的中文文案（"读取图片信息失败…"），排查需要的原因
		// （fop 与字节两条通道各自为什么失败）只能落在这里 —— 任务上的
		// ErrorMsg 存的是文案，不存 cause，2026-11-26 那次事故就靠这条日志定位。
		if s.logger != nil {
			s.logger.Warnf("读取图片信息失败: task=%d key=%s err=%v", task.ID, task.ObjectKey, err)
		}
		return nil, err
	}
	format := strings.ToLower(strings.TrimSpace(imageInfo.Format))
	if !whitelistContains(s.cfg().Whitelist, format) {
		return nil, apperr.New(apperr.CodeImageInvalid)
	}
	// 比例校验与格式/尺寸同在一条 verify 链：回调与手动确认都经过这里，
	// 不会有一套校验跑在另一条路径上。未声明用途的任务没有目标比例，
	// 跳过本步（私信发图是任意比例照片，不能被封面规则误判）。
	if task.TargetRatio != (domain.Ratio{}) && !task.TargetRatio.Matches(imageInfo.Width, imageInfo.Height) {
		return nil, apperr.New(apperr.CodeImageRatioMismatch)
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

// initialStatus 返回新图片的初始状态。
//
// 这里是"合规闸门"的入口：审核开启时新图一律 pending（不可见），
// 只有审核给出 pass 才变可见。关掉审核等于恢复"上传即可见"的老行为。
func (s *Service) initialStatus() domain.Status {
	if s.cfg().AuditEnabled {
		return domain.StatusPending
	}
	return domain.StatusPass
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
			// 审核关闭时保持老行为（入库即可见）；开启时入库为待审核，
			// 只有拿到 pass 结论才允许展示（ADR-0034）。
			Status: s.initialStatus(),
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
	imageID := image.ID
	task.ImageID = &imageID
	task.ImageURL = image.URL

	if err := s.tasks.Save(ctx, task, finalizedKeepAlive); err != nil && s.logger != nil {
		s.logger.Warnf("保存完成的上传任务状态失败: task=%d err=%v", task.ID, err)
	}
	s.applyPendingAudit(ctx, &image)

	// 主动审核兜底：回调可能永远不来（域名封禁期回调地址进不来、或控制台漏配），
	// 所以入库后立刻自己跑一次审核。失败只记日志，图保持待审核转人工队列。
	//
	// 同步调用是有意的：私信发图、设置封面这类场景需要"立刻知道能不能用"，
	// 异步化会把不确定性推给用户。单次调用是秒级，落在一次 HTTP 请求里可接受。
	if image.Status == domain.StatusPending {
		if err := s.CensorNow(ctx, &image); err != nil && s.logger != nil {
			s.logger.Warnf("入库后主动审核未完成（图保持待审核）: image=%d err=%v", image.ID, err)
		}
	}

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
	if !canTransition(image.Status, status) {
		return
	}
	if err := s.applyStatus(ctx, image, status, auditSourcePendingCallback); err != nil && s.logger != nil {
		s.logger.Warnf("应用审核结果失败: image=%d err=%v", image.ID, err)
	}
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
	if !canTransition(image.Status, status) {
		return nil
	}
	return s.applyStatus(ctx, &image, status, auditSourceCallback)
}

// 审核结论的来源。写进日志与（后续的）审计列，用来回答"这张图为什么是这个状态"。
const (
	auditSourceCallback        = "callback"
	auditSourcePendingCallback = "callback-pending"
	auditSourceActive          = "api"
	auditSourceTimeout         = "timeout"
	auditSourceManual          = "manual"
)

// canTransition 判断审核结论能否覆盖当前状态。
//
// 两条硬规则：
//  1. **blocked 是终局**。任何来源都不能把它改回 pass —— 否则一次乱序回调
//     （先 block 后 pass）就把违规图放出去了。违规图的对象在判定时已被删除，
//     "放行"本来也无从谈起。
//  2. **同状态不重复写**。回调会重试，重复写只是浪费一次 UPDATE。
func canTransition(current, next domain.Status) bool {
	if current == next {
		return false
	}
	if current == domain.StatusBlocked {
		return false
	}
	if next == domain.StatusUnknown {
		return false
	}
	return true
}

// applyStatus 落一次状态变更，并处理"判定违规即删除对象"这条副作用。
//
// 删对象失败**不让状态回滚**：图已经是违规的，必须立刻不可见。
// 失败只记日志并保持 object_deleted_at 为空，由重试扫描补删（见 T4）。
func (s *Service) applyStatus(ctx context.Context, image *domain.Image, status domain.Status, source string) error {
	if err := s.repo.UpdateStatus(ctx, image.ID, status); err != nil {
		return err
	}
	image.Status = status

	// 判定违规先上报计分：删对象可能失败，但"这个人违规了"这件事已经成立。
	if status == domain.StatusBlocked {
		s.recordViolation(ctx, image)
	}

	if status != domain.StatusBlocked || !s.cfg().AuditAutoDelete {
		return nil
	}
	if err := s.storage.Delete(image.Bucket, image.ObjectKey); err != nil {
		if s.logger != nil {
			s.logger.Warnf("删除违规图对象失败（待重试）: image=%d key=%s source=%s err=%v",
				image.ID, image.ObjectKey, source, err)
		}
	}
	return nil
}

// List 图片列表。
func (s *Service) List(ctx context.Context, q domain.Query) ([]domain.Image, int64, error) {
	return s.repo.List(ctx, q)
}

// Remove 批量删除图片：先删对象存储，再删引用与记录。
func (s *Service) Remove(ctx context.Context, ids []domain.ImageID) (int, error) {
	return removeImages(ctx, s.repo, s.storage, ids)
}

// removeImages 是"先删对象、确认成功才清账本并硬删行"的唯一实现。
//
// 单独抽出来是为了让维护命令（孤儿图真删）复用同一段删除顺序，
// 而不是另造一套：任何一处顺序写反都会留下"行没了对象还在"或
// "对象没了行还在"的半删状态。
func removeImages(ctx context.Context, repo domain.Repository, storage domain.ObjectStorage, ids []domain.ImageID) (int, error) {
	images, err := repo.FindByIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	if len(images) == 0 {
		return 0, apperr.New(apperr.CodeImageUnavailable)
	}
	for _, image := range images {
		// 先删存储对象。失败就整条返回：此时图片行、账本行都还没动，
		// 下次重试看到的仍是完整的一条，不会出现"本地行没了对象还在"。
		if err := storage.Delete(image.Bucket, image.ObjectKey); err != nil {
			return 0, apperr.Wrap(apperr.CodeImageDeleteFailed, err)
		}
	}
	// 对象确认删掉后才删引用账本与图片行（同一个事务）。
	if err := repo.DeleteWithRefs(ctx, ids); err != nil {
		return 0, err
	}
	return len(images), nil
}

// VerifyCallback 校验回调签名。
func (s *Service) VerifyCallback(req *http.Request) (bool, error) {
	return s.storage.VerifyCallback(req)
}

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
