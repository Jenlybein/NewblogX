package mysql

import (
	"context"
	"testing"

	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

func newProviderRepo(t *testing.T) *ProviderRepo {
	t.Helper()
	return NewProvider(testutil.SetupSQLite(t, &schema.AIProviderRow{}))
}

// TestProviderRepo_Lifecycle 覆盖新增 → 读 → 改 → 软删除的整条路径。
func TestProviderRepo_Lifecycle(t *testing.T) {
	repo := newProviderRepo(t)
	ctx := context.Background()

	item := domain.AIProvider{
		Name:   "主力模型",
		Vendor: "openai",
		Model:  "gpt-4.1-mini",
		Fields: map[string]string{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}
	if err := repo.Create(ctx, &item); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if item.ID.IsZero() {
		t.Fatal("新增后应回填主键")
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Fields["api_key"] != "sk-test" || got.Model != "gpt-4.1-mini" {
		t.Fatalf("读取结果不对: %+v", got)
	}

	got.Name = "改名了"
	got.Fields["base_url"] = "https://proxy.example.com/v1"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	after, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if after.Name != "改名了" || after.Fields["base_url"] != "https://proxy.example.com/v1" {
		t.Fatalf("更新未生效: %+v", after)
	}

	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("软删除后列表长度 = %d，期望 0", len(list))
	}
	if _, err := repo.Get(ctx, item.ID); !apperr.IsCode(err, apperr.CodeAIProviderMissing) {
		t.Fatalf("读取已删除项 err = %v，期望 CodeAIProviderMissing", err)
	}
}

// TestProviderRepo_SetEnabledIsExclusive 验证启用一条时其余自动停用，
// 且这个切换在同一个事务里完成（不会出现两条同时为真）。
func TestProviderRepo_SetEnabledIsExclusive(t *testing.T) {
	repo := newProviderRepo(t)
	ctx := context.Background()

	first := domain.AIProvider{Name: "a", Vendor: "openai", Model: "m1", Enabled: true, Fields: map[string]string{"api_key": "k1"}}
	second := domain.AIProvider{Name: "b", Vendor: "openai", Model: "m2", Fields: map[string]string{"api_key": "k2"}}
	if err := repo.Create(ctx, &first); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if err := repo.Create(ctx, &second); err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	if err := repo.SetEnabled(ctx, second.ID); err != nil {
		t.Fatalf("启用失败: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	enabled := make([]ctype.ID, 0, 1)
	for _, item := range list {
		if item.Enabled {
			enabled = append(enabled, item.ID)
		}
	}
	if len(enabled) != 1 || enabled[0] != second.ID {
		t.Fatalf("启用集合 = %v，期望只有 %v", enabled, second.ID)
	}
}

// TestProviderRepo_SetEnabledMissing 验证启用不存在的行会报错（前端拿到可读提示）。
func TestProviderRepo_SetEnabledMissing(t *testing.T) {
	repo := newProviderRepo(t)
	if err := repo.SetEnabled(context.Background(), ctype.NewID(9999)); !apperr.IsCode(err, apperr.CodeAIProviderMissing) {
		t.Fatalf("err = %v，期望 CodeAIProviderMissing", err)
	}
}

// TestProviderRepo_FieldsRoundTrip 验证字段 JSON 往返不丢内容（含中文与斜杠）。
func TestProviderRepo_FieldsRoundTrip(t *testing.T) {
	repo := newProviderRepo(t)
	ctx := context.Background()

	fields := map[string]string{
		"api_key":    "sk-a/b+c=",
		"base_url":   "https://example.com/v1/chat",
		"access_key": "中文键值",
		"region":     "cn-beijing",
	}
	item := domain.AIProvider{Name: "往返", Vendor: "ark", Model: "ep-1", Fields: fields}
	if err := repo.Create(ctx, &item); err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	for key, want := range fields {
		if got.Fields[key] != want {
			t.Fatalf("字段 %s = %q，期望 %q", key, got.Fields[key], want)
		}
	}
}

// TestProviderRepo_ListReadsPrimary 钉住"读完必须走主库"这条要求。
//
// 背景是一起真实事故：线上是 gorm dbresolver 读写分离，读请求默认落到从库。
// 服务层每次写完都会 reload（Create/Update/Delete/Enable 都调），而从库是
// **异步复制** —— 刚插入的那行在主库可见、从库还没有。于是 reload 读到空，
// 把内存快照覆盖成空，此后没有任何路径再刷新它。现象是后台"添加配置成功"
// 但列表一直空着，直到重启进程。
//
// 这里用两个独立的内存库模拟主从：sources 建表并写入，replicas 也建表但
// 永远不写数据（等价于"复制还没追上"）。如果有人去掉 dbresolver.Write，
// List 会走 replicas 读到 0 条，本用例立刻失败。
func TestProviderRepo_ListReadsPrimary(t *testing.T) {
	primary := testutil.SetupSQLite(t, &schema.AIProviderRow{})
	replica := testutil.SetupSQLite(t, &schema.AIProviderRow{})

	if err := primary.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{primary.Dialector},
		Replicas: []gorm.Dialector{replica.Dialector},
		Policy:   dbresolver.RandomPolicy{},
	})); err != nil {
		t.Fatalf("注册读写分离失败: %v", err)
	}

	repo := NewProvider(primary)
	ctx := context.Background()

	item := domain.AIProvider{Name: "主库读", Vendor: "openai", Model: "gpt-4.1-mini"}
	if err := repo.Create(ctx, &item); err != nil {
		t.Fatalf("新增失败: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("写完后立刻读到 %d 条，期望 1 条：List 走了从库，会被复制延迟坑掉", len(list))
	}
}
