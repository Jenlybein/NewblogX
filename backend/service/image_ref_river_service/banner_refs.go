package image_ref_river_service

import (
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum/image_ref_enum"

	"gorm.io/gorm"
)

func RebuildBannerRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, banner *models.BannerModel) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if cover := strings.TrimSpace(banner.Cover); cover != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldBannerCover,
			Position: 0,
			URL:      cover,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeBanner, banner.ID, candidates)
}

func RebuildBannerRefsByRow(tx *gorm.DB, qiNiuConfig conf.QiNiu, snapshot rowSnapshot) error {
	bannerID, err := snapshot.ID()
	if err != nil {
		return err
	}
	if snapshot.IsDeleted() {
		return DeleteOwnerRefs(tx, image_ref_enum.RefTypeBanner, bannerID)
	}
	cover, err := snapshot.RequireString("cover")
	if err != nil {
		return err
	}
	return RebuildBannerRefs(tx, qiNiuConfig, &models.BannerModel{
		Model: models.Model{ID: bannerID},
		Cover: cover,
	})
}
