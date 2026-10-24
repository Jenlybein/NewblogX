package es_service

import (
	"encoding/json"
	"myblogx/models"
)

func ExtractArticles(data map[string]any) (articles []models.ArticleModel) {
	hits := data["hits"].([]any)

	for _, hit := range hits {
		item := hit.(map[string]any)
		sourceMap := item["_source"]

		// 利用 JSON 中转，将 map[string]any 快速转为结构体
		var article models.ArticleModel
		jsonBytes, _ := json.Marshal(sourceMap)
		_ = json.Unmarshal(jsonBytes, &article)

		articles = append(articles, article)
	}
	return
}
