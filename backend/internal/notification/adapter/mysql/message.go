// Package mysql 是通知模块的 MySQL 适配器。
package mysql

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/domain"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
)

// messageRow 对应 article_message_models 表。
type messageRow struct {
	ID                 ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt
	Type               int8
	ReceiverID         ctype.ID
	ActionUserID       *ctype.ID
	ActionUserNickname *string
	ActionUserAvatar   *string
	Content            string
	ArticleID          ctype.ID
	CommentID          ctype.ID
	ArticleTitle       string
	LinkTitle          string
	LinkHerf           string
	IsRead             bool
	ReadAt             *time.Time
}

// TableName 固定表名。
func (messageRow) TableName() string { return "article_message_models" }

// MessageRepo 实现站内信持久化端口。
type MessageRepo struct {
	db *gorm.DB
}

var _ domain.MessageRepository = (*MessageRepo)(nil)

// NewMessage 构造站内信仓储。
func NewMessage(db *gorm.DB) *MessageRepo { return &MessageRepo{db: db} }

// List 实现 domain.MessageRepository。
func (r *MessageRepo) List(ctx context.Context, receiver uint64, types []domain.MessageType, page domain.Page) ([]domain.Message, bool, error) {
	page = page.Normalize()
	var rows []messageRow
	// 显式排序：旧实现没有 ORDER BY，翻页会出现重复/漏行。
	// 收件箱按时间倒序是唯一合理的稳定顺序。
	err := r.db.WithContext(ctx).
		Where("receiver_id = ? AND type IN ?", receiver, typeNumbers(types)).
		Order("created_at DESC, id DESC").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMessage(row))
	}
	return out, hasMore, nil
}

// Find 实现 domain.MessageRepository。
func (r *MessageRepo) Find(ctx context.Context, receiver, id uint64) (domain.Message, error) {
	var row messageRow
	err := r.db.WithContext(ctx).
		Where("id = ? AND receiver_id = ?", id, receiver).
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.Message{}, apperr.New(apperr.CodeMessageNotFound)
		}
		return domain.Message{}, err
	}
	return toMessage(row), nil
}

// MarkRead 实现 domain.MessageRepository。
func (r *MessageRepo) MarkRead(ctx context.Context, receiver, id uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&messageRow{}).
		Where("id = ? AND receiver_id = ?", id, receiver).
		Updates(map[string]any{"is_read": true, "read_at": &now, "updated_at": now}).Error
}

// MarkGroupRead 实现 domain.MessageRepository。
func (r *MessageRepo) MarkGroupRead(ctx context.Context, receiver uint64, types []domain.MessageType) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&messageRow{}).
		Where("receiver_id = ? AND type IN ? AND is_read = ?", receiver, typeNumbers(types), false).
		Updates(map[string]any{"is_read": true, "read_at": &now, "updated_at": now})
	return res.RowsAffected, res.Error
}

// Delete 实现 domain.MessageRepository。
func (r *MessageRepo) Delete(ctx context.Context, receiver, id uint64) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND receiver_id = ?", id, receiver).
		Delete(&messageRow{}).Error
}

// DeleteGroup 实现 domain.MessageRepository。
func (r *MessageRepo) DeleteGroup(ctx context.Context, receiver uint64, types []domain.MessageType) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("receiver_id = ? AND type IN ?", receiver, typeNumbers(types)).
		Delete(&messageRow{})
	return res.RowsAffected, res.Error
}

// Create 实现 domain.MessageRepository。
func (r *MessageRepo) Create(ctx context.Context, m *domain.Message) error {
	if m == nil {
		return apperr.New(apperr.CodeMessageNotFound)
	}
	if m.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		m.ID = next
	}
	now := time.Now()
	row := messageRow{
		ID:           m.ID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Type:         int8(m.Type),
		ReceiverID:   ctype.NewID(m.Receiver),
		Content:      m.Content,
		ArticleID:    m.ArticleID,
		CommentID:    m.CommentID,
		ArticleTitle: m.ArticleTitle,
		LinkTitle:    m.LinkTitle,
		LinkHerf:     m.LinkHref,
		IsRead:       false,
	}
	if m.ActionUser != 0 {
		actionUser := ctype.NewID(m.ActionUser)
		nickname := m.ActionUserNickname
		avatar := m.ActionUserAvatar
		row.ActionUserID = &actionUser
		row.ActionUserNickname = &nickname
		row.ActionUserAvatar = &avatar
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	m.CreatedAt = now
	m.UpdatedAt = now
	return nil
}

// ExistsDigg 实现 domain.MessageRepository。
//
// 旧实现的去重条件分两种：评论点赞按 comment_id，文章点赞/收藏按 article_id。
func (r *MessageRepo) ExistsDigg(ctx context.Context, actionUser uint64, typ domain.MessageType, articleID, commentID uint64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&messageRow{}).
		Where("action_user_id = ? AND type = ?", actionUser, int8(typ))
	if commentID != 0 {
		query = query.Where("comment_id = ?", commentID)
	} else {
		query = query.Where("article_id = ?", articleID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CountUnread 实现 domain.MessageRepository。
func (r *MessageRepo) CountUnread(ctx context.Context, receiver uint64) (map[domain.MessageType]int64, error) {
	type countRow struct {
		Type  int8
		Count int64
	}
	var rows []countRow
	err := r.db.WithContext(ctx).Model(&messageRow{}).
		Select("type, COUNT(*) AS count").
		Where("receiver_id = ? AND is_read = ?", receiver, false).
		Group("type").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[domain.MessageType]int64, len(rows))
	for _, row := range rows {
		out[domain.MessageType(row.Type)] += row.Count
	}
	return out, nil
}

// toMessage 把表行转成领域实体。
func toMessage(row messageRow) domain.Message {
	out := domain.Message{
		ID:           row.ID,
		Type:         domain.MessageType(row.Type),
		Receiver:     row.ReceiverID.Uint64(),
		Content:      row.Content,
		ArticleID:    row.ArticleID,
		CommentID:    row.CommentID,
		ArticleTitle: row.ArticleTitle,
		LinkTitle:    row.LinkTitle,
		LinkHref:     row.LinkHerf,
		IsRead:       row.IsRead,
		ReadAt:       row.ReadAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if row.ActionUserID != nil {
		out.ActionUser = (*row.ActionUserID).Uint64()
	}
	if row.ActionUserNickname != nil {
		out.ActionUserNickname = *row.ActionUserNickname
	}
	if row.ActionUserAvatar != nil {
		out.ActionUserAvatar = *row.ActionUserAvatar
	}
	return out
}

// typeNumbers 把领域类型转成表里的整数。
func typeNumbers(types []domain.MessageType) []int8 {
	out := make([]int8, 0, len(types))
	for _, typ := range types {
		out = append(out, int8(typ))
	}
	return out
}
