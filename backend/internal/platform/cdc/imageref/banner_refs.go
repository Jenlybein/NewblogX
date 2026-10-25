package imageref

import (
	"strings"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"

	"gorm.io/gorm"
)

// RebuildBannerRefs 重建一个轮播位的图片引用。
func RebuildBannerRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, bannerID ctype.ID, cover string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if trimmed := strings.TrimSpace(cover); trimmed != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldBannerCover,
			Position: 0,
			URL:      trimmed,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeBanner, bannerID, candidates)
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
	return RebuildBannerRefs(tx, qiNiuConfig, bannerID, cover)
}
