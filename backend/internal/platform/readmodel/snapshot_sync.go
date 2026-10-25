package readmodel

import (
	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

func SyncUserDisplaySnapshots(db *gorm.DB, userID ctype.ID) error {
	if db == nil || userID == 0 {
		return nil
	}

	userMap, err := LoadUserDisplayMap(db, []ctype.ID{userID})
	if err != nil {
		return err
	}
	user, ok := userMap[userID]
	if !ok {
		return nil
	}

	if err = db.Table("comment_models").Where("user_id = ?", userID).Updates(map[string]any{
		"user_nickname": user.Nickname,
		"user_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Table("comment_models").Where("reply_user_id = ?", userID).Update("reply_user_nickname", user.Nickname).Error; err != nil {
		return err
	}
	if err = db.Table("user_follow_models").Where("followed_user_id = ?", userID).Updates(map[string]any{
		"followed_nickname": user.Nickname,
		"followed_avatar":   user.Avatar,
		"followed_abstract": user.Abstract,
	}).Error; err != nil {
		return err
	}
	if err = db.Table("user_follow_models").Where("fans_user_id = ?", userID).Updates(map[string]any{
		"fans_nickname": user.Nickname,
		"fans_avatar":   user.Avatar,
		"fans_abstract": user.Abstract,
	}).Error; err != nil {
		return err
	}
	if err = db.Table("favorite_models").Where("user_id = ?", userID).Updates(map[string]any{
		"owner_nickname": user.Nickname,
		"owner_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Table("user_article_favor_models").Where("article_author_id = ?", userID).Updates(map[string]any{
		"article_author_nickname": user.Nickname,
		"article_author_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Table("chat_session_models").Where("receiver_id = ?", userID).Updates(map[string]any{
		"receiver_nickname": user.Nickname,
		"receiver_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	return nil
}
