package image_service

import (
	"errors"
	"strings"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"

	"gorm.io/gorm"
)

var ErrImageUnavailable = errors.New("图片不存在或不可用")

func ResolveImageURLByID(db *gorm.DB, imageID ctype.ID) (string, error) {
	if imageID == 0 {
		return "", nil
	}
	var image models.ImageModel
	if err := db.Select("id", "url", "status").Take(&image, imageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrImageUnavailable
		}
		return "", err
	}
	switch image.Status {
	case enum.ImageStatusPass, enum.ImageStatusReviewing:
		return strings.TrimSpace(image.URL), nil
	default:
		return "", ErrImageUnavailable
	}
}

func FindImageIDByURL(db *gorm.DB, rawURL string) (*ctype.ID, error) {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return nil, nil
	}
	var image models.ImageModel
	if err := db.Select("id").Where("url = ?", url).Take(&image).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &image.ID, nil
}
