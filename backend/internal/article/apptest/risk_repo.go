package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/risk"
	"myblogx/internal/kernel/ctype"
)

// RiskRepo 是 risk.Repository 的内存实现（票 312 / ADR-0024）。
//
// 一篇文章一行、覆盖写：与真实仓储同样的语义，所以用例层的"覆盖 vs 累加"
// 这类差别能被测出来。
type RiskRepo struct {
	mu      sync.Mutex
	records map[ctype.ID]*risk.Record
}

// NewRiskRepo 构造空的风险记录 fake。
func NewRiskRepo() *RiskRepo {
	return &RiskRepo{records: make(map[ctype.ID]*risk.Record)}
}

// Upsert 写入或覆盖一篇文章的结论。
func (r *RiskRepo) Upsert(_ context.Context, record *risk.Record) error {
	if record == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *record
	r.records[record.ArticleID] = &copied
	return nil
}

// FindByArticle 读一篇文章的结论。
func (r *RiskRepo) FindByArticle(_ context.Context, articleID domain.ArticleID) (*risk.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[articleID]
	if !ok {
		return nil, nil
	}
	copied := *record
	return &copied, nil
}

// ListByArticles 批量读结论。
func (r *RiskRepo) ListByArticles(_ context.Context, articleIDs []domain.ArticleID) (map[ctype.ID]*risk.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[ctype.ID]*risk.Record, len(articleIDs))
	for _, id := range articleIDs {
		if record, ok := r.records[id]; ok {
			copied := *record
			out[id] = &copied
		}
	}
	return out, nil
}

// All 返回全部记录（按文章 ID 升序），测试断言用。
func (r *RiskRepo) All() []risk.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]risk.Record, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, *record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArticleID < out[j].ArticleID })
	return out
}
