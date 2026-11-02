package apptest

import (
	"context"
	"sort"
	"sync"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/domain"
)

// SidebarCards 是右栏卡片配置表的内存实现。
//
// 与 MySQL 实现保持同一个排序口径：sort_order 升序、同序按 id 升序。
// 返回顺序不去重、不隐藏停用行 —— 过滤交给用例层，这样用例层的过滤
// 才有被单独钉住的机会。
type SidebarCards struct {
	mu    sync.Mutex
	rows  []domain.SidebarCard
	next  uint64
	calls int
}

var _ domain.SidebarCardRepository = (*SidebarCards)(nil)

// NewSidebarCards 构造空仓库。
func NewSidebarCards() *SidebarCards { return &SidebarCards{next: 1} }

// Seed 直接写入一批卡片（测试造配置用），保留调用方给的 id 与顺序。
func (s *SidebarCards) Seed(rows ...domain.SidebarCard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range rows {
		if row.ID.IsZero() {
			row.ID = ctype.NewID(s.next)
			s.next++
		}
		s.rows = append(s.rows, row)
	}
}

// ListEnabled 返回启用中的卡片，按 sort_order、id 升序。
func (s *SidebarCards) ListEnabled(_ context.Context) ([]domain.SidebarCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hit := make([]domain.SidebarCard, 0, len(s.rows))
	for _, row := range s.rows {
		if row.Enabled {
			hit = append(hit, row)
		}
	}
	sortSidebarCards(hit)
	return hit, nil
}

// ListAll 返回全部卡片（含停用），按 sort_order、id 升序。
func (s *SidebarCards) ListAll(_ context.Context) ([]domain.SidebarCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]domain.SidebarCard(nil), s.rows...)
	sortSidebarCards(out)
	return out, nil
}

// Replace 全量替换卡片配置，为没有 id 的行分配 id。
func (s *SidebarCards) Replace(_ context.Context, cards []domain.SidebarCard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	now := time.Now()
	rows := make([]domain.SidebarCard, 0, len(cards))
	for _, card := range cards {
		if card.ID.IsZero() {
			card.ID = ctype.NewID(s.next)
		}
		if card.CreatedAt.IsZero() {
			card.CreatedAt = now
		}
		card.UpdatedAt = now
		rows = append(rows, card)
		if card.ID.Uint64() >= s.next {
			s.next = card.ID.Uint64() + 1
		}
	}
	s.rows = rows
	return nil
}

// All 返回全部行（含停用），测试断言用。
func (s *SidebarCards) All() []domain.SidebarCard {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]domain.SidebarCard(nil), s.rows...)
	sortSidebarCards(out)
	return out
}

// ReplaceCalls 报告 Replace 被调用的次数。
func (s *SidebarCards) ReplaceCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func sortSidebarCards(rows []domain.SidebarCard) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].ID.Uint64() < rows[j].ID.Uint64()
	})
}

// SidebarData 是右栏各卡数据的读端口替身。
//
// 三个数据源各自独立：某一种卡取数失败不能拖垮别种卡。
type SidebarData struct {
	mu       sync.Mutex
	articles []domain.SidebarArticle
	comments []domain.SidebarComment
	tags     []domain.SidebarTag
	err      error
	limits   map[string]int
}

var _ domain.SidebarDataSource = (*SidebarData)(nil)

// NewSidebarData 构造空数据源。
func NewSidebarData() *SidebarData { return &SidebarData{limits: make(map[string]int)} }

// PutArticles 设置热门文章数据。
func (d *SidebarData) PutArticles(items ...domain.SidebarArticle) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.articles = items
}

// PutComments 设置最新评论数据。
func (d *SidebarData) PutComments(items ...domain.SidebarComment) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.comments = items
}

// PutTags 设置标签云数据。
func (d *SidebarData) PutTags(items ...domain.SidebarTag) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tags = items
}

// FailWith 让每一种取数都返回同一个错误。
func (d *SidebarData) FailWith(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

// HotArticles 实现 domain.SidebarDataSource。
func (d *SidebarData) HotArticles(_ context.Context, limit int) ([]domain.SidebarArticle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limits["hot_articles"] = limit
	if d.err != nil {
		return nil, d.err
	}
	return truncate(d.articles, limit), nil
}

// LatestComments 实现 domain.SidebarDataSource。
func (d *SidebarData) LatestComments(_ context.Context, limit int) ([]domain.SidebarComment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limits["latest_comments"] = limit
	if d.err != nil {
		return nil, d.err
	}
	return truncate(d.comments, limit), nil
}

// TagCloud 实现 domain.SidebarDataSource。
func (d *SidebarData) TagCloud(_ context.Context, limit int) ([]domain.SidebarTag, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limits["tag_cloud"] = limit
	if d.err != nil {
		return nil, d.err
	}
	return truncate(d.tags, limit), nil
}

// LastLimit 报告某一种卡最近一次取数用的 limit。
func (d *SidebarData) LastLimit(kind string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.limits[kind]
}

func truncate[T any](items []T, limit int) []T {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return append([]T(nil), items...)
}
