package es_service

import (
	"encoding/json"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
)

// ArticleDoc 是 article_index 的 `_source` 投影。
//
// 注意它不是数据库行：字段名对应 ES mapping（见 platform/es/mapping），
// 而不是 MySQL 的列。所以**不能**拿它去读写库。
type ArticleDoc struct {
	ID               ctype.ID                     `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	Title            string                       `json:"title"`
	Abstract         string                       `json:"abstract"`
	Content          string                       `json:"content"`
	Cover            string                       `json:"cover"`
	AuthorID         ctype.ID                     `json:"author_id"`
	CategoryID       *ctype.ID                    `json:"category_id"`
	ViewCount        int                          `json:"view_count"`
	DiggCount        int                          `json:"digg_count"`
	CommentCount     int                          `json:"comment_count"`
	FavorCount       int                          `json:"favor_count"`
	CommentsToggle   bool                         `json:"comments_toggle"`
	PublishStatus    enum.ArticleStatus           `json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `json:"visibility_status"`
}

// ExtractArticles 把 ES 命中结果解析成文档投影。
func ExtractArticles(data map[string]any) (articles []ArticleDoc) {
	hits := data["hits"].([]any)

	for _, hit := range hits {
		item := hit.(map[string]any)
		sourceMap := item["_source"]

		// 利用 JSON 中转，将 map[string]any 快速转为结构体
		var article ArticleDoc
		jsonBytes, _ := json.Marshal(sourceMap)
		_ = json.Unmarshal(jsonBytes, &article)

		articles = append(articles, article)
	}
	return
}
