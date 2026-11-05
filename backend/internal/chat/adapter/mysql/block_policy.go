package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// chatBlockRow 对应 chat_block_models（ADR-0026 唯一获准新增的私信表）。
type chatBlockRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	OwnerID   ctype.ID
	BlockedID ctype.ID
	Level     uint8
}

func (chatBlockRow) TableName() string { return "chat_block_models" }

// BlockPolicy 读私信的屏蔽 / 拉黑关系。
//
// 只做读：写入（屏蔽 / 拉黑 / 解除）由后台用户管理或后续接口负责，本模块
// 不提供写端口 —— 用例层只需要"能不能发 / 该不该隐藏"这两个结论。
type BlockPolicy struct {
	db *gorm.DB
}

var _ domain.BlockPolicy = (*BlockPolicy)(nil)

// NewBlockPolicy 构造读策略。
func NewBlockPolicy(db *gorm.DB) *BlockPolicy { return &BlockPolicy{db: db} }

// Level 返回 owner 对 target 的级别；没有记录（或已软删解除）时为 None。
func (p *BlockPolicy) Level(ctx context.Context, ownerID, targetID ctype.ID) (domain.BlockLevel, error) {
	if p == nil || p.db == nil {
		return domain.BlockLevelNone, nil
	}
	var row chatBlockRow
	err := p.db.WithContext(ctx).
		Select("level").
		Where("owner_id = ? AND blocked_id = ?", ownerID.Uint64(), targetID.Uint64()).
		Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.BlockLevelNone, nil
	}
	if err != nil {
		return domain.BlockLevelNone, err
	}
	if row.Level == uint8(domain.BlockLevelHard) {
		return domain.BlockLevelHard, nil
	}
	return domain.BlockLevelSoft, nil
}
