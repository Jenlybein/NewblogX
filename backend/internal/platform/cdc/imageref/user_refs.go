package imageref

import (
	"strings"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"

	"gorm.io/gorm"
)

// RebuildUserRefs 重建一个用户的头像引用。
func RebuildUserRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, userID ctype.ID, avatar string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if trimmed := strings.TrimSpace(avatar); trimmed != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldUserAvatar,
			Position: 0,
			URL:      trimmed,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeUser, userID, candidates)
}

func RebuildUserRefsByRow(tx *gorm.DB, qiNiuConfig conf.QiNiu, snapshot rowSnapshot) error {
	userID, err := snapshot.ID()
	if err != nil {
		return err
	}
	if snapshot.IsDeleted() {
		return DeleteOwnerRefs(tx, image_ref_enum.RefTypeUser, userID)
	}
	avatar, err := snapshot.RequireString("avatar")
	if err != nil {
		return err
	}
	return RebuildUserRefs(tx, qiNiuConfig, userID, avatar)
}
