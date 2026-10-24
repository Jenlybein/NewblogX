package read_repo

import (
	"myblogx/models"
	"myblogx/models/ctype"

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

	if err = db.Model(&models.CommentModel{}).Where("user_id = ?", userID).Updates(map[string]any{
		"user_nickname": user.Nickname,
		"user_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Model(&models.CommentModel{}).Where("reply_user_id = ?", userID).Update("reply_user_nickname", user.Nickname).Error; err != nil {
		return err
	}
	if err = db.Model(&models.UserFollowModel{}).Where("followed_user_id = ?", userID).Updates(map[string]any{
		"followed_nickname": user.Nickname,
		"followed_avatar":   user.Avatar,
		"followed_abstract": user.Abstract,
	}).Error; err != nil {
		return err
	}
	if err = db.Model(&models.UserFollowModel{}).Where("fans_user_id = ?", userID).Updates(map[string]any{
		"fans_nickname": user.Nickname,
		"fans_avatar":   user.Avatar,
		"fans_abstract": user.Abstract,
	}).Error; err != nil {
		return err
	}
	if err = db.Model(&models.FavoriteModel{}).Where("user_id = ?", userID).Updates(map[string]any{
		"owner_nickname": user.Nickname,
		"owner_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Model(&models.UserArticleFavorModel{}).Where("article_author_id = ?", userID).Updates(map[string]any{
		"article_author_nickname": user.Nickname,
		"article_author_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	if err = db.Model(&models.ChatSessionModel{}).Where("receiver_id = ?", userID).Updates(map[string]any{
		"receiver_nickname": user.Nickname,
		"receiver_avatar":   user.Avatar,
	}).Error; err != nil {
		return err
	}
	return nil
}
