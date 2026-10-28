package domain

import (
	"context"

	"myblogx/internal/kernel/ctype"
)

// AIProvider 是一条模型配置。
//
// Fields 存供应商特有字段（api_key / base_url / region …），键集合由 app 层的
// 供应商元数据（vendors.go）定义。为什么不列化：各家需要的字段不同，而项目的
// -db 只执行 CREATE TABLE IF NOT EXISTS、没有 ALTER 通道，每加一个供应商就
// 改一次表结构是不可接受的。
type AIProvider struct {
	ID      ctype.ID
	Name    string
	Vendor  string
	Model   string
	Enabled bool
	Fields  map[string]string
}

// AIProviderRepository 是模型配置的持久化端口。
type AIProviderRepository interface {
	// List 返回全部未删除的配置，按创建时间升序。
	List(ctx context.Context) ([]AIProvider, error)
	// Get 读一条；不存在时返回 apperr.CodeAIProviderMissing。
	Get(ctx context.Context, id ctype.ID) (AIProvider, error)
	Create(ctx context.Context, item *AIProvider) error
	Update(ctx context.Context, item AIProvider) error
	Delete(ctx context.Context, id ctype.ID) error
	// SetEnabled 启用指定的一条，同时把其余置为停用。
	//
	// 放在同一个事务里：否则中途失败会出现两条同时启用，或者一条都没有 ——
	// 后者会让站内 AI 直接不可用，而界面上看不出原因。
	SetEnabled(ctx context.Context, id ctype.ID) error
}
