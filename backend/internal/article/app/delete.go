package app

import (
	"context"

	"myblogx/internal/article/domain"
)

// Delete 删除文章，并在同一事务里清理关联数据。
//
// 与旧实现（service/article_service/delete.go 的 DeleteArticles）等价，
// 但修正了两处：
//   - 旧实现逐篇 `select 再 delete` 六张表，N+1 严重；这里改成按文章一次性清理；
//   - 旧实现把数据库句柄从 handler 传进 service；这里改为事务内的仓储调用。
//
// 清理范围（软删语义，与 Model.DeletedAt 一致）：
// 评论 / 点赞 / 收藏 / 置顶 / 浏览历史 / 文章标签关系 / 文章本体。
func (s *Service) Delete(ctx context.Context, cmd DeleteCmd) (*DeleteResult, error) {
	var result *DeleteResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeDeletedBy(a, cmd.Actor) {
			return domain.ErrArticleNotFound
		}

		res := &DeleteResult{ArticleID: a.ID()}

		// 1) 评论（跨模块）
		n, err := r.Comments().DeleteByArticle(ctx, a.ID())
		if err != nil {
			return err
		}
		res.Comments = n

		// 2) 点赞
		n, err = r.Diggs().DeleteByArticle(ctx, a.ID())
		if err != nil {
			return err
		}
		res.Diggs = n

		// 3) 收藏
		n, err = r.Favorites().CountByArticle(ctx, a.ID())
		if err != nil {
			return err
		}
		res.Favors = n
		if err := r.Favorites().DeleteItemsByArticle(ctx, a.ID()); err != nil {
			return err
		}

		// 4) 置顶
		if err := r.Tops().DeleteByArticle(ctx, a.ID()); err != nil {
			return err
		}

		// 5) 浏览历史
		if err := r.ViewHistory().DeleteByArticle(ctx, a.ID()); err != nil {
			return err
		}

		// 6) 文章标签关系
		tagIDs, err := r.Tags().ArticleTagIDs(ctx, a.ID())
		if err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			if err := r.Tags().ReplaceArticleTags(ctx, a.ID(), nil); err != nil {
				return err
			}
		}
		res.ArticleTags = len(tagIDs)

		// 7) 作者统计：文章数 -1，浏览量扣除该文当前浏览数
		//
		// 与旧实现一致：`StatApplyArticleDelta(tx, authorID, -1, -(viewCount + delta))`。
		viewTotal := a.Counters().View
		if err := r.AuthorStats().ApplyArticleDelta(ctx, a.AuthorID(), -1, -viewTotal); err != nil {
			return err
		}

		// 8) 文章本体（软删）
		a.SoftDelete(s.clock.Now())
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}

		result = res
		return emit(ctx, r, EventArticleDeleted, a.ID())
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}
