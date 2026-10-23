package read_service

import (
	"myblogx/models/ctype"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type UserDisplay = read_repo.UserDisplay
type ArticleBase = read_repo.ArticleBase

func LoadUserDisplayMap(db *gorm.DB, userIDs []ctype.ID) (map[ctype.ID]UserDisplay, error) {
	return read_repo.LoadUserDisplayMap(db, userIDs)
}

func SyncUserDisplaySnapshots(db *gorm.DB, userID ctype.ID) error {
	return read_repo.SyncUserDisplaySnapshots(db, userID)
}

func SyncArticleFavorSnapshots(db *gorm.DB, articleIDs []ctype.ID) error {
	return read_repo.SyncArticleFavorSnapshots(db, articleIDs)
}
