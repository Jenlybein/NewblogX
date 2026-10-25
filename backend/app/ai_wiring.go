package app

import (
	"context"

	"myblogx/conf"
	aihttp "myblogx/internal/ai/adapter/http"
	aimysql "myblogx/internal/ai/adapter/mysql"
	aiapp "myblogx/internal/ai/app"
	"myblogx/internal/ai/domain"
	redis_service "myblogx/internal/platform/redis"
	searchredis "myblogx/internal/search/adapter/redis"
	searchapp "myblogx/internal/search/app"
	searchdomain "myblogx/internal/search/domain"
	"myblogx/router"

	"github.com/elastic/go-elasticsearch/v7"
)

// aiService 供 outbox 中继补评分。
var aiService *aiapp.Service

// WireAI 装配 AI 模块，并注入路由处理器。
func WireAI(infra *Infra) {
	if infra == nil {
		return
	}
	redisOptions := redis_service.NewOptions(infra.Redis, infra.Logger)
	catalog := aimysql.NewCatalog(infra.DB)
	svc := aiapp.New(
		aimysql.New(infra.DB),
		catalog,
		catalog, // 同一个适配器同时实现候选读取与标签标题
		&aiSearcher{
			counters: searchredis.NewCounters(redisOptions),
			esClient: infra.ESClient,
			index:    infra.Config.ES.Index,
		},
		func() conf.AI {
			if infra.RuntimeSite == nil {
				return conf.AI{}
			}
			return infra.RuntimeSite.GetRuntimeAI()
		},
		infra.Logger,
	).WithRawLogger(infra.Logger)

	aiService = svc
	router.SetAIHandler(aihttp.NewHandler(svc))
}

// aiSearcher 把 search 模块的用例适配成 AI 模块的搜索端口。
//
// AI 搜索是匿名搜索（不关心 viewer 与偏好标签），所以中间两个参数固定为 nil。
type aiSearcher struct {
	counters searchdomain.CounterReader
	esClient *elasticsearch.Client
	index    string
}

var _ domain.Searcher = (*aiSearcher)(nil)

// Search 实现 domain.Searcher。
func (s *aiSearcher) Search(ctx context.Context, req searchdomain.Request) (searchdomain.Response, error) {
	_ = ctx
	return searchapp.Search(req, nil, nil, s.counters, s.esClient, s.index)
}
