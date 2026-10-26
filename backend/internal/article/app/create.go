package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
)

// 事件名。adapter 侧按名字分发到下游（ES 投影、通知、评分）。
const (
	// EventArticleCreated 文章创建成功。
	EventArticleCreated = "article.created"
	// EventArticleUpdated 文章内容变更。
	EventArticleUpdated = "article.updated"
	// EventArticleSubmitted 进入审核队列。
	EventArticleSubmitted = "article.submitted"
	// EventArticlePublished 审核通过/直接发布。
	EventArticlePublished = "article.published"
	// EventArticleRejected 审核拒绝。
	EventArticleRejected = "article.rejected"
	// EventArticleDeleted 文章被删除。
	EventArticleDeleted = "article.deleted"
	// EventArticleBackToDraft 作者退回草稿。
	EventArticleBackToDraft = "article.back_to_draft"
	// EventArticleDigg 文章被点赞（只在"新增点赞"时发，取消点赞不发）。
	EventArticleDigg = "article.digg"
	// EventArticleFavorited 文章被收藏。
	EventArticleFavorited = "article.favorited"
)

// Create 创建文章。
//
// 与旧 CreateArticle 等价的动作序列：
//  1. 校验分类归属、标签可用（脏输入整单失败，而不是静默丢弃）；
//  2. 按站点配置把"提交"解析成 审核中 或 已发布；
//  3. 同一事务内：写文章 + 写标签关系 +（需审核时）建审核任务 + 作者文章数 +1。
func (s *Service) Create(ctx context.Context, cmd CreateArticleCmd) (*domain.Article, error) {
	if !cmd.Actor.IsLoggedIn() {
		return nil, domain.ErrForbidden
	}

	// 只允许草稿或提交审核；不允许直接指定"已发布"绕过审核。
	target := cmd.TargetStatus
	if target != domain.StatusDraft && target != domain.StatusExamining {
		return nil, domain.ErrInvalidTransition
	}

	var created *domain.Article

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if cmd.CategoryID != nil {
			if err := ensureCategoryOwned(ctx, r, cmd.Actor.UserID, *cmd.CategoryID); err != nil {
				return err
			}
		}
		tags, err := loadUsableTags(ctx, r, cmd.TagIDs)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		status := domain.StatusDraft
		var submittedAt *time.Time
		if target == domain.StatusExamining {
			status = domain.ResolveSubmitStatus(cmd.SkipExamining)
			t := now
			submittedAt = &t
		}

		article, err := domain.NewArticle(domain.NewArticleParams{
			ID:             s.ids.NextArticleID(),
			AuthorID:       cmd.Actor.UserID,
			Title:          cmd.Title,
			Abstract:       cmd.Abstract,
			Content:        cmd.Content,
			CategoryID:     cmd.CategoryID,
			Cover:          cmd.Cover,
			CommentsToggle: cmd.CommentsToggle,
			Status:         status,
			Visibility:     cmd.Visibility,
			SubmittedAt:    submittedAt,
			Now:            now,
		})
		if err != nil {
			return err
		}

		tagIDs := make([]domain.TagID, 0, len(tags))
		for _, t := range tags {
			tagIDs = append(tagIDs, t.ID())
		}
		if len(tagIDs) > 0 {
			article.SetTags(tagIDs, now)
		}

		if err := r.Articles().Create(ctx, article); err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			if err := r.Tags().ReplaceArticleTags(ctx, article.ID(), tagIDs); err != nil {
				return err
			}
		}
		if status == domain.StatusExamining {
			if err := createReviewTask(ctx, s, r, article, review.SourceCreate, now); err != nil {
				return err
			}
		}
		// 作者文章数 +1，与旧实现一致（同步、同事务）。
		if err := r.AuthorStats().ApplyArticleDelta(ctx, article.AuthorID(), 1, 0); err != nil {
			return err
		}

		created = article
		return emit(ctx, r, EventArticleCreated, article.ID())
	})

	if err != nil {
		return nil, err
	}
	return created, nil
}

// ensureCategoryOwned 校验分类存在且属于该用户。
//
// 旧实现把"不存在"和"不属于我"都报成"分类不存在"——顺带避免了探测他人分类 ID。
func ensureCategoryOwned(ctx context.Context, r Repos, userID domain.UserID, id domain.CategoryID) error {
	c, err := r.Categories().Find(ctx, id)
	if err != nil {
		return err
	}
	if !c.BelongsTo(userID) {
		return taxonomy.ErrCategoryNotFound
	}
	return nil
}

// loadUsableTags 读取并校验标签；带停用标签整单失败。
func loadUsableTags(ctx context.Context, r Repos, ids []domain.TagID) ([]*taxonomy.Tag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	tags, err := r.Tags().FindMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	// FindMany 会跳过缺失 ID，导致"请求 3 个只回来 2 个"也必须报错。
	if len(tags) != len(ids) {
		return nil, taxonomy.ErrTagDisabled
	}
	return taxonomy.FilterUsable(tags)
}

// createReviewTask 建一条待处理审核任务。
func createReviewTask(ctx context.Context, s *Service, r Repos, a *domain.Article, source review.Source, now time.Time) error {
	authorName := ""
	if u, err := r.Users().Profile(ctx, a.AuthorID()); err == nil {
		authorName = u.Nickname
	}
	task := review.NewTask(review.NewTaskParams{
		ID:            ctype.NewID(s.ids.NextReviewTaskID()),
		ArticleID:     a.ID(),
		AuthorID:      a.AuthorID(),
		ArticleTitle:  a.Title(),
		AuthorName:    authorName,
		PublishStatus: a.Status(),
		Source:        source,
		Now:           now,
	})
	return r.Reviews().Save(ctx, task)
}

// cancelPendingReviewTasks 撤销某文章所有待处理任务（改回草稿/重新提交时）。
func cancelPendingReviewTasks(ctx context.Context, r Repos, articleID domain.ArticleID, reason string, now time.Time) error {
	tasks, err := r.Reviews().ListPendingByArticle(ctx, articleID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if err := t.Cancel(reason, now); err != nil && !errors.Is(err, review.ErrTaskHandled) {
			return err
		}
		if err := r.Reviews().Update(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// emit 发一条领域事件。
//
// payload 只带 ID 等标量，不带整个聚合：事件消费者要详情会自己查，
// 避免事件体随实体字段膨胀（也是日志治理里"禁 any"的同一条原则）。
func emit(ctx context.Context, r Repos, name string, articleID domain.ArticleID) error {
	payload, err := json.Marshal(map[string]uint64{"article_id": articleID.Uint64()})
	if err != nil {
		return err
	}
	return r.Outbox().Emit(ctx, Event{
		Name:          name,
		Payload:       payload,
		AggregateID:   articleID,
		AggregateType: "article",
	})
}

// emitWithActor 发一条需要知道"是谁做的"的事件。
//
// 点赞/收藏通知要告诉作者"某某赞了你"，所以事件体里除了文章 ID
// 还必须带上操作者 ID。与 emit 一样只带标量，不带整个聚合。
func emitWithActor(ctx context.Context, r Repos, name string, articleID domain.ArticleID, actorID domain.UserID) error {
	payload, err := json.Marshal(map[string]uint64{
		"article_id": articleID.Uint64(),
		"actor_id":   actorID.Uint64(),
	})
	if err != nil {
		return err
	}
	return r.Outbox().Emit(ctx, Event{
		Name:          name,
		Payload:       payload,
		AggregateID:   articleID,
		AggregateType: "article",
	})
}
