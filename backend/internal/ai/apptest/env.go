// Package apptest 提供 AI 模块的内存替身。
package apptest

import (
	"context"
	"sync"

	"myblogx/conf"
	"myblogx/internal/ai/app"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	searchdomain "myblogx/internal/search/domain"
)

// Scores 是评分仓储的内存实现。
type Scores struct {
	mu      sync.Mutex
	records []domain.ScoreRecord
	next    uint64
	// Articles 是预置的文章快照。
	Articles map[uint64]domain.ArticleSnapshot
}

var _ domain.ScoreRepository = (*Scores)(nil)

// NewScores 构造空仓库。
func NewScores() *Scores {
	return &Scores{next: 1, Articles: make(map[uint64]domain.ArticleSnapshot)}
}

// Article 实现 domain.ScoreRepository。
func (s *Scores) Article(_ context.Context, id uint64) (domain.ArticleSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	article, ok := s.Articles[id]
	if !ok {
		return domain.ArticleSnapshot{}, apperr.New(apperr.CodeAIArticleMissing)
	}
	return article, nil
}

// Latest 实现 domain.ScoreRepository。
func (s *Scores) Latest(_ context.Context, articleID uint64) (*domain.ScoreRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].ArticleID.Uint64() == articleID {
			record := s.records[i]
			return &record, nil
		}
	}
	return nil, nil
}

// CreateIfAbsent 实现 domain.ScoreRepository。
func (s *Scores) CreateIfAbsent(_ context.Context, record *domain.ScoreRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.records {
		if row.ArticleID == record.ArticleID {
			return false, nil
		}
	}
	if record.ID.IsZero() {
		record.ID = ctype.NewID(s.next)
		s.next++
	}
	s.records = append(s.records, *record)
	return true, nil
}

// Seed 直接插入一条记录（测试播种用）。
func (s *Scores) Seed(record domain.ScoreRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.ID.IsZero() {
		record.ID = ctype.NewID(s.next)
		s.next++
	}
	s.records = append(s.records, record)
}

// All 返回全部记录（测试断言用）。
func (s *Scores) All() []domain.ScoreRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.ScoreRecord(nil), s.records...)
}

// Catalog 是分类/标签候选的内存实现。
type Catalog struct {
	// ByUser 按用户预置分类。
	ByUser map[uint64][]domain.Option
	// TagList 是全部标签。
	TagList []domain.Option
}

var (
	_ domain.Catalog   = (*Catalog)(nil)
	_ domain.TagReader = (*Catalog)(nil)
)

// NewCatalog 构造空候选。
func NewCatalog() *Catalog {
	return &Catalog{ByUser: make(map[uint64][]domain.Option)}
}

// Categories 实现 domain.Catalog。
func (c *Catalog) Categories(_ context.Context, userID uint64) ([]domain.Option, error) {
	return c.ByUser[userID], nil
}

// Tags 实现 domain.Catalog。
func (c *Catalog) Tags(_ context.Context) ([]domain.Option, error) {
	return c.TagList, nil
}

// TagTitles 实现 domain.TagReader。
func (c *Catalog) TagTitles(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(c.TagList))
	for _, tag := range c.TagList {
		out = append(out, tag.Title)
	}
	return out, nil
}

// Searcher 是搜索端口的假实现，按标签数返回不同的结果。
type Searcher struct {
	mu      sync.Mutex
	Calls   []searchdomain.Request
	Results func(req searchdomain.Request) searchdomain.Response
}

var _ domain.Searcher = (*Searcher)(nil)

// Search 实现 domain.Searcher。
func (s *Searcher) Search(_ context.Context, req searchdomain.Request) (searchdomain.Response, error) {
	s.mu.Lock()
	s.Calls = append(s.Calls, req)
	s.mu.Unlock()
	if s.Results == nil {
		return searchdomain.Response{}, nil
	}
	return s.Results(req), nil
}

// Env 打包一组替身。
type Env struct {
	Scores   *Scores
	Catalog  *Catalog
	Searcher *Searcher
	AI       conf.AI
	Service  *app.Service
}

// NewEnv 用给定 AI 配置构造替身。
func NewEnv(aiConf conf.AI) *Env {
	scores := NewScores()
	catalog := NewCatalog()
	searcher := &Searcher{}
	return &Env{
		Scores:   scores,
		Catalog:  catalog,
		Searcher: searcher,
		AI:       aiConf,
		Service:  app.New(scores, catalog, catalog, searcher, func() conf.AI { return aiConf }, nil),
	}
}

// NewDefaultService 用空实现组装服务（路由表测试用）。
func NewDefaultService() *app.Service { return NewEnv(conf.AI{}).Service }
