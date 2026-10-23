package ai_search

import (
	"myblogx/models/ctype"
	"myblogx/utils/markdown"
	"time"
)

// ArticleSearchRewrite 是搜索意图识别后的结构化结果。
type ArticleSearchRewrite struct {
	Intent  string   `json:"intent"`
	Content string   `json:"content"`
	Query   []string `json:"query"`
	TagList []string `json:"tag_list"`
	Sort    int8     `json:"sort"`
}

// AISearchList 是喂给大模型做结果总结的精简文章结构。
type AISearchList struct {
	ID           ctype.ID               `json:"id"`
	CreatedAt    time.Time              `json:"created_at"`
	Title        string                 `json:"title"`
	Abstract     string                 `json:"abstract,omitempty"`
	Content      string                 `json:"content,omitempty"`
	Part         []markdown.ContentPart `json:"part,omitempty"`
	ViewCount    int                    `json:"view_count"`
	DiggCount    int                    `json:"digg_count"`
	CommentCount int                    `json:"comment_count"`
	FavorCount   int                    `json:"favor_count"`
	Tags         []string               `json:"tags"`
}
