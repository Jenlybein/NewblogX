package article_api

import (
	"myblogx/models"
	"myblogx/models/ctype"
)

type authorStatDelta struct {
	ArticleCount        int
	ArticleVisitedCount int
}

func collectArticleIDs(list []models.ArticleModel) []ctype.ID {
	ids := make([]ctype.ID, 0, len(list))
	for _, article := range list {
		if article.ID.IsZero() {
			continue
		}
		ids = append(ids, article.ID)
	}
	return ids
}

func buildAuthorStatDeltaMap(viewDeltaMap map[ctype.ID]int, list []models.ArticleModel) map[ctype.ID]authorStatDelta {
	result := make(map[ctype.ID]authorStatDelta)
	for _, article := range list {
		if article.AuthorID.IsZero() {
			continue
		}
		delta := result[article.AuthorID]
		delta.ArticleCount++
		delta.ArticleVisitedCount += article.ViewCount + viewDeltaMap[article.ID]
		result[article.AuthorID] = delta
	}
	return result
}
