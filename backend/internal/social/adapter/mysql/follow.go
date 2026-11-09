// Package mysql 是社交模块的 MySQL 适配器。
package mysql

import (
	"context"
	"time"

	"myblogx/internal/platform/idgen"
	platformmysql "myblogx/internal/platform/mysql"
	"myblogx/internal/social/domain"

	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
)

// followRow 对应 user_follow_models 表。
type followRow struct {
	ID               ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	FollowedUserID   ctype.ID
	FollowedNickname string
	FollowedAvatar   string
	FollowedAbstract string
	FansUserID       ctype.ID
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
		row.ID = next
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

// ListRelations 实现 domain.Repository。
//
// 三个视图共用一条查询路径，差别只在"拿哪一端当对端"与"互关要不要再收一次"：
//
//	follow ：fans_user_id = 我        → 对端是 followed_user_id
//	fans   ：followed_user_id = 我    → 对端是 fans_user_id
//	mutual ：fans_user_id = 我 且 对端也关注我 → 对端是 followed_user_id
//
// 互关必须在这里筛（而不是取一页回来让调用方过滤）：先在 SQL 里筛再分页，
// 分页才是对的；反过来"先分页后过滤"会让每页条数随关系分布抖动，
// 前端只能靠多翻几页凑数。
func (r *Repo) ListRelations(ctx context.Context, ownerID uint64, kind domain.RelationKind, page domain.Page) ([]domain.Follow, bool, error) {
	page = page.Normalize()

	tx := r.db.WithContext(ctx).Model(&followRow{})
	var columns []string
	switch kind {
	case domain.KindFans:
		tx = tx.Where("followed_user_id = ?", ownerID)
		columns = fansColumns
	case domain.KindMutual:
		// 我关注的人里，那些也在关注我的人。
		tx = tx.Where("fans_user_id = ?", ownerID).
			Where("followed_user_id IN (?)",
				r.db.Model(&followRow{}).Select("fans_user_id").Where("followed_user_id = ?", ownerID))
		columns = followedColumns
	default:
		tx = tx.Where("fans_user_id = ?", ownerID)
		columns = followedColumns
	}

	var rows []followRow
	if err := tx.Select(columns).
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

// 只取本视图需要的那一端快照：follow/mutual 读被关注者，fans 读粉丝。
var (
	followedColumns = []string{"created_at", "followed_user_id", "followed_nickname", "followed_avatar", "followed_abstract"}
	fansColumns     = []string{"created_at", "fans_user_id", "fans_nickname", "fans_avatar", "fans_abstract"}
)

// CountRelations 实现 domain.Repository。
//
// 三个数是三条 COUNT：关注 / 粉丝各自一条，"互关"是"我关注的人里也关注我的"，
// 用一条带子查询的 COUNT。都是单表命中索引的计数，不值得为省两次往返
// 拼一条难读的条件聚合。
func (r *Repo) CountRelations(ctx context.Context, ownerID uint64) (domain.RelationCounts, error) {
	var counts domain.RelationCounts

	follow, err := r.count(ctx, ownerID, "fans_user_id = ?")
	if err != nil {
		return counts, err
	}
	fans, err := r.count(ctx, ownerID, "followed_user_id = ?")
	if err != nil {
		return counts, err
	}

	var mutual int64
	if err := r.db.WithContext(ctx).Model(&followRow{}).
		Where("fans_user_id = ?", ownerID).
		Where("followed_user_id IN (?)",
			r.db.Model(&followRow{}).Select("fans_user_id").Where("followed_user_id = ?", ownerID)).
		Count(&mutual).Error; err != nil {
		return counts, err
	}

	counts.Follow = int(follow)
	counts.Fans = int(fans)
	counts.Mutual = int(mutual)
	return counts, nil
}

// count 是带条件的活关系计数（软删由 GORM 自动加 deleted_at IS NULL）。
func (r *Repo) count(ctx context.Context, ownerID uint64, where string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&followRow{}).Where(where, ownerID).Count(&n).Error
	return n, err
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
		case row.FansUserID.Uint64() == viewerID:
			state[row.FollowedUserID.Uint64()] |= iFollow
		case row.FollowedUserID.Uint64() == viewerID:
			state[row.FansUserID.Uint64()] |= heFollow
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
