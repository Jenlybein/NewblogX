package image_ref_river_service

import (
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum/image_ref_enum"

	"gorm.io/gorm"
)

func RebuildFavoriteRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, favorite *models.FavoriteModel) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if cover := strings.TrimSpace(favorite.Cover); cover != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldFavoriteCover,
			Position: 0,
			URL:      cover,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeFavorite, favorite.ID, candidates)
}

func RebuildFavoriteRefsByRow(tx *gorm.DB, qiNiuConfig conf.QiNiu, snapshot rowSnapshot) error {
	favoriteID, err := snapshot.ID()
	if err != nil {
		return err
	}
	if snapshot.IsDeleted() {
		return DeleteOwnerRefs(tx, image_ref_enum.RefTypeFavorite, favoriteID)
	}
	cover, err := snapshot.RequireString("cover")
	if err != nil {
		return err
	}
	return RebuildFavoriteRefs(tx, qiNiuConfig, &models.FavoriteModel{
		Model: models.Model{ID: favoriteID},
		Cover: cover,
	})
}
