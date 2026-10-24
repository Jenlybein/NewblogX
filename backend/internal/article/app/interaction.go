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
func (s *Service) Favorite(ctx context.Context, cmd FavoriteCmd) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
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
		// 通知作者"有人收藏了你的文章"（旧实现同样只在收藏成功时发）。
		return emitWithActor(ctx, r, EventArticleFavorited, a.ID(), cmd.Actor.UserID)
	})
}

// Unfavorite 取消收藏。
func (s *Service) Unfavorite(ctx context.Context, cmd FavoriteCmd) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		item, err := r.Favorites().FindItemAnyFolder(ctx, cmd.ArticleID, cmd.Actor.UserID)
		if err != nil {
			return err
		}
		if err := r.Favorites().DeleteItem(ctx, item.ArticleID(), item.UserID(), item.FolderID()); err != nil {
			return err
		}
		return r.Counters().Add(ctx, cmd.ArticleID, domain.Counters{Favor: -1})
	})
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

// Digg 点赞/取消点赞，返回操作后是否处于"已点赞"状态。
//
// 关系落库（防重复），计数走缓存增量。两者分开是既有设计：
// 增量丢了只影响展示数字，关系丢了会导致重复计数。
func (s *Service) Digg(ctx context.Context, articleID domain.ArticleID, viewer domain.Viewer) (bool, error) {
	var liked bool

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		if !domain.CanBeViewedBy(a, viewer) || !viewer.IsLoggedIn() {
			return domain.ErrArticleNotFound
		}

		exists, err := r.Diggs().Exists(ctx, articleID, viewer.UserID)
		if err != nil {
			return err
		}

		if exists {
			removed, err := r.Diggs().Remove(ctx, articleID, viewer.UserID)
			if err != nil {
				return err
			}
			if removed {
				if err := r.Counters().Add(ctx, articleID, domain.Counters{Digg: -1}); err != nil {
					return err
				}
			}
			liked = false
			return nil
		}

		added, err := r.Diggs().Add(ctx, articleID, viewer.UserID)
		if err != nil {
			return err
		}
		if added {
			if err := r.Counters().Add(ctx, articleID, domain.Counters{Digg: 1}); err != nil {
				return err
			}
			// 只在"新增点赞"时通知作者，取消点赞不通知（与旧实现一致）。
			if err := emitWithActor(ctx, r, EventArticleDigg, articleID, viewer.UserID); err != nil {
				return err
			}
		}
		liked = true
		return nil
	})

	return liked, err
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
func (s *Service) ListViewHistory(ctx context.Context, viewer domain.Viewer, page domain.Page) ([]*viewhistory.Record, int64, error) {
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
		records, total, err = r.ViewHistory().List(ctx, viewer.UserID, p.Offset, p.Limit)
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

// ListTops 列出某用户置顶的文章 ID。
func (s *Service) ListTops(ctx context.Context, userID domain.UserID) ([]domain.ArticleID, error) {
	var ids []domain.ArticleID
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		ids, err = r.Tops().ListArticleIDs(ctx, userID)
		return err
	})
	return ids, err
}
