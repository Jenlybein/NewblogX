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
// 返回值是操作后的权威状态与收藏总数（ADR-0016 决策 4）：前端据此直接 patch，
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
		// 成员变了，夹子的 article_count 与 updated_at 必须在同一事务内刷新：
		// 计数一旦靠"下次登录再修"就会长期说谎（#344）。
		if err := r.Favorites().RefreshFolderStats(ctx, folderID, s.clock.Now()); err != nil {
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

// Unfavorite 取消收藏：只移除**目标收藏夹**里的那条关系。
//
// 旧实现用 `FindItemAnyFolder` 取任意一条、再按 `item.FolderID()` 删除，完全忽略
// 调用方传入的 folder。同一篇文章同时收在 A/B 两个收藏夹时，点「移出 A」可能删掉
// B 的那条：A 的勾还在、toast 却说「已从 A 移除」，而 B 的收藏凭空消失。
// 现在的口径：
//   - `FolderID` 非空：只在目标收藏夹里找。找不到就报 `ErrItemNotFound`（按「不存在」
//     处理），绝不顺手删别的夹子里的关系；
//   - `FolderID` 为零：与 {@link Service.Favorite} 同口径，落到默认收藏夹
//     （`FavoriteCmd.FolderID` 的既有语义），不会退化回「随便删一条」。
//
// 返回值上的 `IsFavor` 语义是「该用户**是否还在任一收藏夹**里收着这篇文章」：
// 一篇文章可以收在多个收藏夹，只移除了其中一个时它仍为 true。详情接口的
// `is_favor`（`query.go` 的 `FindItemAnyFolder`）用的是同一条语义。
func (s *Service) Unfavorite(ctx context.Context, cmd FavoriteCmd) (FavoriteResult, error) {
	var out FavoriteResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}

		folderID := cmd.FolderID
		if folderID.IsZero() {
			f, err := r.Favorites().DefaultFolder(ctx, cmd.Actor.UserID)
			if err != nil {
				return err
			}
			folderID = f.ID()
		}

		// 只认目标夹里的关系：不存在就是不存在（ErrItemNotFound），不猜、不兜底。
		if _, err := r.Favorites().FindItem(ctx, cmd.ArticleID, cmd.Actor.UserID, folderID); err != nil {
			return err
		}
		if err := r.Favorites().DeleteItem(ctx, cmd.ArticleID, cmd.Actor.UserID, folderID); err != nil {
			return err
		}
		// 与收藏对称：移出也是一次成员变更，计数与夹子更新时间一起刷新。
		if err := r.Favorites().RefreshFolderStats(ctx, folderID, s.clock.Now()); err != nil {
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

// Digg 把点赞关系收敛到目标态，返回操作后的权威状态与计数（ADR-0016 决策 3/4）。
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

// ListViewHistory 列出浏览历史的一页（游标分页）。
//
// onlyAdminAuthors 是「显示非管理员发布文章」开关的反面：浏览历史同样属于
// 公开读面，开关关闭时作者不是管理员的记录不再出现（ADR-0015 决策 4）。
// 与收藏内容一致，过滤下推到查询层，保证分页在**过滤之后**进行。
//
// 为什么不再返回 total：游标分页只答一个方向的问题（"还有没有下一页"），
// 多取一条即可；total 反而会随 keyword / 游标无关的删除来回变，前端用不上。
func (s *Service) ListViewHistory(ctx context.Context, viewer domain.Viewer, q ViewHistoryQuery, onlyAdminAuthors bool) (ViewHistoryPage, error) {
	if !viewer.IsLoggedIn() {
		return ViewHistoryPage{}, domain.ErrForbidden
	}

	var page ViewHistoryPage
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		lq := viewhistory.ListQuery{
			UserID:           viewer.UserID,
			Keyword:          q.Keyword,
			OnlyAdminAuthors: onlyAdminAuthors,
			Before:           q.Before,
			Limit:            q.Limit,
		}.Normalize()

		// 多取一条判断 has_more：取到 limit+1 条就是"还有下一页"，
		// 最后一条切掉不外发（它只是探针）。
		limit := lq.Limit
		lq.Limit = limit + 1
		records, err := r.ViewHistory().List(ctx, lq)
		if err != nil {
			return err
		}
		if len(records) > limit {
			records = records[:limit]
			page.HasMore = true
		}
		page.Records = records
		return nil
	})
	return page, err
}

// ClearViewHistory 清空当前用户的全部浏览历史。
//
// 不可逆操作（软删）：前端必须二次确认。只动 viewer 自己的行——
// 与 RemoveViewHistory 一样，用户 ID 只来自会话，不接受请求参数。
func (s *Service) ClearViewHistory(ctx context.Context, viewer domain.Viewer) error {
	if !viewer.IsLoggedIn() {
		return domain.ErrForbidden
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		return r.ViewHistory().DeleteAll(ctx, viewer.UserID)
	})
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
		if err != nil {
			return err
		}
		// 与详情 / 热门置顶同一权威读入口（#279）。
		persisted := make(map[domain.ArticleID]int, len(cards))
		for _, c := range cards {
			persisted[c.ArticleID] = c.ViewCount
		}
		views, err := ledgerOf(r).ApplyViews(ctx, persisted)
		if err != nil {
			return err
		}
		for i := range cards {
			cards[i].ViewCount = views[cards[i].ArticleID]
		}
		return nil
	})
	return cards, err
}
