// Package mysql 是社交模块的 MySQL 适配器。
package mysql

import (
	"context"
	"time"

	"myblogx/internal/platform/idgen"
	platformmysql "myblogx/internal/platform/mysql"
	"myblogx/internal/social/domain"

	"gorm.io/gorm"
)

// followRow 对应 user_follow_models 表。
type followRow struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	FollowedUserID   uint64
	FollowedNickname string
	FollowedAvatar   string
	FollowedAbstract string
	FansUserID       uint64
	FansNickname     string
	FansAvatar       string
	FansAbstract     string
}

// TableName 固定表名。
func (followRow) TableName() string { return "user_follow_models" }

// Repo 实现 domain.Repository。
type Repo struct {
	db *gorm.DB
}

var _ domain.Repository = (*Repo)(nil)

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// Follow 实现 domain.Repository。
func (r *Repo) Follow(ctx context.Context, f *domain.Follow) (bool, error) {
	row := &followRow{
		FollowedUserID:   f.FollowedUserID,
		FollowedNickname: f.FollowedNickname,
		FollowedAvatar:   f.FollowedAvatar,
		FollowedAbstract: f.FollowedAbstract,
		FansUserID:       f.FansUserID,
		FansNickname:     f.FansNickname,
		FansAvatar:       f.FansAvatar,
		FansAbstract:     f.FansAbstract,
	}
	if next, err := idgen.Next(); err == nil {
		row.ID = next.Uint64()
	}
	return platformmysql.RestoreOrCreateUnique(r.db.WithContext(ctx), row, []string{"followed_user_id", "fans_user_id"})
}

// Unfollow 实现 domain.Repository。
func (r *Repo) Unfollow(ctx context.Context, followedID, fansID uint64) (bool, error) {
	res := r.db.WithContext(ctx).
		Where("followed_user_id = ? AND fans_user_id = ?", followedID, fansID).
		Delete(&followRow{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ListFollowing 实现 domain.Repository。
func (r *Repo) ListFollowing(ctx context.Context, ownerID, followedFilter uint64, page domain.Page) ([]domain.Follow, bool, error) {
	page = page.Normalize()
	tx := r.db.WithContext(ctx).Model(&followRow{}).Where("fans_user_id = ?", ownerID)
	if followedFilter != 0 {
		tx = tx.Where("followed_user_id = ?", followedFilter)
	}
	var rows []followRow
	if err := tx.Select("created_at", "followed_user_id", "followed_nickname", "followed_avatar", "followed_abstract").
		Order("created_at asc, id asc").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return toFollows(rows), hasMore, nil
}

// ListFans 实现 domain.Repository。
func (r *Repo) ListFans(ctx context.Context, ownerID, fansFilter uint64, page domain.Page) ([]domain.Follow, bool, error) {
	page = page.Normalize()
	tx := r.db.WithContext(ctx).Model(&followRow{}).Where("followed_user_id = ?", ownerID)
	if fansFilter != 0 {
		tx = tx.Where("fans_user_id = ?", fansFilter)
	}
	var rows []followRow
	if err := tx.Select("created_at", "fans_user_id", "fans_nickname", "fans_avatar", "fans_abstract").
		Order("created_at asc, id asc").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return toFollows(rows), hasMore, nil
}

// RelationBatch 实现 domain.Repository。
func (r *Repo) RelationBatch(ctx context.Context, viewerID uint64, ids []uint64) (map[uint64]domain.Relation, error) {
	out := make(map[uint64]domain.Relation, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		out[id] = domain.RelationStranger
	}

	var rows []followRow
	if err := r.db.WithContext(ctx).
		Where("(followed_user_id = ? AND fans_user_id IN ?) OR (followed_user_id IN ? AND fans_user_id = ?)",
			viewerID, ids, ids, viewerID).
		Find(&rows).Error; err != nil {
		return out, err
	}

	const (
		iFollow  uint8 = 1
		heFollow uint8 = 2
	)
	state := make(map[uint64]uint8, len(ids))
	for _, row := range rows {
		switch {
		case row.FansUserID == viewerID:
			state[row.FollowedUserID] |= iFollow
		case row.FollowedUserID == viewerID:
			state[row.FansUserID] |= heFollow
		}
	}
	for other, s := range state {
		switch s {
		case iFollow:
			out[other] = domain.RelationFollowed
		case heFollow:
			out[other] = domain.RelationFans
		case iFollow | heFollow:
			out[other] = domain.RelationFriend
		}
	}
	return out, nil
}

func toFollows(rows []followRow) []domain.Follow {
	out := make([]domain.Follow, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Follow{
			CreatedAt:        row.CreatedAt,
			FollowedUserID:   row.FollowedUserID,
			FollowedNickname: row.FollowedNickname,
			FollowedAvatar:   row.FollowedAvatar,
			FollowedAbstract: row.FollowedAbstract,
			FansUserID:       row.FansUserID,
			FansNickname:     row.FansNickname,
			FansAvatar:       row.FansAvatar,
			FansAbstract:     row.FansAbstract,
		})
	}
	return out
}
