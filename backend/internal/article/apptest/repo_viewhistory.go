package apptest

import (
	"context"
	"sort"
	"sync"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/viewhistory"
)

// 浏览历史的仓储 fake 单独成文件：它的 List 契约（游标 + limit）比其它社交仓储
// 更复杂，留在 repo_social.go 会把那个文件顶过 archcheck R6 的 800 行上限。
// ViewHistoryRepo 是 viewhistory.Repository 的内存实现。
type ViewHistoryRepo struct {
	mu    sync.Mutex
	byKey map[itemKey2]*viewhistory.Record
	fault fault
	count counter
	// lastQuery 记录 List 收到的查询（见该方法的注释）。
	lastQuery viewhistory.ListQuery
}

// NewViewHistoryRepo 创建空仓储。
func NewViewHistoryRepo() *ViewHistoryRepo {
	return &ViewHistoryRepo{byKey: make(map[itemKey2]*viewhistory.Record)}
}

// FailNext 注入一次性故障。
func (r *ViewHistoryRepo) FailNext(method string, err error) { r.fault.FailNext(method, err) }

// Calls 返回调用次数。
func (r *ViewHistoryRepo) Calls(method string) int { return r.count.Calls(method) }

// Upsert 写入或刷新浏览记录。
func (r *ViewHistoryRepo) Upsert(_ context.Context, rec *viewhistory.Record) error {
	r.count.inc("Upsert")
	if err := r.fault.take("Upsert"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := itemKey2{rec.UserID(), rec.ArticleID()}
	r.byKey[key] = viewhistory.NewRecord(rec.UserID(), rec.ArticleID(), rec.ViewedAt())
	return nil
}

// List 列出一页浏览历史（游标分页）。
//
// fake 没有文章表，拿不到标题，所以 keyword 只记录不生效（真实过滤由 MySQL
// 集成测试守）；作者过滤同理。游标与 limit 则必须真的生效：has_more 的口径
// （"多取一条"）与分页顺序是本层用例要观测的行为。
func (r *ViewHistoryRepo) List(_ context.Context, q viewhistory.ListQuery) ([]*viewhistory.Record, error) {
	r.count.inc("List")
	if err := r.fault.take("List"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastQuery = q

	var all []*viewhistory.Record
	for k, v := range r.byKey {
		if k.userID == q.UserID {
			all = append(all, v)
		}
	}
	// 与 MySQL 实现同一口径：(浏览时间, 文章 ID) 倒序。
	sort.Slice(all, func(i, j int) bool {
		if !all[i].ViewedAt().Equal(all[j].ViewedAt()) {
			return all[i].ViewedAt().After(all[j].ViewedAt())
		}
		return all[i].ArticleID().Uint64() > all[j].ArticleID().Uint64()
	})

	if q.Before != nil {
		kept := all[:0]
		for _, v := range all {
			// 严格早于游标位置：时间更早，或同一时刻且文章 ID 更小（与 ORDER BY 一致）。
			older := v.ViewedAt().Before(q.Before.ViewedAt) ||
				(v.ViewedAt().Equal(q.Before.ViewedAt) && v.ArticleID().Uint64() < q.Before.ArticleID.Uint64())
			if older {
				kept = append(kept, v)
			}
		}
		all = kept
	}

	if q.Limit > 0 && len(all) > q.Limit {
		all = all[:q.Limit]
	}
	return all, nil
}

// LastListQuery 返回上一次 List 收到的查询。
func (r *ViewHistoryRepo) LastListQuery() viewhistory.ListQuery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastQuery
}

// ListOnlyAdminAuthors 返回上一次 List 收到的过滤开关。
func (r *ViewHistoryRepo) ListOnlyAdminAuthors() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastQuery.OnlyAdminAuthors
}

// Delete 删除指定记录。
func (r *ViewHistoryRepo) Delete(_ context.Context, userID domain.UserID, articleIDs []domain.ArticleID) error {
	r.count.inc("Delete")
	if err := r.fault.take("Delete"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, aid := range articleIDs {
		delete(r.byKey, itemKey2{userID, aid})
	}
	return nil
}

// DeleteAll 清空某用户的全部浏览历史。
func (r *ViewHistoryRepo) DeleteAll(_ context.Context, userID domain.UserID) error {
	r.count.inc("DeleteAll")
	if err := r.fault.take("DeleteAll"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.byKey {
		if k.userID == userID {
			delete(r.byKey, k)
		}
	}
	return nil
}

// DeleteByArticle 删除某文章的全部记录。
func (r *ViewHistoryRepo) DeleteByArticle(_ context.Context, articleID domain.ArticleID) error {
	r.count.inc("DeleteByArticle")
	if err := r.fault.take("DeleteByArticle"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.byKey {
		if k.articleID == articleID {
			delete(r.byKey, k)
		}
	}
	return nil
}

// Snapshot 参与事务回滚。
func (r *ViewHistoryRepo) Snapshot() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[itemKey2]*viewhistory.Record, len(r.byKey))
	for k, v := range r.byKey {
		cp[k] = v
	}
	return cp
}

// RestoreState 覆盖回快照。
func (r *ViewHistoryRepo) RestoreState(s any) {
	if cp, ok := s.(map[itemKey2]*viewhistory.Record); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.byKey = cp
	}
}
