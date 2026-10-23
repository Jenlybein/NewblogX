package user_service

import (
	"myblogx/models"
	"myblogx/models/ctype"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InitUserDefaults(tx *gorm.DB, userID ctype.ID) error {
	if tx == nil || userID == 0 {
		return nil
	}

	confModel := models.UserConfModel{
		UserID:                   userID,
		FavoritesVisibility:      true,
		FollowVisibility:         true,
		FansVisibility:           true,
		HomeStyleID:              1,
		DiggNoticeEnabled:        true,
		CommentNoticeEnabled:     true,
		FavorNoticeEnabled:       true,
		PrivateChatNoticeEnabled: true,
		StrangerChatEnabled:      true,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&confModel).Error; err != nil {
		return err
	}

	statModel := models.UserStatModel{
		UserID:              userID,
		ViewCount:           0,
		FansCount:           0,
		FollowCount:         0,
		ArticleVisitedCount: 0,
		ArticleCount:        0,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&statModel).Error
}
