// Package search_service 是搜索模块的兼容层。
//
// 真正的 ES 查询构造已迁到 internal/search（S4.4）。这里只保留类型别名与
// 一个 SearchArticles 包装，供尚未迁移的 AI 模块（S4.8）继续调用；
// AI 迁完后本包整体删除。
package search_service

import (
	"myblogx/internal/platform/redis"
	searchredis "myblogx/internal/search/adapter/redis"
	searchapp "myblogx/internal/search/app"
	"myblogx/internal/search/domain"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"

	"github.com/elastic/go-elasticsearch/v7"
)

type PageMode = domain.PageMode

const (
	PageModeHasMore = domain.PageModeHasMore
	PageModeCount   = domain.PageModeCount
)

type PageInfo = domain.PageInfo
type ArticleSearchRequest = domain.Request
type SearchTag = domain.Tag
type SearchCategory = domain.Category
type SearchAuthor = domain.Author
type SearchTop = domain.Top
type SearchHighlight = domain.Highlight
type SearchListResponse = domain.Item
type SearchPagination = domain.Pagination
type ArticleSearchResponse = domain.Response

// SearchArticles 兼容旧签名：不传 claims 表示匿名。
func SearchArticles(cr ArticleSearchRequest, claims *jwts.MyClaims, likeTagIDs []ctype.ID, redisDeps redis_service.Deps, esClient *elasticsearch.Client, index string) (ArticleSearchResponse, error) {
	var viewer *domain.Viewer
	if claims != nil {
		viewer = &domain.Viewer{ID: claims.UserID, Admin: claims.IsAdmin()}
	}
	return searchapp.Search(cr, viewer, likeTagIDs, searchredis.NewCounters(redisDeps), esClient, index)
}
