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
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面，由 adapter 每请求
// 从站点运行时配置现读后传入；开关关闭且作者不是管理员时同样返回"不存在"
// （ADR-0017 决策 4，规则本身在 domain.CanBeSeenByReader）。
func (s *Service) Detail(ctx context.Context, id domain.ArticleID, viewer domain.Viewer, onlyAdminAuthors bool) (*ArticleView, error) {
	var view *ArticleView

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, id)
		if err != nil {
			return err
		}
		if !domain.CanBeViewedBy(a, viewer) {
			return domain.ErrArticleNotFound
		}

		// 作者资料一次取全：既给可见性判定用（是否管理员），也给响应组装用。
		author, err := r.Users().Profile(ctx, a.AuthorID())
		if err != nil {
			return err
		}
		if !domain.CanBeSeenByReader(onlyAdminAuthors, author.Admin, viewer.UserID == a.AuthorID()) {
			return domain.ErrArticleNotFound
		}

		if err := enrichCounters(ctx, r, a); err != nil {
			return err
		}

		v, err := buildView(ctx, r, a, viewer, author)
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
//
// author 由调用方取好传进来：可见性判定（作者是否管理员）与响应组装
// 需要同一份资料，取两次就是一次多余的查询。
func buildView(ctx context.Context, r Repos, a *domain.Article, viewer domain.Viewer, author AuthorView) (*ArticleView, error) {
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
