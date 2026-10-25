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
	"myblogx/internal/ai/diagnose"
	"myblogx/internal/ai/domain"
	"myblogx/internal/ai/metainfo"
	"myblogx/internal/ai/overwrite"
	"myblogx/internal/ai/scoring"
	aisearch "myblogx/internal/ai/search"
	"myblogx/internal/apperr"
	searchdomain "myblogx/internal/search/domain"

	"github.com/sirupsen/logrus"
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
	scores   domain.ScoreRepository
	catalog  domain.Catalog
	tags     domain.TagReader
	searcher domain.Searcher
	ai       func() conf.AI
	rawLog   *logrus.Logger
	logger   Logger
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

// config 返回当前 AI 配置。
func (s *Service) config() (conf.AI, error) {
	if s.ai == nil {
		return conf.AI{}, apperr.New(apperr.CodeAIConfigMissing)
	}
	return s.ai(), nil
}

// ── 元信息生成 ──────────────────────────────────────────────────

// Metainfo 根据正文生成标题、摘要、分类与标签建议。
func (s *Service) Metainfo(ctx context.Context, userID uint64, content string) (*metainfo.MetainfoResponse, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	return metainfo.GenerateArticleMetainfo(ctx, s.catalog, s.rawLog, aiConf, userID, content)
}

// ── 文本改写与诊断 ──────────────────────────────────────────────

// RewriteStream 流式改写选中文本。
func (s *Service) RewriteStream(req overwrite.RewriteRequest) (chan string, chan error, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, nil, err
	}
	return overwrite.RewriteContentStream(aiConf, req)
}

// Diagnose 诊断选中文本。
func (s *Service) Diagnose(req diagnose.DiagnoseRequest) (*diagnose.DiagnoseResponse, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, err
	}
	return diagnose.DiagnoseSelectedText(aiConf, req)
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
	return aisearch.RewriteArticleSearch(tags, aiConf, content)
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
func (s *Service) StreamAnswer(question string, list []searchdomain.Item) (chan string, chan error, error) {
	aiConf, err := s.config()
	if err != nil {
		return nil, nil, err
	}
	return aisearch.AnalyzeArticleSearchStream(aiConf, question, list)
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
	RecordID       uint64
	ArticleID      uint64
	AITotalScore   int
	TotalScore     int
	ScoreLevel     string
	ArticleType    string
	Dimensions     []ScoreDimension
	MainIssues     []scoring.ArticleScoreIssue
	OverallComment string
	CreatedAt      time.Time
}

// ScoreSummary 读公开评分摘要（不含评语与原因）。
func (s *Service) ScoreSummary(ctx context.Context, articleID uint64) (ScoreView, error) {
	record, err := s.scores.Latest(ctx, articleID)
	if err != nil {
		return ScoreView{}, apperr.Wrap(apperr.CodeAIScoreMissing, err)
	}
	if record == nil {
		return ScoreView{ArticleID: articleID}, nil
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
		return ScoreView{ArticleID: articleID}, nil
	}
	return buildScoreView(record, true)
}

// RescoreInput 是重新评分的入参。
type RescoreInput struct {
	ArticleID uint64
	Title     string
	Content   string
}

// Rescore 重新评分并落库。
func (s *Service) Rescore(ctx context.Context, viewerID uint64, in RescoreInput) (ScoreView, error) {
	article, err := s.requireAuthor(ctx, viewerID, in.ArticleID)
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

	scoreResp, err := scoring.ScoreArticleQuality(aiConf, scoring.ArticleScoreRequest{Title: title, Content: content})
	if err != nil {
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
	scoreResp, err := scoring.ScoreArticleQuality(aiConf, scoring.ArticleScoreRequest{Title: title, Content: content})
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
	if article.AuthorID != viewerID {
		return domain.ArticleSnapshot{}, apperr.New(apperr.CodeAIPermissionDenied)
	}
	return article, nil
}

// persistScore 把评分结果转成记录并落库。
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
		ModelName:        strings.TrimSpace(aiConf.ChatModel),
		PromptVersion:    scoringPromptVersion,
	}
	if _, err := repo.CreateIfAbsent(ctx, record); err != nil {
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
		id := uint64(item.ID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		list = append(list, item)
	}
	return list
}
