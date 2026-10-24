package mysql

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"

	"gorm.io/gorm"
)

// SyncDisplay 实现 domain.SnapshotSyncer：把昵称/头像/简介刷新到 7 张表的冗余列。
//
// 为什么需要它：评论、关注、收藏、私信这些列表页为了不 join 用户表，
// 各自存了一份昵称头像（有意的反范式）。用户改了昵称，这些副本必须跟着刷新，
// 否则会出现"个人主页叫新名字、评论区还叫旧名字"。
//
// 为什么用 Table("xxx_models") 而不是定义这批表的 row 结构：
// 这些表属于 comment / social / chat 模块，本模块只写 2-3 个冗余列，
// 定义完整 row 反而制造"user 模块也拥有评论表"的错觉。
//
// 代价是这条链路仍然是跨模块写。它会在 S4.2（comment）与 S4.5（social/chat）
// 建好之后，改成发 domain 事件、由各模块自己刷新自己的表——那时本方法只剩
// 一个 outbox 发布动作。
func (r *Repo) SyncDisplay(ctx context.Context, id domain.UserID) error {
	if id.IsZero() {
		return nil
	}

	var row UserRow
	if err := r.db.WithContext(ctx).
		Select("id", "nickname", "avatar", "abstract").
		Take(&row, "id = ?", id.Uint64()).Error; err != nil {
		return translate(err)
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 每一条都独立判断，任何一条失败就整体回滚：
		// 只刷新了一半的快照，比完全没刷新更难排查。
		steps := []func(*gorm.DB) error{
			func(tx *gorm.DB) error {
				return tx.Table("comment_models").Where("user_id = ?", id.Uint64()).
					Updates(map[string]any{
						"user_nickname": row.Nickname,
						"user_avatar":   row.Avatar,
					}).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("comment_models").Where("reply_user_id = ?", id.Uint64()).
					Update("reply_user_nickname", row.Nickname).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("user_follow_models").Where("followed_user_id = ?", id.Uint64()).
					Updates(map[string]any{
						"followed_nickname": row.Nickname,
						"followed_avatar":   row.Avatar,
						"followed_abstract": row.Abstract,
					}).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("user_follow_models").Where("fans_user_id = ?", id.Uint64()).
					Updates(map[string]any{
						"fans_nickname": row.Nickname,
						"fans_avatar":   row.Avatar,
						"fans_abstract": row.Abstract,
					}).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("favorite_models").Where("user_id = ?", id.Uint64()).
					Updates(map[string]any{
						"owner_nickname": row.Nickname,
						"owner_avatar":   row.Avatar,
					}).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("user_article_favor_models").Where("article_author_id = ?", id.Uint64()).
					Updates(map[string]any{
						"article_author_nickname": row.Nickname,
						"article_author_avatar":   row.Avatar,
					}).Error
			},
			func(tx *gorm.DB) error {
				return tx.Table("chat_session_models").Where("receiver_id = ?", id.Uint64()).
					Updates(map[string]any{
						"receiver_nickname": row.Nickname,
						"receiver_avatar":   row.Avatar,
					}).Error
			},
		}
		for _, step := range steps {
			if err := step(tx); err != nil {
				return apperr.Wrap(apperr.CodeUserNotFound, err)
			}
		}
		return nil
	})
}

// 编译期断言：快照同步也由本类型提供。
var _ domain.SnapshotSyncer = (*Repo)(nil)
