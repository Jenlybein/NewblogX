package search_service

import (
	"myblogx/models/ctype"
	"myblogx/repository/user_repo"

	"gorm.io/gorm"
)

func LoadUserLikeTagIDs(db *gorm.DB, userID ctype.ID) ([]ctype.ID, error) {
	return user_repo.LoadLikeTagIDs(db, userID)
}
