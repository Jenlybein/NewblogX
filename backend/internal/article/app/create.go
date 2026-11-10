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

	// 站点「发布与展示」开关：草稿看 draft 开关、提交审核/创建即发布看 publish 开关。
	// 判定放在事务之前——被站点策略拦下的请求一个字节都不该落库。
	// （ADR-0017 决策 2；管理员不受影响，见 domain.CheckWriteOnCreate。）
	if err := domain.CheckWriteOnCreate(cmd.Actor, target, cmd.Policy); err != nil {
		return nil, err
	}

	// 投稿配额（票 304 / ADR-0031）：只有提交才算一次提交动作，存草稿不计。
	// 预占放在事务之前——超限时一个字节都不该落库；闸门落地后评分调用也要排在
	// 预占之后，否则"评分成功才发现超限"会白烧一次模型调用。
	var reservation SubmitQuotaReservation
	gateRejected := false
	var verdict qualityVerdict
	// riskRejected 是提交时同步风险扫描命中 medium 及以上的结论（票 302 新规则）。
	// 它与 gateRejected 的落点不同：质量不达标落草稿可改，风险命中直接 REJECTED。
	riskRejected := false
	// 新建文章在评分时还没有落库，但评分记录要按真实的 article_id 写，
	// 所以闸门开启时先在事务外生成 ID，写事务复用它。
	var articleID domain.ArticleID
	if target == domain.StatusExamining {
		res, err := s.reserveSubmitQuota(ctx, cmd.Actor)
		if err != nil {
			return nil, err
		}
		reservation = res

		// 站点「文章免审核」开启时按用户选定的口径：全部直接真发布，
		// 同步风险扫描也不拦（异步合规判定仍作事后兜底）。
		if !cmd.SkipExamining {
			risk, err := s.scanSubmitRisk(ctx, cmd.Title, cmd.Content, domain.ArticleID(""))
			if err != nil {
				s.releaseSubmitQuota(ctx, reservation)
				return nil, err
			}
			riskRejected = risk.Rejected
		}

		if cmd.QualityGate.Enabled && !riskRejected {
			// 评分必须在数据库事务之外（票 306 / ADR-0032）：一次模型调用 5–30 秒，
			// 把它留在 WithinTx 里等于钉死连接池。顺序固定为
			// 配额预占 → 风险扫描 → 评分 → 判定 → 落库。
			articleID = s.ids.NextArticleID()
			gateVerdict, err := s.judgeQuality(ctx, cmd.QualityGate, QualityGateInput{
				ArticleID:     articleID,
				AuthorID:      cmd.Actor.UserID,
				Title:         cmd.Title,
				Content:       cmd.Content,
				UpdatedAt:     s.clock.Now(),
				AuthorIsAdmin: cmd.Actor.IsAdmin(),
			})
			if err != nil {
				// 拿不到可用分数：本次提交不落库，归还配额（失败关闭）。
				s.releaseSubmitQuota(ctx, reservation)
				return nil, err
			}
			verdict = gateVerdict
			gateRejected = !gateVerdict.Passed
		}
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
		outcome := decideSubmit(cmd.SkipExamining, gateRejected, riskRejected)
		status := domain.StatusDraft
		visibility := domain.NormalizeVisibility(cmd.Visibility)
		var submittedAt *time.Time
		if target == domain.StatusExamining {
			status = outcome.Status
			visibility = outcome.Visibility
			t := now
			submittedAt = &t
		}
		if articleID.IsZero() {
			articleID = s.ids.NextArticleID()
		}

		article, err := domain.NewArticle(domain.NewArticleParams{
			ID:             articleID,
			AuthorID:       cmd.Actor.UserID,
			Title:          cmd.Title,
			Abstract:       cmd.Abstract,
			Content:        cmd.Content,
			CategoryID:     cmd.CategoryID,
			Cover:          cmd.Cover,
			CommentsToggle: cmd.CommentsToggle,
			Status:         status,
			Visibility:     visibility,
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
		if riskRejected {
			if err := saveRiskRejectionTask(ctx, s, r, article, now); err != nil {
				return err
			}
		}
		// 作者文章数 +1，与旧实现一致（同步、同事务）。
		if err := r.AuthorStats().ApplyArticleDelta(ctx, article.AuthorID(), 1, 0); err != nil {
			return err
		}

		created = article
		return emit(ctx, r, submitEventName(status, riskRejected, false, true), article.ID())
	})

	if err != nil {
		// 提交因系统故障失败：归还额度，不因平台问题罚作者（票 304 验收标准）。
		s.releaseSubmitQuota(ctx, reservation)
		return nil, err
	}
	if gateRejected {
		// 文章已经以草稿落库（作者能继续改、能立刻重投），因此这里是"带数据的
		// 业务拒绝"而不是 "什么都没发生"：错误码 + 分数 / 差额 / 问题列表。
		return nil, newQualityRejection(created.ID(), verdict, cmd.QualityGate.PassScore)
	}
	if riskRejected {
		// 文章已以 REJECTED 落库（不给改），返回固定文案的业务拒绝。
		// 命中词、维度、等级都不带给作者。
		return nil, newRiskRejection()
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
