package image_ref_river_service

import (
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum/image_ref_enum"

	"gorm.io/gorm"
)

func RebuildUserRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, user *models.UserModel) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	candidates := make([]refCandidate, 0, 1)
	if avatar := strings.TrimSpace(user.Avatar); avatar != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldUserAvatar,
			Position: 0,
			URL:      avatar,
		})
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeUser, user.ID, candidates)
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
	return RebuildUserRefs(tx, qiNiuConfig, &models.UserModel{
		Model:  models.Model{ID: userID},
		Avatar: avatar,
	})
}
