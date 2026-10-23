package image_ref_river_service

import (
	"strings"

	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum/image_ref_enum"

	"gorm.io/gorm"
)

func RebuildArticleRefs(tx *gorm.DB, qiNiuConfig conf.QiNiu, article *models.ArticleModel) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return replaceOwnerRefs(tx, qiNiuConfig, image_ref_enum.RefTypeArticle, article.ID, parseArticleRefCandidates(article))
}

func RebuildArticleRefsByRow(tx *gorm.DB, qiNiuConfig conf.QiNiu, snapshot rowSnapshot) error {
	articleID, err := snapshot.ID()
	if err != nil {
		return err
	}
	if snapshot.IsDeleted() {
		return DeleteOwnerRefs(tx, image_ref_enum.RefTypeArticle, articleID)
	}
	content, err := snapshot.RequireString("content")
	if err != nil {
		return err
	}
	cover, err := snapshot.RequireString("cover")
	if err != nil {
		return err
	}
	return RebuildArticleRefs(tx, qiNiuConfig, &models.ArticleModel{
		Model:   models.Model{ID: articleID},
		Content: content,
		Cover:   cover,
	})
}

func parseArticleRefCandidates(article *models.ArticleModel) []refCandidate {
	candidates := make([]refCandidate, 0, 8)
	for index, imageURL := range ParseMarkdownImageURLs(article.Content) {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldArticleContent,
			Position: index,
			URL:      imageURL,
		})
	}
	if cover := strings.TrimSpace(article.Cover); cover != "" {
		candidates = append(candidates, refCandidate{
			Field:    image_ref_enum.RefFieldArticleCover,
			Position: 0,
			URL:      cover,
		})
	}
	return candidates
}
