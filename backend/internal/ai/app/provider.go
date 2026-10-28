package app

import (
	"context"
	"strings"
	"sync"

	"myblogx/conf"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// sensitivePlaceholder 是密钥回显用的占位符。
//
// 与站点配置的 domain.SensitivePlaceholder 同值不同包：AI 模块不 import
// site 模块（跨模块依赖被 R11 拦）。约定一致：前端原样回传 = 没改。
const sensitivePlaceholder = "******"

// ProviderInput 是新增/修改一条配置的入参。
type ProviderInput struct {
	Name    string
	Vendor  string
	Model   string
	Fields  map[string]string
	Enabled bool
}

// ProviderService 管理模型配置列表，并在内存里维护"当前启用的是哪条"。
//
// 为什么要有缓存：站内 AI 每次调用都要问"用哪个模型"，而这条信息在数据库里。
// 如果每次调用都查库，等于把一次 LLM 请求的开销再加一次磁盘 IO；更要紧的是
// 它发生在请求路径上，一旦库抖一下，AI 就整体不可用。
type ProviderService struct {
	repo     domain.AIProviderRepository
	logger   Logger
	onChange func()

	mu     sync.RWMutex
	items  []domain.AIProvider
	active *conf.AIProvider
}

// NewProviderService 构造服务。
func NewProviderService(repo domain.AIProviderRepository, logger Logger) *ProviderService {
	return &ProviderService{repo: repo, logger: logger}
}

// WithOnChange 注入"配置变更后"的回调。装配层用它清掉 eino 模型缓存。
func (s *ProviderService) WithOnChange(fn func()) *ProviderService {
	s.onChange = fn
	return s
}

// Load 从库里重建内存快照。启动时调用一次。
func (s *ProviderService) Load(ctx context.Context) error {
	return s.reload(ctx)
}

// Active 返回启用中的那条配置。纯内存读，零 IO。
//
// 返回 nil 表示没有任何启用的配置 —— 调用方据此报 CodeAIProviderMissing，
// 而不是拿一个空配置去发请求（那只会得到一句上游的 401）。
func (s *ProviderService) Active() *conf.AIProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active == nil {
		return nil
	}
	clone := *s.active
	return &clone
}

// List 返回全部配置，密钥字段已遮蔽。
func (s *ProviderService) List() []domain.AIProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.AIProvider, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, s.mask(item))
	}
	return out
}

// Get 读一条（遮蔽后），主要给前端编辑用。
func (s *ProviderService) Get(_ context.Context, id ctype.ID) (domain.AIProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.ID == id {
			return s.mask(item), nil
		}
	}
	return domain.AIProvider{}, apperr.New(apperr.CodeAIProviderMissing)
}

// Create 新增一条配置。
func (s *ProviderService) Create(ctx context.Context, in ProviderInput) (domain.AIProvider, error) {
	_, item, err := s.buildAndValidate(in, nil)
	if err != nil {
		return domain.AIProvider{}, err
	}

	if err := s.repo.Create(ctx, &item); err != nil {
		return domain.AIProvider{}, err
	}
	// 新增时勾了"启用"，要在插入之后单独走一次（SetEnabled 依赖行已存在）。
	if in.Enabled {
		if err := s.repo.SetEnabled(ctx, item.ID); err != nil {
			return domain.AIProvider{}, err
		}
	}
	if err := s.reload(ctx); err != nil {
		return domain.AIProvider{}, err
	}
	return s.mask(item), nil
}

// Update 修改一条配置。
//
// 密钥字段传回占位符表示"没改"，此时沿用库里的旧值 —— 前端每次保存都会把
// 整个对象回传，不做回滚就会把密钥写成 ******。
func (s *ProviderService) Update(ctx context.Context, id ctype.ID, in ProviderInput) error {
	current, err := s.rawItem(id)
	if err != nil {
		return err
	}
	_, item, err := s.buildAndValidate(in, &current)
	if err != nil {
		return err
	}
	item.ID = id

	if err := s.repo.Update(ctx, item); err != nil {
		return err
	}
	if err := s.reload(ctx); err != nil {
		return err
	}
	return nil
}

// Delete 删除一条配置（软删除）。
func (s *ProviderService) Delete(ctx context.Context, id ctype.ID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	return s.reload(ctx)
}

// Enable 启用一条配置，其余自动停用。
func (s *ProviderService) Enable(ctx context.Context, id ctype.ID) error {
	if err := s.repo.SetEnabled(ctx, id); err != nil {
		return err
	}
	return s.reload(ctx)
}

// Vendors 返回供应商元数据（前端据此渲染动态表单）。
func (s *ProviderService) Vendors() []VendorSpec { return Vendors() }

// buildAndValidate 做三件事：查供应商元数据、补齐默认值、校验必填。
//
// previous 非空表示这是修改：密钥字段等于占位符时沿用旧值。
func (s *ProviderService) buildAndValidate(in ProviderInput, previous *domain.AIProvider) (VendorSpec, domain.AIProvider, error) {
	spec, ok := VendorByID(in.Vendor)
	if !ok {
		return VendorSpec{}, domain.AIProvider{}, apperr.New(apperr.CodeAIVendorUnsupported)
	}

	fields := spec.WithDefaults(in.Fields)
	if previous != nil {
		for _, key := range spec.SecretKeys() {
			if strings.TrimSpace(fields[key]) == sensitivePlaceholder {
				fields[key] = previous.Fields[key]
			}
		}
	}
	if err := spec.Validate(fields); err != nil {
		return VendorSpec{}, domain.AIProvider{}, err
	}

	// model 有独立列，只从入参取；Fields 里不会有它（Validate 也跳过它）。
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return VendorSpec{}, domain.AIProvider{}, apperr.New(apperr.CodeAIProviderInvalid)
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = spec.Name + " · " + model
	}

	return spec, domain.AIProvider{
		Name:    name,
		Vendor:  spec.ID,
		Model:   model,
		Enabled: in.Enabled,
		Fields:  fields,
	}, nil
}

// rawItem 取未遮蔽的一条（内部用，不能给前端）。
func (s *ProviderService) rawItem(id ctype.ID) (domain.AIProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.AIProvider{}, apperr.New(apperr.CodeAIProviderMissing)
}

// mask 把敏感字段替换成占位符。
func (s *ProviderService) mask(item domain.AIProvider) domain.AIProvider {
	spec, ok := VendorByID(item.Vendor)
	if !ok {
		return item
	}
	fields := make(map[string]string, len(item.Fields))
	for key, value := range item.Fields {
		fields[key] = value
	}
	for _, key := range spec.SecretKeys() {
		if strings.TrimSpace(fields[key]) != "" {
			fields[key] = sensitivePlaceholder
		}
	}
	item.Fields = fields
	return item
}

// reload 重建内存快照并通知外部。
func (s *ProviderService) reload(ctx context.Context) error {
	items, err := s.repo.List(ctx)
	if err != nil {
		return err
	}

	var active *conf.AIProvider
	for _, item := range items {
		if item.Enabled {
			active = &conf.AIProvider{
				ID:     item.ID.String(),
				Name:   item.Name,
				Vendor: item.Vendor,
				Model:  item.Model,
				Fields: item.Fields,
			}
			break
		}
	}

	s.mu.Lock()
	s.items = items
	s.active = active
	s.mu.Unlock()

	if s.onChange != nil {
		s.onChange()
	}
	return nil
}
