package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// aiProviderRow 对应 ai_provider_models 表。
//
// 字段与 testkit/schema/rows.go 的 AIProviderRow 必须逐字一致：
// schema_sql_integration_test 会把两边建出来的表逐列比对（含类型与 IS_NULLABLE）。
type aiProviderRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	Name      string `gorm:"type:varchar(64);not null"`
	Vendor    string `gorm:"type:varchar(32);not null"`
	Model     string `gorm:"type:varchar(128);not null"`
	Enabled   bool   `gorm:"index"`
	Config    string `gorm:"type:longtext;not null"`
}

// TableName 固定表名。
func (aiProviderRow) TableName() string { return "ai_provider_models" }

// ProviderRepo 实现模型配置的持久化端口。
type ProviderRepo struct {
	db *gorm.DB
}

var _ domain.AIProviderRepository = (*ProviderRepo)(nil)

// NewProvider 构造模型配置仓储。
func NewProvider(db *gorm.DB) *ProviderRepo { return &ProviderRepo{db: db} }

// List 实现 domain.AIProviderRepository。
//
// 必须钉主库（dbresolver.Write），不能走默认的从库：服务层"写完立刻 reload"
// （Create/Update/Delete/Enable 都会调 reload），而从库是异步复制，刚插入的
// 那行在主库可见、从库还没有。于是 reload 读到空列表，内存快照被写成空，
// 且此后没有任何路径会再刷新它 —— 表现为后台"添加成功但列表一直空着"，
// 直到重启进程。实测延迟窗口虽短，但 POST 后的 GET 恰好落在里面。
//
// 这张表是后台配置（个位数行），走主库没有压力；AI 调用读的是内存快照，
// 根本不查库，所以这里钉主库不会给主库增加请求路径上的负载。
func (r *ProviderRepo) List(ctx context.Context) ([]domain.AIProvider, error) {
	var rows []aiProviderRow
	if err := r.db.Clauses(dbresolver.Write).WithContext(ctx).Order("created_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AIProvider, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProvider(row))
	}
	return out, nil
}

// Get 实现 domain.AIProviderRepository。
//
// 同样钉主库：它和 List 一样服务"改完立刻读回"的路径，走从库会读到旧值。
func (r *ProviderRepo) Get(ctx context.Context, id ctype.ID) (domain.AIProvider, error) {
	var row aiProviderRow
	err := r.db.Clauses(dbresolver.Write).WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.AIProvider{}, apperr.New(apperr.CodeAIProviderMissing)
		}
		return domain.AIProvider{}, err
	}
	return toProvider(row), nil
}

// Create 实现 domain.AIProviderRepository。
func (r *ProviderRepo) Create(ctx context.Context, item *domain.AIProvider) error {
	if item == nil {
		return apperr.New(apperr.CodeAIProviderInvalid)
	}
	if item.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		item.ID = next
	}
	now := time.Now()
	row := aiProviderRow{
		ID:        item.ID,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      item.Name,
		Vendor:    item.Vendor,
		Model:     item.Model,
		Enabled:   item.Enabled,
		Config:    encodeProviderFields(item.Fields),
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

// Update 实现 domain.AIProviderRepository。
//
// 用显式列更新而不是 Save：Save 会把整行（含 CreatedAt）覆盖回内存里的值，
// 并发下容易把创建时间写成零值。
func (r *ProviderRepo) Update(ctx context.Context, item domain.AIProvider) error {
	res := r.db.WithContext(ctx).
		Model(&aiProviderRow{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"name":       item.Name,
			"vendor":     item.Vendor,
			"model":      item.Model,
			"config":     encodeProviderFields(item.Fields),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeAIProviderMissing)
	}
	return nil
}

// Delete 实现 domain.AIProviderRepository（软删除）。
func (r *ProviderRepo) Delete(ctx context.Context, id ctype.ID) error {
	res := r.db.WithContext(ctx).Delete(&aiProviderRow{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeAIProviderMissing)
	}
	return nil
}

// SetEnabled 实现 domain.AIProviderRepository。
func (r *ProviderRepo) SetEnabled(ctx context.Context, id ctype.ID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&aiProviderRow{}).
			Where("enabled = ?", true).
			Update("enabled", false).Error; err != nil {
			return err
		}
		res := tx.Model(&aiProviderRow{}).
			Where("id = ?", id).
			Updates(map[string]any{"enabled": true, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return apperr.New(apperr.CodeAIProviderMissing)
		}
		return nil
	})
}

// toProvider 把行转成领域实体。
func toProvider(row aiProviderRow) domain.AIProvider {
	return domain.AIProvider{
		ID:      row.ID,
		Name:    row.Name,
		Vendor:  row.Vendor,
		Model:   row.Model,
		Enabled: row.Enabled,
		Fields:  decodeProviderFields(row.Config),
	}
}

// encodeProviderFields 序列化字段表。编码失败不应该写库（会丢密钥），
// 但 map[string]string 实际不可能失败；返回 "{}" 只是兜底。
func encodeProviderFields(fields map[string]string) string {
	if len(fields) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// decodeProviderFields 反序列化字段表；坏数据按空表处理，让后台还能改。
func decodeProviderFields(raw string) map[string]string {
	out := make(map[string]string)
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}
