package app

import (
	"context"
	"fmt"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/top"
	"myblogx/internal/article/domain/viewhistory"
)

// Favorite 收藏文章。
//
// 与旧实现一致：不能收藏自己的文章；不指定收藏夹时落到默认收藏夹
// （没有则自动创建）。收藏数走缓存增量，不写文章表。
//
// 返回值是操作后的权威状态与收藏总数（ADR-0024 决策 4）：前端据此直接 patch，
// 不必重新取整篇正文。
func (s *Service) Favorite(ctx context.Context, cmd FavoriteCmd) (FavoriteResult, error) {
	var out FavoriteResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeFavorited(a, cmd.Actor) {
			if cmd.Actor.IsLoggedIn() && cmd.Actor.UserID == a.AuthorID() {
				return favorite.ErrSelfFavorite
			}
			return domain.ErrArticleNotFound
		}

		folderID := cmd.FolderID
		if folderID.IsZero() {
			f, err := r.Favorites().DefaultFolder(ctx, cmd.Actor.UserID)
			if err != nil {
				return err
			}
			folderID = f.ID()
		} else {
			f, err := r.Favorites().FindFolder(ctx, folderID)
			if err != nil {
				return err
			}
			if !f.BelongsTo(cmd.Actor.UserID) {
				return favorite.ErrFolderForbidden
			}
		}

		if _, err := r.Favorites().FindItem(ctx, a.ID(), cmd.Actor.UserID, folderID); err == nil {
			return favorite.ErrItemExists
		}

		item := favorite.NewItemFromArticle(folderID, cmd.Actor.UserID, a)
		if u, err := r.Users().Profile(ctx, a.AuthorID()); err == nil {
			item.WithAuthorProfile(u.Nickname, u.Avatar)
		}
		if err := r.Favorites().SaveItem(ctx, item); err != nil {
			return err
		}
		if err := r.Counters().Add(ctx, a.ID(), domain.Counters{Favor: 1}); err != nil {
			return err
		}

		// 计数在"库内快照 + 当前缓存增量"上重算：前端拿到的数字必须是权威值。
		if err := enrichCounters(ctx, r, a); err != nil {
			return err
		}
		out = FavoriteResult{IsFavor: true, FavorCount: a.Counters().Favor}

		// 通知作者"有人收藏了你的文章"（旧实现同样只在收藏成功时发）。
		return emitWithActor(ctx, r, EventArticleFavorited, a.ID(), cmd.Actor.UserID)
	})

	return out, err
}

// Unfavorite 取消收藏。
//
// 返回值同上：移除后仍可能被收在别的收藏夹里，所以 `IsFavor` 要按"还有没有关系行"
// 重新判定，而不是无条件写 false。
func (s *Service) Unfavorite(ctx context.Context, cmd FavoriteCmd) (FavoriteResult, error) {
	var out FavoriteResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}

		item, err := r.Favorites().FindItemAnyFolder(ctx, cmd.ArticleID, cmd.Actor.UserID)
		if err != nil {
			return err
		}
		if err := r.Favorites().DeleteItem(ctx, item.ArticleID(), item.UserID(), item.FolderID()); err != nil {
			return err
		}
		if err := r.Counters().Add(ctx, cmd.ArticleID, domain.Counters{Favor: -1}); err != nil {
			return err
		}

		if err := enrichCounters(ctx, r, a); err != nil {
			return err
		}
		out = FavoriteResult{IsFavor: false, FavorCount: a.Counters().Favor}
		// 一篇文章可以收在多个收藏夹：只移除了其中一个时 is_favor 仍为 true。
		if _, err := r.Favorites().FindItemAnyFolder(ctx, cmd.ArticleID, cmd.Actor.UserID); err == nil {
			out.IsFavor = true
		}
		return nil
	})

	return out, err
}

// Top 置顶/取消置顶文章。
//
// 只有作者能置顶自己的文章（domain.CanBeTopped）。旧实现没有数量上限，
// 这里保持一致，不做额外限制。
func (s *Service) Top(ctx context.Context, cmd TopCmd) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeTopped(a, cmd.Actor) {
			return domain.ErrForbidden
		}

		if cmd.Remove {
			return r.Tops().Delete(ctx, cmd.Actor.UserID, a.ID())
		}
		return r.Tops().Save(ctx, top.NewEntry(cmd.Actor.UserID, a.ID()))
	})
}

// Digg 把点赞关系收敛到目标态，返回操作后的权威状态与计数（ADR-0024 决策 3/4）。
//
// 语义与旧 toggle 的区别：
//   - 传 `Liked=true` 时"确保已赞"：已赞就是无操作，不再反向取消；
//   - 传 `Liked=false` 时"确保未赞"：未赞就是无操作。
//
// 关系落库（防重复），计数走缓存增量。两者分开是既有设计：
// 增量丢了只影响展示数字，关系丢了会导致重复计数。
// 通知只在**真新增**时发（取消 / 同态重复都不通知，与旧实现一致）。
func (s *Service) Digg(ctx context.Context, cmd DiggCmd) (DiggResult, error) {
	var out DiggResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeViewedBy(a, cmd.Actor) || !cmd.Actor.IsLoggedIn() {
			return domain.ErrArticleNotFound
		}

		exists, err := r.Diggs().Exists(ctx, cmd.ArticleID, cmd.Actor.UserID)
		if err != nil {
			return err
		}

		switch {
		case cmd.Liked && !exists:
			added, err := r.Diggs().Add(ctx, cmd.ArticleID, cmd.Actor.UserID)
			if err != nil {
				return err
			}
			if added {
				if err := r.Counters().Add(ctx, cmd.ArticleID, domain.Counters{Digg: 1}); err != nil {
					return err
				}
				if err := emitWithActor(ctx, r, EventArticleDigg, cmd.ArticleID, cmd.Actor.UserID); err != nil {
					return err
				}
			}
		case !cmd.Liked && exists:
			removed, err := r.Diggs().Remove(ctx, cmd.ArticleID, cmd.Actor.UserID)
			if err != nil {
				return err
			}
			if removed {
				if err := r.Counters().Add(ctx, cmd.ArticleID, domain.Counters{Digg: -1}); err != nil {
					return err
				}
			}
		}

		// 无论是真变化还是同态重复，都回填当前权威计数：前端直接用它 patch。
		if err := enrichCounters(ctx, r, a); err != nil {
			return err
		}
		out = DiggResult{IsDigg: cmd.Liked, DiggCount: a.Counters().Digg}
		return nil
	})

	return out, err
}

// RecordView 记录一次浏览。
//
// 两个动作：浏览量 +1（缓存增量）、写浏览历史（登录用户才写）。
// 匿名用户只加浏览量——与旧实现一致，匿名没有历史记录。
func (s *Service) RecordView(ctx context.Context, articleID domain.ArticleID, viewer domain.Viewer) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if !domain.CanBeViewedBy(a, viewer) {
			return domain.ErrArticleNotFound
		}

		if err := r.Counters().Add(ctx, articleID, domain.Counters{View: 1}); err != nil {
			return err
		}
		if viewer.IsLoggedIn() {
			return r.ViewHistory().Upsert(ctx, viewhistory.NewRecord(viewer.UserID, articleID, s.clock.Now()))
		}
		return nil
	})
}

// ListViewHistory 分页列出浏览历史。
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面：浏览历史同样属于
// 公开读面，开关关闭时作者不是管理员的记录不再出现（ADR-0017 决策 4）。
// 与收藏内容一致，过滤下推到查询层，保证 total 与可见条目对得上。
func (s *Service) ListViewHistory(ctx context.Context, viewer domain.Viewer, page domain.Page, onlyAdminAuthors bool) ([]*viewhistory.Record, int64, error) {
	if !viewer.IsLoggedIn() {
		return nil, 0, domain.ErrForbidden
	}
	var (
		records []*viewhistory.Record
		total   int64
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p := page.Normalize()
		var err error
		records, total, err = r.ViewHistory().List(ctx, viewer.UserID, p.Offset, p.Limit, onlyAdminAuthors)
		return err
	})
	return records, total, err
}

// RemoveViewHistory 删除若干条浏览历史。
func (s *Service) RemoveViewHistory(ctx context.Context, viewer domain.Viewer, articleIDs []domain.ArticleID) error {
	if !viewer.IsLoggedIn() {
		return domain.ErrForbidden
	}
	if len(articleIDs) == 0 {
		return fmt.Errorf("%w: 未指定要删除的记录", domain.ErrInvalidTransition)
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		return r.ViewHistory().Delete(ctx, viewer.UserID, articleIDs)
	})
}

// ListTopCards 列出某用户置顶文章的展示卡片。
//
// 曾经还有一个 `ListTops`（只返回 ID），注释里写着"设置置顶时前端只用得到 ID"；
// 实际上设置置顶走的是 `ToggleTop`，从来不需要先读一遍 ID 列表。
// 首页精选区块改成返回卡片之后它就彻底没人调了，于是一并删除——
// 连同仓储端口上的 `ListArticleIDs`（唯一调用方就是它）。
func (s *Service) ListTopCards(ctx context.Context, userID domain.UserID) ([]top.Card, error) {
	var cards []top.Card
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		cards, err = r.Tops().ListCards(ctx, userID)
		return err
	})
	return cards, err
}
