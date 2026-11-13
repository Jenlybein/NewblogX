package app

import (
	"context"
	"fmt"

	"myblogx/conf"
	"myblogx/internal/ai"
	aihttp "myblogx/internal/ai/adapter/http"
	aimysql "myblogx/internal/ai/adapter/mysql"
	airedis "myblogx/internal/ai/adapter/redis"
	aiapp "myblogx/internal/ai/app"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/llm"
	"myblogx/internal/platform/log/writer"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/skills"
	searchredis "myblogx/internal/search/adapter/redis"
	searchapp "myblogx/internal/search/app"
	searchdomain "myblogx/internal/search/domain"

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

	// 每日额度的计数端口（ADR-0028）。装在这里而不是 ai 包内部：
	// internal/ai 不 import platform/redis，落点属于装配层的决定。
	ai.SetQuotaGate(airedis.NewDailyQuota(redisOptions))

	// 模型配置列表 + 内存快照。
	//
	// onChange 挂到 factory.Invalidate 上：配置一改就把 eino 的模型缓存清掉，
	// 否则旧的连接与旧的密钥还会被用一阵子。
	providers := aiapp.NewProviderService(aimysql.NewProvider(infra.DB), infra.Logger).
		WithOnChange(llm.SharedFactory().Invalidate)
	// 加载失败不阻断启动：AI 会返回"还没有启用的模型配置"，其它功能照常。
	// 最常见的原因是部署时忘了跑一次 -db（新表不会被 entrypoint 自动创建）。
	if err := providers.Load(context.Background()); err != nil {
		infra.Logger.Errorf("加载模型配置失败（AI 将不可用，检查是否已建 ai_provider_models 表）: %v", err)
	}

	svc := aiapp.New(
		aimysql.New(infra.DB),
		catalog,
		catalog, // 同一个适配器同时实现候选读取与标签标题
		&aiSearcher{
			counters: searchredis.NewCounters(redisOptions),
			esClient: infra.ESClient,
			index:    infra.Config.ES.Index,
			// AI 搜索复用 type 1，因此同样受「显示非管理员发布文章」开关约束。
			onlyAdminAuthors: onlyAdminAuthors(infra.Effective),
		},
		func() conf.AI {
			if infra.RuntimeSite == nil {
				return conf.AI{}
			}
			cfg := infra.RuntimeSite.GetRuntimeAI()
			// 每次现填而不是缓存整份 conf.AI：站点级设置与"用哪条模型"是
			// 两处数据源，任一处改了都要立刻生效。
			cfg.Active = providers.Active()
			// 额度在此归一化（越界值回落到默认值）：出站口每调用一次都要用
			// 它做判定，不该让一个手改的负数变成"全站 AI 立刻不可用"。
			cfg.DailyLimits = infra.Effective.AIDailyLimits()
			return cfg
		},
		infra.Logger,
	).WithRawLogger(infra.Logger).
		WithSceneSkills(sceneSkillCatalog{catalog: infra.Skills})

	aiService = svc
	router.SetAIHandler(aihttp.NewHandler(svc, providers))

	// 安装 AI 调用上报：每次调用的场景、模型、token 用量、耗时都落
	// ai_call_logs（ClickHouse + 每日文件）。
	//
	// 装在这里而不是 ai 包内部：internal/ai 只管"该记什么"，不 import
	// 日志写入层（platform/log/writer）—— 落点属于装配层的决定。
	installCallReporter(infra)
}

// installCallReporter 把 ai 包的上报出口接到日志管道上。
func installCallReporter(infra *Infra) {
	ai.SetCallReporter(func(ctx context.Context, res ai.CallResult) {
		errCode := ""
		errMessage := ""
		if res.Err != nil {
			if code, ok := apperr.CodeOf(res.Err); ok {
				errCode = string(code)
			}
			errMessage = res.Err.Error()
		}
		// 策略文本版本进 extra_json，而不是加列：ai_call_logs 的列是 ClickHouse
		// 里写死的，版本是"这次调用用了哪份口径"的附加事实，零 DDL 也能落。
		var extra map[string]string
		if res.Meta.PromptVersion != "" {
			extra = map[string]string{"prompt_version": res.Meta.PromptVersion}
		}
		writer.EmitAICallLog(infra.LogOptions, writer.AICallLogInput{
			Scene:            res.Meta.Scene,
			Vendor:           res.Usage.Vendor,
			Model:            res.Usage.Model,
			Stream:           res.Stream,
			Success:          res.Err == nil,
			Attempts:         res.Usage.Attempts,
			UsageAvailable:   res.Usage.Available,
			PromptTokens:     res.Usage.PromptTokens,
			CompletionTokens: res.Usage.CompletionTokens,
			TotalTokens:      res.Usage.Total(),
			CachedTokens:     res.Usage.CachedTokens,
			ReasoningTokens:  res.Usage.ReasoningTokens,
			LatencyMS:        res.Usage.Duration.Milliseconds(),
			UserID:           res.Meta.UserID,
			RequestID:        res.Meta.RequestID,
			TraceID:          res.Meta.TraceID,
			ErrorCode:        errCode,
			ErrorMessage:     errMessage,
			Extra:            extra,
		})
	})
}

// sceneSkillCatalog 把平台层 skill 目录适配成 AI 用例层的读取端口。
//
// 适配放在装配层：internal/ai 的用例只知道"要一段策略文本与版本"，
// 不知道它是从磁盘哪个目录读的；skills 加载器也不 import 任何业务包（ADR-0021）。
type sceneSkillCatalog struct {
	catalog *skills.Catalog
}

var _ domain.SceneSkillReader = sceneSkillCatalog{}

// ReadSceneSkill 读一份 skill 的正文与版本。
//
// 正文按需读盘（加载器只驻留目录层），所以运营替换 SKILL.md 后下一次调用
// 立刻用新文本；版本取加载时算出的字节哈希，随调用记录落进 extra_json。
func (r sceneSkillCatalog) ReadSceneSkill(name string) (domain.SceneSkill, error) {
	if r.catalog == nil {
		return domain.SceneSkill{}, apperr.New(apperr.CodeAIPolicySkillMissing)
	}
	skill, ok := r.catalog.Get(name)
	if !ok {
		return domain.SceneSkill{}, apperr.Wrap(
			apperr.CodeAIPolicySkillMissing, fmt.Errorf("skills 目录里没有名为 %q 的 skill", name))
	}
	body, version, err := skill.BodyWithVersion()
	if err != nil {
		return domain.SceneSkill{}, apperr.Wrap(apperr.CodeAIPolicySkillMissing, err)
	}
	// version 必须与 body 来自同一次读盘：正文可以热改，加载时的哈希会过期，
	// 用它记版本就会出现"新正文配旧版本号"（#309 用户故事 32 / ADR-0021）。
	return domain.SceneSkill{Name: skill.Name(), Body: body, Version: version}, nil
}

// aiSearcher 把 search 模块的用例适配成 AI 模块的搜索端口。
//
// AI 搜索是匿名搜索（不关心 viewer 与偏好标签），所以中间两个参数固定为 nil；
// 但「显示非管理员发布文章」开关依旧生效（AI 搜出来的是公开读面）。
type aiSearcher struct {
	counters searchdomain.CounterReader
	esClient *elasticsearch.Client
	index    string
	// onlyAdminAuthors 现读显示开关的反面（见 app.onlyAdminAuthors）。
	onlyAdminAuthors func() bool
}

var _ domain.Searcher = (*aiSearcher)(nil)

// Search 实现 domain.Searcher。
func (s *aiSearcher) Search(ctx context.Context, req searchdomain.Request) (searchdomain.Response, error) {
	_ = ctx
	if s.onlyAdminAuthors != nil {
		req.OnlyAdminAuthors = s.onlyAdminAuthors()
	}
	return searchapp.Search(req, nil, nil, s.counters, s.esClient, s.index)
}
