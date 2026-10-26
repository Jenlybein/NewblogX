package mysql

import (
	"context"
	"encoding/json"
	"errors"

	"myblogx/internal/user/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingsRepo 实现 domain.SettingsRepository。
//
// 单独一个类型而不是并进 Repo：接口方法都叫 Find，Go 不允许同名方法共存。
type SettingsRepo struct {
	db *gorm.DB
}

var _ domain.SettingsRepository = (*SettingsRepo)(nil)

// Find 实现 domain.SettingsRepository。
func (r *SettingsRepo) Find(ctx context.Context, id domain.UserID) (domain.Settings, error) {
	var row UserConfRow
	err := r.db.WithContext(ctx).Take(&row, "user_id = ?", id.Uint64()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 老账号可能没有偏好行：返回默认值而不是报错，让页面能打开。
		return domain.DefaultSettings(id), nil
	}
	if err != nil {
		return domain.Settings{}, translate(err)
	}
	return toSettings(row), nil
}

// Save 实现 domain.SettingsRepository。
func (r *SettingsRepo) Save(ctx context.Context, s domain.Settings) error {
	if err := initSettingsDefaults(ctx, r.db, s.UserID); err != nil {
		return err
	}
	row := fromSettings(s)
	// map 形式的 Updates 不走 serializer，like_tags 必须自己序列化成 JSON 字符串，
	// 否则驱动会把 []uint64 当成多列值，MySQL 直接报 "Operand should contain 1 column(s)"。
	likeTags, err := json.Marshal(row.LikeTags)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&UserConfRow{}).
		Where("user_id = ?", s.UserID.Uint64()).
		Updates(map[string]any{
			"like_tags":                   string(likeTags),
			"updated_username_date":       row.UpdatedUsernameDate,
			"favorites_visibility":        row.FavoritesVisibility,
			"follow_visibility":           row.FollowVisibility,
			"fans_visibility":             row.FansVisibility,
			"home_style_id":               row.HomeStyleID,
			"digg_notice_enabled":         row.DiggNoticeEnabled,
			"comment_notice_enabled":      row.CommentNoticeEnabled,
			"favor_notice_enabled":        row.FavorNoticeEnabled,
			"private_chat_notice_enabled": row.PrivateChatNoticeEnabled,
			"stranger_chat_enabled":       row.StrangerChatEnabled,
		}).Error
}

// InitDefaults 实现 domain.SettingsRepository。
func (r *SettingsRepo) InitDefaults(ctx context.Context, id domain.UserID) error {
	return initSettingsDefaults(ctx, r.db, id)
}

// initSettingsDefaults 幂等建偏好行。
//
// 用 ON CONFLICT DO NOTHING 而不是「先查再插」：并发下先查再插会两边都以为不存在。
func initSettingsDefaults(ctx context.Context, db *gorm.DB, id domain.UserID) error {
	row := fromSettings(domain.DefaultSettings(id))
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return translate(err)
	}
	return nil
}

// StatsRepo 实现 domain.StatsRepository。
type StatsRepo struct {
	db *gorm.DB
}

var _ domain.StatsRepository = (*StatsRepo)(nil)

// Find 实现 domain.StatsRepository。
func (r *StatsRepo) Find(ctx context.Context, id domain.UserID) (domain.Stats, error) {
	var row UserStatRow
	err := r.db.WithContext(ctx).Take(&row, "user_id = ?", id.Uint64()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ZeroStats(id), nil
	}
	if err != nil {
		return domain.Stats{}, translate(err)
	}
	return domain.Stats{
		UserID:              row.UserID,
		ViewCount:           row.ViewCount,
		FansCount:           row.FansCount,
		FollowCount:         row.FollowCount,
		ArticleVisitedCount: row.ArticleVisitedCount,
		ArticleCount:        row.ArticleCount,
	}, nil
}

// Init 实现 domain.StatsRepository。
func (r *StatsRepo) Init(ctx context.Context, id domain.UserID) error {
	row := UserStatRow{UserID: id}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return translate(err)
	}
	return nil
}

// Add 实现 domain.StatsRepository。
func (r *StatsRepo) Add(ctx context.Context, id domain.UserID, delta domain.StatsDelta) error {
	if delta.IsZero() {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先确保行存在，否则 UPDATE 影响 0 行、增量静默丢失。
		row := UserStatRow{UserID: id}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return translate(err)
		}
		// 用 SQL 表达式累加，而不是先读后写：并发下的读改写会丢更新。
		if err := tx.Model(&UserStatRow{}).Where("user_id = ?", id.Uint64()).Updates(map[string]any{
			"view_count":            gorm.Expr("view_count + ?", delta.View),
			"fans_count":            gorm.Expr("fans_count + ?", delta.Fans),
			"follow_count":          gorm.Expr("follow_count + ?", delta.Follow),
			"article_visited_count": gorm.Expr("article_visited_count + ?", delta.ArticleVisited),
			"article_count":         gorm.Expr("article_count + ?", delta.Article),
		}).Error; err != nil {
			return translate(err)
		}
		return nil
	})
}
