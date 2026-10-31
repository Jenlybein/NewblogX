package app

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
)

// Update 更新文章。
//
// 与旧 UpdateArticle 等价：
//   - 只有作者能改（旧实现用 `id = ? AND author_id = ?` 查询，查不到即"不存在"）；
//   - 显式要求草稿 → 退回草稿并撤销待审任务；
//   - 显式要求提交 → 进审核队列；
//   - 改的是实质内容且当前已发布/审核中 → 自动重新送审（可见性变更不算）。
func (s *Service) Update(ctx context.Context, cmd UpdateArticleCmd) (*UpdateResult, error) {
	var result *UpdateResult

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeEditedBy(a, cmd.Actor) {
			// 与旧实现一致：不是自己的文章一律当"不存在"，不暴露存在性。
			return domain.ErrArticleNotFound
		}

		// 站点「发布与展示」开关（ADR-0017 决策 2）：
		//   - 关闭发布 → 不能更新已发布/审核中的文章、不能发布自己的草稿；
		//   - 关闭草稿 → 存量草稿可打开可删除，但保存（含退回草稿）会被拒。
		// 判定必须在 ApplyEdit 之前：先改内存再报错会让聚合带上"被拒绝的修改"。
		if err := domain.CheckWriteOnUpdate(a, cmd.Actor, cmd.TargetStatus, cmd.Policy); err != nil {
			return err
		}

		if cmd.CategorySet && cmd.CategoryID != nil {
			if err := ensureCategoryOwned(ctx, r, a.AuthorID(), *cmd.CategoryID); err != nil {
				return err
			}
		}

		var tagIDs []domain.TagID
		if cmd.TagIDs != nil {
			tags, err := loadUsableTags(ctx, r, *cmd.TagIDs)
			if err != nil {
				return err
			}
			for _, t := range tags {
				tagIDs = append(tagIDs, t.ID())
			}
		}

		now := s.clock.Now()
		flags, err := a.ApplyEdit(domain.EditPatch{
			Title:          cmd.Title,
			Abstract:       cmd.Abstract,
			Content:        cmd.Content,
			CategoryID:     cmd.CategoryID,
			CategorySet:    cmd.CategorySet,
			Cover:          cmd.Cover,
			CommentsToggle: cmd.CommentsToggle,
			Visibility:     cmd.Visibility,
		}, now)
		if err != nil {
			return err
		}

		tagsReplaced := cmd.TagIDs != nil
		if tagsReplaced {
			a.SetTags(tagIDs, now)
		}

		// 没有任何字段变化时直接返回，不写库（旧实现的 Noop 分支）。
		if flags == (domain.EditFlags{}) && !tagsReplaced && cmd.TargetStatus == nil {
			result = &UpdateResult{Article: a, Changed: false}
			return nil
		}

		resubmitted := false
		explicitDraft := cmd.TargetStatus != nil && *cmd.TargetStatus == domain.StatusDraft
		explicitSubmit := cmd.TargetStatus != nil && *cmd.TargetStatus == domain.StatusExamining
		autoResubmit := a.NeedsReReview(flags, cmd.SkipExamining)

		switch {
		case explicitDraft:
			if err := a.BackToDraft(now); err != nil {
				return err
			}
			if err := cancelPendingReviewTasks(ctx, r, a.ID(), "作者改回草稿", now); err != nil {
				return err
			}
		case explicitSubmit || autoResubmit:
			if err := a.Submit(now, cmd.SkipExamining); err != nil {
				return err
			}
			reason := "作者重新提交审核"
			if explicitSubmit {
				reason = "作者提交审核"
			}
			if err := cancelPendingReviewTasks(ctx, r, a.ID(), reason, now); err != nil {
				return err
			}
			source := review.SourceResubmit
			if autoResubmit {
				source = review.SourceEdit
			}
			if err := createReviewTask(ctx, s, r, a, source, now); err != nil {
				return err
			}
			resubmitted = true
		}

		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		if tagsReplaced {
			if err := r.Tags().ReplaceArticleTags(ctx, a.ID(), tagIDs); err != nil {
				return err
			}
		}

		result = &UpdateResult{Article: a, Changed: true, Resubmitted: resubmitted, TagsReplaced: tagsReplaced}

		event := EventArticleUpdated
		switch {
		case explicitDraft:
			event = EventArticleBackToDraft
		case a.Status() == domain.StatusExamining:
			event = EventArticleSubmitted
		}
		return emit(ctx, r, event, a.ID())
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}
