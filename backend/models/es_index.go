package models

import "strings"

const DefaultArticleESIndex = "article_index"

func ResolveArticleESIndex(index string) string {
	clean := strings.TrimSpace(index)
	if clean == "" {
		return DefaultArticleESIndex
	}
	return clean
}
