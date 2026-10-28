package app

import (
	"context"
	"testing"

	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// fakeRepo 是内存版仓储，让用例只验证服务层行为，不牵扯 SQL。
type fakeRepo struct {
	items  []domain.AIProvider
	seq    uint64
	deleted map[ctype.ID]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{seq: 1000, deleted: map[ctype.ID]bool{}}
}

func (f *fakeRepo) List(context.Context) ([]domain.AIProvider, error) {
	out := make([]domain.AIProvider, 0, len(f.items))
	for _, item := range f.items {
		if f.deleted[item.ID] {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, id ctype.ID) (domain.AIProvider, error) {
	for _, item := range f.items {
		if item.ID == id && !f.deleted[id] {
			return item, nil
		}
	}
	return domain.AIProvider{}, apperr.New(apperr.CodeAIProviderMissing)
}

func (f *fakeRepo) Create(_ context.Context, item *domain.AIProvider) error {
	f.seq++
	item.ID = ctype.NewID(f.seq)
	f.items = append(f.items, *item)
	return nil
}

func (f *fakeRepo) Update(_ context.Context, item domain.AIProvider) error {
	for i := range f.items {
		if f.items[i].ID == item.ID {
			f.items[i].Name = item.Name
			f.items[i].Vendor = item.Vendor
			f.items[i].Model = item.Model
			f.items[i].Fields = item.Fields
			return nil
		}
	}
	return apperr.New(apperr.CodeAIProviderMissing)
}

func (f *fakeRepo) Delete(_ context.Context, id ctype.ID) error {
	f.deleted[id] = true
	return nil
}

func (f *fakeRepo) SetEnabled(_ context.Context, id ctype.ID) error {
	found := false
	for i := range f.items {
		f.items[i].Enabled = f.items[i].ID == id
		if f.items[i].ID == id {
			found = true
		}
	}
	if !found {
		return apperr.New(apperr.CodeAIProviderMissing)
	}
	return nil
}

func newTestService(t *testing.T) (*ProviderService, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	svc := NewProviderService(repo, nil)
	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	return svc, repo
}

func openAIInput() ProviderInput {
	return ProviderInput{
		Vendor: "openai",
		Model:  "gpt-4.1-mini",
		Fields: map[string]string{"api_key": "sk-test"},
	}
}

// TestCreate_FillsDefaults 验证留空的接口地址会被默认值补齐（后台看到的就是实际用的）。
func TestCreate_FillsDefaults(t *testing.T) {
	svc, _ := newTestService(t)
	created, err := svc.Create(context.Background(), openAIInput())
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if got := created.Fields["base_url"]; got != "https://api.openai.com/v1" {
		t.Fatalf("base_url = %q，期望默认值", got)
	}
	if created.Name == "" {
		t.Fatal("未填备注名时应自动生成")
	}
}

// TestCreate_RejectsMissingKey 验证必填字段缺失时报错。
func TestCreate_RejectsMissingKey(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), ProviderInput{Vendor: "openai", Model: "m"})
	if !apperr.IsCode(err, apperr.CodeAIProviderInvalid) {
		t.Fatalf("err = %v, want CodeAIProviderInvalid", err)
	}
}

// TestCreate_RejectsUnknownVendor 验证未知供应商被拒。
func TestCreate_RejectsUnknownVendor(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), ProviderInput{Vendor: "nope", Model: "m", Fields: map[string]string{"api_key": "k"}})
	if !apperr.IsCode(err, apperr.CodeAIVendorUnsupported) {
		t.Fatalf("err = %v, want CodeAIVendorUnsupported", err)
	}
}

// TestCreate_EnabledBecomesActive 验证勾选启用后 Active 指向它。
func TestCreate_EnabledBecomesActive(t *testing.T) {
	svc, _ := newTestService(t)
	in := openAIInput()
	in.Enabled = true
	created, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	active := svc.Active()
	if active == nil || active.ID != created.ID.String() {
		t.Fatalf("Active = %+v，期望指向刚建的那条", active)
	}
}

// TestEnable_SwitchesSingle 验证启用第二条时第一条被停用（全局只能有一条）。
func TestEnable_SwitchesSingle(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	first := openAIInput()
	first.Enabled = true
	createdFirst, err := svc.Create(ctx, first)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	second := openAIInput()
	second.Model = "gpt-4.1"
	createdSecond, err := svc.Create(ctx, second)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	if err := svc.Enable(ctx, createdSecond.ID); err != nil {
		t.Fatalf("启用失败: %v", err)
	}

	enabled := 0
	for _, item := range repo.items {
		if item.Enabled {
			enabled++
			if item.ID != createdSecond.ID {
				t.Fatalf("启用的是 %v，期望 %v", item.ID, createdSecond.ID)
			}
		}
	}
	if enabled != 1 {
		t.Fatalf("同时启用了 %d 条，期望 1 条", enabled)
	}
	active := svc.Active()
	if active == nil || active.Model != "gpt-4.1" {
		t.Fatalf("Active 未切到第二条: %+v", active)
	}
	_ = createdFirst
}

// TestList_MasksSecrets 验证列表里密钥被遮蔽、非敏感字段保留。
func TestList_MasksSecrets(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Create(context.Background(), openAIInput()); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	list := svc.List()
	if len(list) != 1 {
		t.Fatalf("列表长度 = %d，期望 1", len(list))
	}
	if got := list[0].Fields["api_key"]; got != sensitivePlaceholder {
		t.Fatalf("api_key = %q，期望被遮蔽", got)
	}
	if got := list[0].Fields["base_url"]; got == "" || got == sensitivePlaceholder {
		t.Fatalf("base_url 不该被遮蔽，得到 %q", got)
	}
}

// TestUpdate_KeepsSecretOnPlaceholder 验证回传占位符时沿用旧密钥。
func TestUpdate_KeepsSecretOnPlaceholder(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, openAIInput())
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	in := ProviderInput{
		Name:   "改个名",
		Vendor: "openai",
		Model:  "gpt-4.1",
		// 前端会把遮蔽后的值原样回传
		Fields: map[string]string{"api_key": sensitivePlaceholder, "base_url": "https://proxy.example.com/v1"},
	}
	if err := svc.Update(ctx, created.ID, in); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	stored, err := svc.rawItem(created.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if stored.Fields["api_key"] != "sk-test" {
		t.Fatalf("api_key = %q，占位符应保留原值", stored.Fields["api_key"])
	}
	if stored.Model != "gpt-4.1" {
		t.Fatalf("model = %q，期望已更新", stored.Model)
	}
	_ = repo
}

// TestDelete_RemovesFromList 验证软删除后列表与 Active 都不再包含它。
func TestDelete_RemovesFromList(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	in := openAIInput()
	in.Enabled = true
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if got := len(svc.List()); got != 0 {
		t.Fatalf("删除后列表长度 = %d，期望 0", got)
	}
	if svc.Active() != nil {
		t.Fatal("删除后不该还有启用中的配置")
	}
}

// TestVendors_CoverAllSupported 验证元数据与 llm 层认识的供应商一一对应。
func TestVendors_CoverAllSupported(t *testing.T) {
	specs := Vendors()
	if len(specs) != 8 {
		t.Fatalf("供应商数量 = %d，期望 8", len(specs))
	}
	for _, spec := range specs {
		if _, ok := VendorByID(spec.ID); !ok {
			t.Fatalf("VendorByID 查不到 %s", spec.ID)
		}
		if len(spec.Fields) == 0 {
			t.Fatalf("%s 没有字段定义", spec.ID)
		}
		hasSecret := false
		for _, field := range spec.Fields {
			if field.Secret {
				hasSecret = true
			}
		}
		if !hasSecret {
			t.Fatalf("%s 没有任何敏感字段，密钥会明文回显", spec.ID)
		}
	}
}
