package ai_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/service/ai_service/ai_search"
	"myblogx/service/redis_service"
	"myblogx/service/search_service"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/gin-gonic/gin"
)

func (h AIApi) AIArticleSearchListView(c *gin.Context) {
	app := h.App
	if app.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	aiConf := app.RuntimeSite.GetRuntimeAI()
	cr := middleware.GetBindJson[AIBaseRequest](c)

	rewrite, err := ai_search.RewriteArticleSearch(app.DB, aiConf, cr.Content)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if rewrite.Intent != "search" {
		res.FailWithMsg("当前输入不是文章搜索意图", c)
		return
	}

	// fmt.Println(rewrite)

	key := buildAIArticleSearchKey(rewrite.Query)
	if key == "" {
		res.FailWithMsg("搜索关键词不能为空", c)
		return
	}

	result, err := searchAIArticles(rewrite, redis_service.NewDeps(app.Redis, app.Logger), app.ESClient, app.ES.Index)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	res.OkWithData(result, c)
}

func (h AIApi) AIArticleSearchLLMView(c *gin.Context) {
	app := h.App
	if app.RuntimeSite == nil {
		res.SSEFail("运行时配置服务未初始化", c)
		return
	}
	aiConf := app.RuntimeSite.GetRuntimeAI()
	cr := middleware.GetBindJson[AIBaseRequest](c)
	prepareAISSE(c)

	// 意图识别与搜索重写
	rewrite, err := ai_search.RewriteArticleSearch(app.DB, aiConf, cr.Content)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}

	// 非搜索意图，直接回复
	if rewrite.Intent != "search" {
		res.SSEOk(AIBaseResponse{
			Content: rewrite.Content,
		}, c)
		return
	}

	// 搜索意图，进行搜索
	key := buildAIArticleSearchKey(rewrite.Query)
	if key == "" {
		res.SSEFail("搜索关键词不能为空", c)
		return
	}

	result, err := searchAIArticles(rewrite, redis_service.NewDeps(app.Redis, app.Logger), app.ESClient, app.ES.Index)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}

	contentChan, errChan, err := ai_search.AnalyzeArticleSearchStream(aiConf, cr.Content, result.List)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}

	for contentChan != nil || errChan != nil {
		select {
		// 接收消息
		case text, ok := <-contentChan:
			if !ok {
				contentChan = nil
				continue
			}
			res.SSEOk(AIBaseResponse{
				Content: text,
			}, c)
		// 接收错误
		case streamErr, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if streamErr != nil {
				res.SSEFail(streamErr.Error(), c)
				return
			}
		}
	}
}

func buildAIArticleSearchKey(queryList []string) string {
	result := ""
	for _, item := range queryList {
		item = strings.Join(strings.Fields(strings.TrimSpace(item)), " ")
		if item == "" {
			continue
		}
		if result == "" {
			result = item
			continue
		}
		result += " " + item
	}
	return result
}

func appendUniqueSearchResults(
	list []search_service.SearchListResponse,
	seen map[ctype.ID]struct{},
	appendList []search_service.SearchListResponse,
) []search_service.SearchListResponse {
	for _, item := range appendList {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		list = append(list, item)
	}
	return list
}

func searchAIArticles(rewrite *ai_search.ArticleSearchRewrite, redisDeps redis_service.Deps, esClient *elasticsearch.Client, index string) (search_service.ArticleSearchResponse, error) {
	key := buildAIArticleSearchKey(rewrite.Query)
	limit := common.PageInfo{Page: 1, Limit: 10}.GetLimit()
	result := search_service.ArticleSearchResponse{
		List: []search_service.SearchListResponse{},
		Pagination: search_service.SearchPagination{
			Mode:    search_service.PageModeHasMore,
			Page:    1,
			Limit:   limit,
			HasMore: false,
		},
	}
	if key == "" {
		return result, nil
	}

	list := make([]search_service.SearchListResponse, 0, limit+1)
	seen := make(map[ctype.ID]struct{}, limit+1)

	if len(rewrite.TagList) > 0 {
		tagResult, err := search_service.SearchArticles(search_service.ArticleSearchRequest{
			Type:          1,
			Sort:          rewrite.Sort,
			PageMode:      search_service.PageModeHasMore,
			LegacyTagList: rewrite.TagList,
			Key:           key,
			PageInfo:      common.PageInfo{Page: 1, Limit: limit},
		}, nil, nil, redisDeps, esClient, index)
		if err != nil {
			return search_service.ArticleSearchResponse{}, err
		}
		list = appendUniqueSearchResults(list, seen, tagResult.List)
	}

	queryResult, err := search_service.SearchArticles(search_service.ArticleSearchRequest{
		Type:     1,
		Sort:     rewrite.Sort,
		PageMode: search_service.PageModeHasMore,
		Key:      key,
		PageInfo: common.PageInfo{Page: 1, Limit: limit},
	}, nil, nil, redisDeps, esClient, index)
	if err != nil {
		return search_service.ArticleSearchResponse{}, err
	}
	list = appendUniqueSearchResults(list, seen, queryResult.List)

	if len(list) > limit {
		list = list[:limit]
	}
	result.Pagination.HasMore = false
	result.List = list
	return result, nil
}
