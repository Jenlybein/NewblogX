package app

import (
	"context"

	"myblogx/internal/article/domain"
)

// Detail 读取文章详情。
//
// 与旧 ArticleDetailView 等价：
//   - 不可见一律返回"文章不存在"（不区分"没有"与"无权"，避免探测草稿）；
//   - 计数 = 库内快照 + 缓存增量；
//   - 附带作者信息、分类名、标签、当前用户是否已赞/已收藏。
func (s *Service) Detail(ctx context.Context, id domain.ArticleID, viewer domain.Viewer) (*ArticleView, error) {
	var view *ArticleView

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, id)
		if err != nil {
			return err
		}
		if !domain.CanBeViewedBy(a, viewer) {
			return domain.ErrArticleNotFound
		}

		if err := enrichCounters(ctx, r, a); err != nil {
			return err
		}

		v, err := buildView(ctx, r, a, viewer)
		if err != nil {
			return err
		}
		view = v
		return nil
	})

	if err != nil {
		return nil, err
	}
	return view, nil
}

// List 分页查询文章。
//
// 匿名访问强制 OnlyPublic：这是防"草稿泄漏到前台"的最后一道闸，
// 不允许调用方传 false 绕过。
func (s *Service) List(ctx context.Context, filter domain.Filter, page domain.Page, viewer domain.Viewer) ([]*ArticleView, int64, error) {
	if viewer.IsAnonymous() {
		filter.OnlyPublic = true
	}
	// 非管理员看别人的列表时也必须是"匿名可见"的集合。
	if !viewer.IsAdmin() && filter.AuthorID != nil && *filter.AuthorID != viewer.UserID {
		filter.OnlyPublic = true
	}

	var views []*ArticleView
	var total int64

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		articles, count, err := r.Articles().List(ctx, filter, page)
		if err != nil {
			return err
		}
		total = count

		if len(articles) == 0 {
			return nil
		}

		// 批量取作者，避免 N+1。
		authorIDs := make([]domain.UserID, 0, len(articles))
		for _, a := range articles {
			authorIDs = append(authorIDs, a.AuthorID())
		}
		profiles, err := r.Users().Profiles(ctx, authorIDs)
		if err != nil {
			return err
		}

		// 批量取计数增量。
		ids := make([]domain.ArticleID, 0, len(articles))
		for _, a := range articles {
			ids = append(ids, a.ID())
		}
		deltas, err := r.Counters().Load(ctx, ids)
		if err != nil {
			return err
		}

		// 批量取标签标题。
		tagIDs := map[domain.TagID]struct{}{}
		for _, a := range articles {
			for _, t := range a.Tags() {
				tagIDs[t] = struct{}{}
			}
		}
		titles := map[domain.TagID]string{}
		if len(tagIDs) > 0 {
			list := make([]domain.TagID, 0, len(tagIDs))
			for id := range tagIDs {
				list = append(list, id)
			}
			tags, err := r.Tags().FindMany(ctx, list)
			if err != nil {
				return err
			}
			for _, t := range tags {
				titles[t.ID()] = t.Title()
			}
		}

		views = make([]*ArticleView, 0, len(articles))
		for _, a := range articles {
			if d, ok := deltas[a.ID()]; ok {
				a.AddCounters(d)
			}
			v := &ArticleView{
				Article: *a,
				Author:  profiles[a.AuthorID()],
				Tags:    tagViews(a.Tags(), titles),
			}
			if cm := a.CategoryID(); cm != nil {
				if c, err := r.Categories().Find(ctx, *cm); err == nil {
					v.CategoryName = c.Title()
				}
			}
			views = append(views, v)
		}
		return nil
	})

	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// enrichCounters 把缓存增量合并进聚合的计数。
func enrichCounters(ctx context.Context, r Repos, a *domain.Article) error {
	deltas, err := r.Counters().Load(ctx, []domain.ArticleID{a.ID()})
	if err != nil {
		return err
	}
	if d, ok := deltas[a.ID()]; ok {
		a.AddCounters(d)
	}
	return nil
}

// buildView 组装详情视图。
func buildView(ctx context.Context, r Repos, a *domain.Article, viewer domain.Viewer) (*ArticleView, error) {
	author, err := r.Users().Profile(ctx, a.AuthorID())
	if err != nil {
		return nil, err
	}

	v := &ArticleView{Article: *a, Author: author}

	if cm := a.CategoryID(); cm != nil {
		if c, err := r.Categories().Find(ctx, *cm); err == nil {
			v.CategoryName = c.Title()
		}
	}

	if ids := a.Tags(); len(ids) > 0 {
		tags, err := r.Tags().FindMany(ctx, ids)
		if err != nil {
			return nil, err
		}
		titles := make(map[domain.TagID]string, len(tags))
		for _, t := range tags {
			titles[t.ID()] = t.Title()
		}
		v.Tags = tagViews(ids, titles)
	}

	if viewer.IsLoggedIn() {
		digg, err := r.Diggs().Exists(ctx, a.ID(), viewer.UserID)
		if err != nil {
			return nil, err
		}
		v.Digg = digg
		if _, err := r.Favorites().FindItemAnyFolder(ctx, a.ID(), viewer.UserID); err == nil {
			v.Favor = true
		}
	}

	return v, nil
}

// tagViews 按传入顺序组装标签视图，保持与文章标签顺序一致。
func tagViews(ids []domain.TagID, titles map[domain.TagID]string) []TagView {
	out := make([]TagView, 0, len(ids))
	for _, id := range ids {
		out = append(out, TagView{ID: id, Title: titles[id]})
	}
	return out
}
