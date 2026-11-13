// Package app 是 AI 模块的用例层。
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"myblogx/conf"
	aipkg "myblogx/internal/ai"
	"myblogx/internal/ai/diagnose"
	"myblogx/internal/ai/domain"
	"myblogx/internal/ai/metainfo"
	"myblogx/internal/ai/moderation"
	"myblogx/internal/ai/overwrite"
	"myblogx/internal/ai/scoring"
	aisearch "myblogx/internal/ai/search"
	"myblogx/internal/apperr"
	searchdomain "myblogx/internal/search/domain"

	"github.com/sirupsen/logrus"
	"myblogx/internal/kernel/ctype"
)

// scoringPromptVersion 是评分提示词的版本号，落库用于回溯。
const scoringPromptVersion = "article-scoring-v1"

// 评分默认取多少条搜索结果作为 LLM 上下文。
const searchAnswerLimit = 10

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}

// Service 是 AI 服务。
type Service struct {
	scores      domain.ScoreRepository
	catalog     domain.Catalog
	tags        domain.TagReader
	searcher    domain.Searcher
	sceneSkills domain.SceneSkillReader
	ai          func() conf.AI
	rawLog      *logrus.Logger
	logger      Logger
}

// New 构造 AI 服务。
//
// ai 用函数而不是值：运行时配置随时可能被管理员改掉，必须每次现取。
func New(
	scores domain.ScoreRepository,
	catalog domain.Catalog,
	tags domain.TagReader,
	searcher domain.Searcher,
	ai func() conf.AI,
	logger Logger,
) *Service {
	return &Service{
		scores:   scores,
		catalog:  catalog,
		tags:     tags,
		searcher: searcher,
		ai:       ai,
		logger:   logger,
	}
}

// WithRawLogger 注入 logrus（元信息生成要用）。
func (s *Service) WithRawLogger(logger *logrus.Logger) *Service {
	s.rawLog = logger
	return s
}

// WithSceneSkills 注入运行时策略文本读取端口（场景 → skill 的正文与版本）。
//
// 由装配层接到 skills 目录加载器；单测里换成内存替身。不注入时，
// 配置里一旦绑定了 skill，相关场景会明确失败而不是悄悄用空口径。
func (s *Service) WithSceneSkills(reader domain.SceneSkillReader) *Service {
	s.sceneSkills = reader
	return s
}

// config 返回当前 AI 配置。
func (s *Service) config() (conf.AI, error) {
	if s.ai == nil {
		return conf.AI{}, apperr.New(apperr.CodeAIConfigMissing)
	}
	return s.ai(), nil
}

// Ready 报告 AI 现在能不能真的被调用：总闸开着、且至少有一条启用的模型配置。
//
// 口径与 specFromConfig 一致（它就是按这两条判失败的）。调用方是「开质量闸门」
// 这类"保存前先确认前提"的校验（票 306 / ADR-0023）：把失败面从运行期挪到
// 配置期，避免出现"闸门开着而 AI 关着"这种把所有投稿都拒掉的组合。
func (s *Service) Ready() bool {
	if s == nil {
		return false
	}
	aiConf, err := s.config()
	if err != nil {
		return false
	}
	return aiConf.Enable && aiConf.Active != nil
}

// ── 元信息生成 ──────────────────────────────────────────────────

// Metainfo 根据正文生成标题、摘要、分类与标签建议。
func (s *Service) Metainfo(ctx context.Context, userID uint64, content string) (*metainfo.MetainfoResponse, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	// 场景标签在这里打而不是在 handler：SearchLLM 一个请求里会连调
	// rewrite 与 answer 两个用例，按端点打标会把两者混成一个。
	ctx = aipkg.WithCallMeta(ctx, aipkg.CallMeta{Scene: aipkg.SceneMetainfo})
	return metainfo.GenerateArticleMetainfo(ctx, s.catalog, s.rawLog, aiConf, userID, content)
}

// ── 文本改写与诊断 ──────────────────────────────────────────────

// RewriteStream 流式改写选中文本。
func (s *Service) RewriteStream(ctx context.Context, req overwrite.RewriteRequest) (chan string, chan error, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, nil, err
	}
	ctx = aipkg.WithCallMeta(ctx, aipkg.CallMeta{Scene: aipkg.SceneOverwrite})
	return overwrite.RewriteContentStream(ctx, aiConf, req)
}

// Diagnose 诊断选中文本。
func (s *Service) Diagnose(ctx context.Context, req diagnose.DiagnoseRequest) (*diagnose.DiagnoseResponse, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	ctx = aipkg.WithCallMeta(ctx, aipkg.CallMeta{Scene: aipkg.SceneDiagnose})
	return diagnose.DiagnoseSelectedText(ctx, aiConf, req)
}

// ── 智能搜索 ────────────────────────────────────────────────────

// RewriteSearch 判断是否为搜索意图并改写搜索条件。
func (s *Service) RewriteSearch(ctx context.Context, content string) (*aisearch.ArticleSearchRewrite, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	tags, err := s.tags.TagTitles(ctx)
	if err != nil {
		return nil, err
	}
	ctx = aipkg.WithCallMeta(ctx, aipkg.CallMeta{Scene: aipkg.SceneSearchRewrite})
	return aisearch.RewriteArticleSearch(ctx, tags, aiConf, content)
}

// SearchArticles 用改写结果执行文章搜索。
//
// 顺序与旧实现一致：先带标签搜一遍（提高召回），再不标签搜一遍补齐，最后按 ID 去重。
func (s *Service) SearchArticles(ctx context.Context, rewrite *aisearch.ArticleSearchRewrite) (searchdomain.Response, error) {
	result := searchdomain.Response{
		List: []searchdomain.Item{},
		Pagination: searchdomain.Pagination{
			Mode:  searchdomain.PageModeHasMore,
			Page:  1,
			Limit: searchAnswerLimit,
		},
	}
	if rewrite == nil {
		return result, nil
	}
	key := buildSearchKey(rewrite.Query)
	if key == "" {
		return result, nil
	}
	if s.searcher == nil {
		return result, nil
	}

	list := make([]searchdomain.Item, 0, searchAnswerLimit)
	seen := make(map[uint64]struct{}, searchAnswerLimit)

	if len(rewrite.TagList) > 0 {
		page, err := s.searcher.Search(ctx, searchdomain.Request{
			PageInfo:      searchdomain.PageInfo{Page: 1, Limit: searchAnswerLimit},
			Type:          1,
			Sort:          rewrite.Sort,
			PageMode:      searchdomain.PageModeHasMore,
			LegacyTagList: rewrite.TagList,
			Key:           key,
		})
		if err != nil {
			return searchdomain.Response{}, err
		}
		list = appendUnique(list, seen, page.List)
	}

	page, err := s.searcher.Search(ctx, searchdomain.Request{
		PageInfo: searchdomain.PageInfo{Page: 1, Limit: searchAnswerLimit},
		Type:     1,
		Sort:     rewrite.Sort,
		PageMode: searchdomain.PageModeHasMore,
		Key:      key,
	})
	if err != nil {
		return searchdomain.Response{}, err
	}
	list = appendUnique(list, seen, page.List)

	if len(list) > searchAnswerLimit {
		list = list[:searchAnswerLimit]
	}
	result.List = list
	return result, nil
}

// StreamAnswer 基于检索结果让 LLM 生成回答（流式）。
//
// 「捏捏」的口吻、称呼与禁用语来自运行期配置 ai.scene_skills 绑定的运行时策略
// skill（#310）：这里解析场景绑定、读出正文与版本，正文拼进 system prompt，
// 版本随这次调用记录落库（回溯源文本）。代码里不再写第二份口径文案。
func (s *Service) StreamAnswer(ctx context.Context, question string, list []searchdomain.Item) (chan string, chan error, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, nil, err
	}
	policy, version, err := s.scenePolicy(aiConf, aipkg.SceneSearchAnswer)
	if err != nil {
		return nil, nil, err
	}
	ctx = aipkg.WithCallMeta(ctx, aipkg.CallMeta{
		Scene:         aipkg.SceneSearchAnswer,
		PromptVersion: version,
	})
	return aisearch.AnalyzeArticleSearchStream(ctx, aiConf, question, list, policy)
}

// scenePolicy 按运行期配置把场景绑定解析成最终要注入的正文与版本。
//
// 一个场景可以绑定多份 skill：正文按配置顺序拼接，版本写各份哈希的拼接。
// 没绑定任何 skill 时返回空串 —— 该场景就是它原来的样子，不注入策略文本。
func (s *Service) scenePolicy(aiConf conf.AI, scene string) (string, string, error) {
	names := aiConf.SceneSkills[scene]
	if len(names) == 0 {
		return "", "", nil
	}
	if s.sceneSkills == nil {
		return "", "", apperr.New(apperr.CodeAIPolicySkillMissing)
	}

	bodies := make([]string, 0, len(names))
	versions := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		skill, err := s.sceneSkills.ReadSceneSkill(name)
		if err != nil {
			return "", "", apperr.Wrap(apperr.CodeAIPolicySkillMissing, err)
		}
		if body := strings.TrimSpace(skill.Body); body != "" {
			bodies = append(bodies, body)
		}
		if version := strings.TrimSpace(skill.Version); version != "" {
			versions = append(versions, version)
		}
	}
	return strings.Join(bodies, "\n\n"), strings.Join(versions, "+"), nil
}

// ── 文章质量评分 ────────────────────────────────────────────────

// ScoreDimension 是评分维度。
type ScoreDimension struct {
	Name   string
	Score  int
	Reason string
}

// ScoreView 是评分的展示视图。
type ScoreView struct {
	HasScore       bool
	RecordID       ctype.ID
	ArticleID      ctype.ID
	AITotalScore   int
	TotalScore     int
	ScoreLevel     string
	ArticleType    string
	Dimensions     []ScoreDimension
	MainIssues     []scoring.ArticleScoreIssue
	OverallComment string
	CreatedAt      time.Time
}

// GateScoreInput 是一次「发布质量闸门」的评分请求（票 306 / ADR-0023）。
//
// 它与 RescoreInput 的区别是**不要求文章已经落库**：闸门在提交事务之外跑，
// 那时新建的文章还不存在（ID 已经生成）。因此这里自带标题、正文与作者，
// 由本方法直接决定评分记录的内容，不再回读文章行。
type GateScoreInput struct {
	ArticleID ctype.ID
	AuthorID  ctype.ID
	Title     string
	Content   string
	// UpdatedAt 是这次提交对应内容的更新时间，随评分记录落库用于回溯。
	UpdatedAt time.Time
	// AuthorIsAdmin 表示提交者拥有管理员身份：为真时不占每日评分额度（ADR-0028）。
	AuthorIsAdmin bool
}

// GateScoreResult 是闸门拿到的一次可用评分结论。
type GateScoreResult struct {
	TotalScore int
	// Dimensions 是六个维度的得分（ADR-0023）。闸门要做**木桶判定**——任何一项
	// 低于及格线都不能发布——所以维度分必须一起返回，不能只给总分。
	Dimensions []ScoreDimension
	Issues     []scoring.ArticleScoreIssue
}

// ComplianceResult 是一次「投稿合规判定」的结果（票 312 / ADR-0024）。
type ComplianceResult struct {
	Verdict    string
	RiskLevel  int
	Categories []string
	Reasons    []string
	ModelName  string
	// PromptVersion 是这次判定所用策略文本的版本（content hash 前缀）；
	// 空串表示站点没有给该场景绑定 skill。
	PromptVersion string
}

// TextCheckInput 是"对一段自由文本做合规判定"的输入。
//
// 与文章那条路的区别只有一处：没有文章实体可读，文本由调用方直接给。
// 评论就是这种形态（它属于某篇文章，但判定看的是评论内容本身）。
type TextCheckInput struct {
	// Title 是上下文标题（评论传所属文章标题），帮助模型理解语境。
	Title string
	// Content 是要判定的文本。
	Content string
	// MaxTokens 是本次判定的输出上限；<=0 用供应商配置。
	MaxTokens int
}

// CheckTextCompliance 对任意文本做一次合规判定（票 359：评论审核复用文章那套）。
//
// 与 CheckArticleCompliance 共用同一套策略文本、同一个模型档位、同一份结论结构：
// "能不能发"的判据在站内只能有一份，否则评论与文章会得出互相矛盾的结论。
func (s *Service) CheckTextCompliance(ctx context.Context, in TextCheckInput) (*ComplianceResult, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	if !aiConf.Enable {
		return nil, apperr.New(apperr.CodeAIServiceOff)
	}

	policy, version, err := s.scenePolicy(aiConf, aipkg.SceneModeration)
	if err != nil {
		return nil, err
	}
	checkCtx := aipkg.WithCallMeta(ctx, aipkg.CallMeta{
		Scene:         aipkg.SceneModeration,
		PromptVersion: version,
	})

	result, err := moderation.CheckCompliance(checkCtx, aiConf, moderation.CheckRequest{
		Title:     in.Title,
		Content:   in.Content,
		Policy:    policy,
		MaxTokens: in.MaxTokens,
	})
	if err != nil {
		return nil, err
	}

	return &ComplianceResult{
		Verdict:       string(result.Verdict),
		RiskLevel:     result.RiskLevel,
		Categories:    result.Categories,
		Reasons:       result.Reasons,
		ModelName:     result.ModelName,
		PromptVersion: version,
	}, nil
}

// CheckArticleCompliance 对一篇文章做合规判定（票 312 / ADR-0024）。
//
// 它读文章快照、注入 moderation 场景的策略文本、调模型，并把结论原样返回；
// **落库由调用方负责**（文章模块拥有风险记录表，AI 模块不跨模块写别人的表）。
//
// 失败语义：返回 error 表示"没判定出结论"（模型不可用、非 JSON 等）。它与
// "判定为干净"是两件事：前者要在后台显示"未判定"，绝不能当成 clean 放行。
func (s *Service) CheckArticleCompliance(ctx context.Context, articleID uint64) (*ComplianceResult, error) {
	if articleID == 0 {
		return nil, apperr.New(apperr.CodeAIArticleMissing)
	}
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	if !aiConf.Enable {
		return nil, apperr.New(apperr.CodeAIServiceOff)
	}

	article, err := s.scores.Article(ctx, articleID)
	if err != nil {
		return nil, err
	}

	policy, version, err := s.scenePolicy(aiConf, aipkg.SceneModeration)
	if err != nil {
		return nil, err
	}
	checkCtx := aipkg.WithCallMeta(ctx, aipkg.CallMeta{
		Scene:         aipkg.SceneModeration,
		PromptVersion: version,
	})

	result, err := moderation.CheckCompliance(checkCtx, aiConf, moderation.CheckRequest{
		Title:   article.Title,
		Content: article.Content,
		Policy:  policy,
	})
	if err != nil {
		return nil, err
	}

	return &ComplianceResult{
		Verdict:       string(result.Verdict),
		RiskLevel:     result.RiskLevel,
		Categories:    result.Categories,
		Reasons:       result.Reasons,
		ModelName:     result.ModelName,
		PromptVersion: version,
	}, nil
}

// ScoreForGate 为一次提交评分并落库，返回可用的分数与问题列表。
//
// 为什么落库：作者被拒时看到的分数、以及审核列表里的风险标签，都从这一份
// 记录读；不落库的话，同一次提交会被异步补评分再评一次，等于一次提交烧两次
// 模型调用（ADR-0023「代价」一节）。
//
// 拿不到可用分数一律返回 error（失败关闭），由用例层翻成
// article.score_unavailable；拿到分数就返回结果，哪怕只有 23 分。
func (s *Service) ScoreForGate(ctx context.Context, in GateScoreInput) (GateScoreResult, error) {
	aiConf, err := s.config()
	if err != nil {
		return GateScoreResult{}, err
	}
	if !aiConf.Enable {
		return GateScoreResult{}, apperr.New(apperr.CodeAIServiceOff)
	}

	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if title == "" || content == "" {
		return GateScoreResult{}, apperr.New(apperr.CodeAIArticleMissing)
	}

	// 评分按「动作」计额度（ADR-0028）：一次提交内部会按段落分块、顺序调 2..N 次，
	// 在这里预扣一次并给 ctx 打上预扣标记，分块子调用看到标记就不再重复扣。
	lease, err := aipkg.BuyActionQuota(ctx, aiConf, aipkg.SceneScoring, in.AuthorID, in.AuthorIsAdmin)
	if err != nil {
		return GateScoreResult{}, err
	}

	// 评分口径来自运行时策略文本（skills/scoring/SKILL.md，ADR-0023）：绑定被摘掉时
	// policy 为空，只靠内置判据与形态封顶，不报错——评分是发布链路上的必经步骤，
	// 让它因为"少绑一份文案"整站发不出文章，代价远大于收益。
	policy, policyVersion, err := s.scenePolicy(aiConf, aipkg.SceneScoring)
	if err != nil {
		releaseQuotaLease(ctx, lease)
		return GateScoreResult{}, err
	}

	scoreResp, err := scoring.ScoreArticleQuality(
		aipkg.WithCallMeta(ctx, aipkg.CallMeta{
			Scene:         aipkg.SceneScoring,
			UserID:        in.AuthorID,
			IsAdmin:       in.AuthorIsAdmin,
			Billing:       aipkg.BillingPrecharged,
			PromptVersion: policyVersion,
		}), aiConf,
		scoring.ArticleScoreRequest{Title: title, Content: content, Policy: policy})
	if err != nil {
		// 没拿到结论的系统性失败不占额度（与投稿配额同一条口径）。
		releaseQuotaLease(ctx, lease)
		return GateScoreResult{}, err
	}

	updatedAt := in.UpdatedAt
	snapshot := domain.ArticleSnapshot{
		ID:        in.ArticleID,
		AuthorID:  in.AuthorID,
		Title:     title,
		Content:   content,
		UpdatedAt: updatedAt,
	}
	if _, err := persistScore(ctx, s.scores, snapshot, title, content, scoreResp, aiConf); err != nil {
		return GateScoreResult{}, apperr.Wrap(apperr.CodeAIScoreSaveFailed, err)
	}

	return GateScoreResult{
		TotalScore: scoreResp.TotalScore,
		Dimensions: toScoreDimensions(scoreResp.Dimensions),
		Issues:     scoreResp.MainIssues,
	}, nil
}

// toScoreDimensions 把评分包的维度映射成用例层的展示形状。
func toScoreDimensions(list []scoring.ArticleScoreDimension) []ScoreDimension {
	result := make([]ScoreDimension, 0, len(list))
	for _, item := range list {
		result = append(result, ScoreDimension{Name: item.Name, Score: item.Score, Reason: item.Reason})
	}
	return result
}

// ScoreSummary 读公开评分摘要（不含评语与原因）。
func (s *Service) ScoreSummary(ctx context.Context, articleID uint64) (ScoreView, error) {
	record, err := s.scores.Latest(ctx, articleID)
	if err != nil {
		return ScoreView{}, apperr.Wrap(apperr.CodeAIScoreMissing, err)
	}
	if record == nil {
		return ScoreView{ArticleID: ctype.NewID(articleID)}, nil
	}
	return buildScoreView(record, false)
}

// ScoreDetail 读作者可见的完整评分。
func (s *Service) ScoreDetail(ctx context.Context, viewerID, articleID uint64) (ScoreView, error) {
	if _, err := s.requireAuthor(ctx, viewerID, articleID); err != nil {
		return ScoreView{}, err
	}
	record, err := s.scores.Latest(ctx, articleID)
	if err != nil {
		return ScoreView{}, apperr.Wrap(apperr.CodeAIScoreMissing, err)
	}
	if record == nil {
		return ScoreView{ArticleID: ctype.NewID(articleID)}, nil
	}
	return buildScoreView(record, true)
}

// RescoreInput 是重新评分的入参。
type RescoreInput struct {
	ArticleID ctype.ID
	Title     string
	Content   string
}

// Rescore 重新评分并落库。
func (s *Service) Rescore(ctx context.Context, viewerID uint64, in RescoreInput) (ScoreView, error) {
	article, err := s.requireAuthor(ctx, viewerID, in.ArticleID.Uint64())
	if err != nil {
		return ScoreView{}, err
	}
	aiConf, err := s.config()
	if err != nil {
		return ScoreView{}, err
	}

	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if title == "" {
		title = strings.TrimSpace(article.Title)
	}
	if content == "" {
		content = strings.TrimSpace(article.Content)
	}

	// 与闸门评分同档：一次重评动作只计 1 次额度。身份来自 handler 已写入的
	// CallMeta（重评走的是登录 + 作者校验过的接口）。
	meta := aipkg.CallMetaFrom(ctx)
	lease, err := aipkg.BuyActionQuota(ctx, aiConf, aipkg.SceneScoring, ctype.NewID(viewerID), meta.IsAdmin)
	if err != nil {
		return ScoreView{}, err
	}

	policy, policyVersion, err := s.scenePolicy(aiConf, aipkg.SceneScoring)
	if err != nil {
		releaseQuotaLease(ctx, lease)
		return ScoreView{}, err
	}

	scoreResp, err := scoring.ScoreArticleQuality(
		aipkg.WithCallMeta(ctx, aipkg.CallMeta{
			Scene:         aipkg.SceneScoring,
			IsAdmin:       meta.IsAdmin,
			Billing:       aipkg.BillingPrecharged,
			PromptVersion: policyVersion,
		}), aiConf,
		scoring.ArticleScoreRequest{Title: title, Content: content, Policy: policy})
	if err != nil {
		releaseQuotaLease(ctx, lease)
		return ScoreView{}, err
	}
	record, err := persistScore(ctx, s.scores, article, title, content, scoreResp, aiConf)
	if err != nil {
		return ScoreView{}, apperr.Wrap(apperr.CodeAIScoreSaveFailed, err)
	}
	return buildScoreView(record, true)
}

// EnsureScoreIfMissing 在文章已发布但没有评分时补一份（outbox 中继调用）。
func (s *Service) EnsureScoreIfMissing(ctx context.Context, articleID uint64) error {
	if articleID == 0 {
		return nil
	}
	aiConf, err := s.config()
	if err != nil {
		// 配置没装配不该让中继重试到死信。
		return nil
	}
	if !aiConf.Enable {
		return nil
	}
	article, err := s.scores.Article(ctx, articleID)
	if err != nil {
		if apperr.IsCode(err, apperr.CodeAIArticleMissing) {
			return nil
		}
		return err
	}
	if !article.Published {
		return nil
	}
	existing, err := s.scores.Latest(ctx, articleID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	title := strings.TrimSpace(article.Title)
	content := strings.TrimSpace(article.Content)
	// 这条 ctx 来自 outbox 中继而不是 HTTP 请求。作者归属仍然补上（日志要能回答
	// "这篇补评分是谁的文章"），但计费档位是"只记日志不计费"：它不是作者主动
	// 触发的动作，而且与闸门评分存在竞态，重复计量没有意义（ADR-0028）。
	policy, policyVersion, err := s.scenePolicy(aiConf, aipkg.SceneScoring)
	if err != nil {
		return err
	}
	scoreCtx := aipkg.WithCallMeta(ctx, aipkg.CallMeta{
		Scene:         aipkg.SceneScoring,
		UserID:        article.AuthorID,
		Billing:       aipkg.BillingUnmetered,
		PromptVersion: policyVersion,
	})
	scoreResp, err := scoring.ScoreArticleQuality(scoreCtx, aiConf, scoring.ArticleScoreRequest{
		Title: title, Content: content, Policy: policy,
	})
	if err != nil {
		return err
	}
	_, err = persistScore(ctx, s.scores, article, title, content, scoreResp, aiConf)
	return err
}

// requireAuthor 校验当前用户是文章作者。
func (s *Service) requireAuthor(ctx context.Context, viewerID, articleID uint64) (domain.ArticleSnapshot, error) {
	if viewerID == 0 {
		return domain.ArticleSnapshot{}, apperr.New(apperr.CodeAuthRequired)
	}
	article, err := s.scores.Article(ctx, articleID)
	if err != nil {
		return domain.ArticleSnapshot{}, err
	}
	if article.AuthorID.Uint64() != viewerID {
		return domain.ArticleSnapshot{}, apperr.New(apperr.CodeAIPermissionDenied)
	}
	return article, nil
}

// persistScore 把评分结果转成记录并落库。
//
// 一律走 Upsert：同一篇文章再次评分覆盖既有那行，作者改稿后看到的就是新结论。
// 覆盖写由仓储在事务里保证"只有一行"（与并发竞争一起收口），组合层不需要先查后写。
func persistScore(
	ctx context.Context,
	repo domain.ScoreRepository,
	article domain.ArticleSnapshot,
	title string,
	content string,
	resp *scoring.ArticleScoreResponse,
	aiConf conf.AI,
) (*domain.ScoreRecord, error) {
	if resp == nil {
		return nil, apperr.New(apperr.CodeAIScoreSaveFailed)
	}
	dimensionsJSON, err := json.Marshal(resp.Dimensions)
	if err != nil {
		return nil, err
	}
	mainIssuesJSON, err := json.Marshal(resp.MainIssues)
	if err != nil {
		return nil, err
	}
	hashBytes := sha256.Sum256([]byte(content))
	updatedAt := article.UpdatedAt
	record := &domain.ScoreRecord{
		ArticleID:        article.ID,
		UserID:           article.AuthorID,
		TitleSnapshot:    title,
		ContentHash:      hex.EncodeToString(hashBytes[:]),
		ContentLength:    len([]rune(content)),
		ArticleUpdatedAt: &updatedAt,
		AITotalScore:     resp.AITotalScore,
		TotalScore:       resp.TotalScore,
		ScoreLevel:       resp.ScoreLevel,
		ArticleType:      resp.ArticleType,
		DimensionsJSON:   string(dimensionsJSON),
		MainIssuesJSON:   string(mainIssuesJSON),
		OverallComment:   resp.OverallComment,
		Provider:         "llm",
		ModelName:        activeModelName(aiConf),
		PromptVersion:    scoringPromptVersion,
	}
	if err := repo.Upsert(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// buildScoreView 把记录转成视图。
//
// includeAdvice=false 时按旧契约隐去总分、文章类型、原因与评语——
// 公开页面只展示等级和维度分数。
func buildScoreView(record *domain.ScoreRecord, includeAdvice bool) (ScoreView, error) {
	if record == nil {
		return ScoreView{}, nil
	}
	var rawDimensions []scoring.ArticleScoreDimension
	if strings.TrimSpace(record.DimensionsJSON) != "" {
		if err := json.Unmarshal([]byte(record.DimensionsJSON), &rawDimensions); err != nil {
			return ScoreView{}, err
		}
	}
	view := ScoreView{
		HasScore:     true,
		RecordID:     record.ID,
		ArticleID:    record.ArticleID,
		AITotalScore: record.AITotalScore,
		TotalScore:   record.TotalScore,
		ScoreLevel:   record.ScoreLevel,
		ArticleType:  record.ArticleType,
		CreatedAt:    record.CreatedAt,
		Dimensions:   make([]ScoreDimension, 0, len(rawDimensions)),
	}
	for _, item := range rawDimensions {
		dim := ScoreDimension{Name: item.Name, Score: item.Score}
		if includeAdvice {
			dim.Reason = item.Reason
		}
		view.Dimensions = append(view.Dimensions, dim)
	}
	if !includeAdvice {
		view.AITotalScore = 0
		view.ArticleType = ""
		return view, nil
	}
	if strings.TrimSpace(record.MainIssuesJSON) != "" {
		if err := json.Unmarshal([]byte(record.MainIssuesJSON), &view.MainIssues); err != nil {
			return ScoreView{}, err
		}
	}
	view.OverallComment = record.OverallComment
	return view, nil
}

// buildSearchKey 把改写出的关键词拼成一个查询串。
func buildSearchKey(queryList []string) string {
	parts := make([]string, 0, len(queryList))
	for _, item := range queryList {
		item = strings.Join(strings.Fields(strings.TrimSpace(item)), " ")
		if item == "" {
			continue
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, " ")
}

// appendUnique 按 ID 去重追加。
func appendUnique(list []searchdomain.Item, seen map[uint64]struct{}, extra []searchdomain.Item) []searchdomain.Item {
	for _, item := range extra {
		id := item.ID.Uint64()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		list = append(list, item)
	}
	return list
}

// activeModelName 取当前启用配置的模型名，用于评分记录回溯。
//
// 不能直接读 conf.AI 的某个字段：模型名现在挂在 ai_provider_models 表里，
// 由装配层投影到 AI.Active。
func activeModelName(aiConf conf.AI) string {
	if aiConf.Active == nil {
		return ""
	}
	return strings.TrimSpace(aiConf.Active.Model)
}
