package image_ref_river_service

import (
	"errors"
	"net/url"
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/image_ref_enum"

	"gorm.io/gorm"
)

type refCandidate struct {
	Field    image_ref_enum.RefField
	Position int
	URL      string
}

func DeleteOwnerRefs(tx *gorm.DB, refType image_ref_enum.RefType, ownerID ctype.ID) error {
	if tx == nil {
		return errors.New("tx is required")
	}
	return tx.Unscoped().Where("ref_type = ? AND owner_id = ?", refType, ownerID).Delete(&models.ImageRefModel{}).Error
}

func DeleteImageRefsByImageIDs(tx *gorm.DB, imageIDs []ctype.ID) error {
	if tx == nil {
		return errors.New("tx is required")
	}
	if len(imageIDs) == 0 {
		return nil
	}
	return tx.Unscoped().Where("image_id IN ?", imageIDs).Delete(&models.ImageRefModel{}).Error
}

func replaceOwnerRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, refType image_ref_enum.RefType, ownerID ctype.ID, candidates []refCandidate) error {
	if err := DeleteOwnerRefs(tx, refType, ownerID); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	objectKeys := uniqueObjectKeys(candidates, qiNiuConfig)
	if len(objectKeys) == 0 {
		return nil
	}

	var images []models.ImageModel
	if err := tx.Select("id", "object_key").Where("object_key IN ?", objectKeys).Find(&images).Error; err != nil {
		return err
	}

	imageIDMap := make(map[string]ctype.ID, len(images))
	for _, image := range images {
		imageIDMap[image.ObjectKey] = image.ID
	}

	refs := make([]models.ImageRefModel, 0, len(candidates))
	for _, candidate := range candidates {
		objectKey := extractObjectKey(candidate.URL, qiNiuConfig)
		imageID, ok := imageIDMap[objectKey]
		if !ok {
			continue
		}
		refs = append(refs, models.ImageRefModel{
			ImageID:  imageID,
			RefType:  refType,
			OwnerID:  ownerID,
			Field:    candidate.Field,
			Position: candidate.Position,
		})
	}
	if len(refs) == 0 {
		return nil
	}
	return tx.Create(&refs).Error
}

func uniqueObjectKeys(candidates []refCandidate, qiNiuConfig conf.QiNiu) []string {
	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		objectKey := extractObjectKey(candidate.URL, qiNiuConfig)
		if objectKey == "" {
			continue
		}
		if _, ok := seen[objectKey]; ok {
			continue
		}
		seen[objectKey] = struct{}{}
		result = append(result, objectKey)
	}
	return result
}

func extractObjectKey(raw string, qiNiuConfig conf.QiNiu) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	prefix := strings.Trim(qiNiuConfig.Prefix, "/")
	if prefix == "" {
		prefix = "images"
	}
	raw = strings.TrimPrefix(raw, "/")
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") && !strings.ContainsAny(raw, "?#") {
		if strings.HasPrefix(raw, prefix+"/images/") {
			return raw
		}
		return ""
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "?") || strings.Contains(raw, "#") {
		parsed, err := url.Parse(raw)
		if err == nil {
			raw = parsed.Path
		}
	}
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, prefix+"/images/") {
		return raw
	}
	return ""
}
