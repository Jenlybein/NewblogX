package imageref

import (
	"strings"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"

	"gorm.io/gorm"
)

// RebuildFavoriteRefs 重建一个收藏夹的图片引用。
func RebuildFavoriteRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, favoriteID ctype.ID, cover string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if trimmed := strings.TrimSpace(cover); trimmed != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldFavoriteCover,
			Position: 0,
			URL:      trimmed,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeFavorite, favoriteID, candidates)
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
	return RebuildFavoriteRefs(tx, qiNiuConfig, favoriteID, cover)
}
