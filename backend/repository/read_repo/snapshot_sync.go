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

func SyncArticleFavorSnapshots(db *gorm.DB, articleIDs []ctype.ID) error {
	if db == nil {
		return nil
	}
	articleIDs = NormalizeIDs(articleIDs)
	if len(articleIDs) == 0 {
		return nil
	}

	articleMap, err := LoadArticleBaseMap(db, articleIDs)
	if err != nil {
		return err
	}
	if len(articleMap) == 0 {
		return nil
	}

	authorIDs := make([]ctype.ID, 0, len(articleMap))
	for _, article := range articleMap {
		authorIDs = append(authorIDs, article.AuthorID)
	}
	authorMap, err := LoadUserDisplayMap(db, authorIDs)
	if err != nil {
		return err
	}

	for _, articleID := range articleIDs {
		article, ok := articleMap[articleID]
		if !ok {
			continue
		}
		author := authorMap[article.AuthorID]
		if err = db.Model(&models.UserArticleFavorModel{}).
			Where("article_id = ?", articleID).
			Updates(map[string]any{
				"article_title":           article.Title,
				"article_abstract":        article.Abstract,
				"article_cover":           article.Cover,
				"article_status":          article.PublishStatus,
				"article_author_id":       article.AuthorID,
				"article_author_nickname": author.Nickname,
				"article_author_avatar":   author.Avatar,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}
