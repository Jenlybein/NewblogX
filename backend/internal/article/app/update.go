package app

import (
	"context"

	"myblogx/internal/article/domain"
)

// Update 更新文章。
//
// 与旧 UpdateArticle 等价：
//   - 只有作者能改（旧实现用 `id = ? AND author_id = ?` 查询，查不到即"不存在"）；
//   - 显式要求草稿 → 退回草稿并撤销待审任务；
//   - 显式要求提交 → 进审核队列；
//   - 改的是实质内容且当前已发布/审核中 → 自动重新送审（可见性变更不算）。
//
// 质量闸门开启时，评分调用必须发生在数据库事务之外（票 306 / ADR-0023）：
// 因此先用一次**只读预演**（previewUpdate，事务内只读、不写库）算出"这次到底
// 会不会提交"以及"提交的标题/正文是什么"，然后预占配额、在事务外评分，
// 最后才进写事务（写事务会重新读一遍文章，以它读到的状态为准）。
func (s *Service) Update(ctx context.Context, cmd UpdateArticleCmd) (*UpdateResult, error) {
	var result *UpdateResult
	// 投稿配额的预占点在这里而不是事务之前：这次更新到底会不会"提交"，
	// 要读到文章当前状态才知道（草稿提交 vs 已发布文章编辑后自动送审）。
	// 失败时在函数末尾归还（票 304 / ADR-0022）。
	var reservation SubmitQuotaReservation
	gateRejected := false
	var verdict qualityVerdict
	riskRejected := false

	// 只读预演 + **事务外**的同步风险扫描与评分。两者都必须在 WithinTx 之外：
	// 风险扫描要读已有评分记录，评分是一次 5–30 秒的模型调用，占着事务等于
	// 钉死连接池（ADR-0023 硬纪律、ADR-0025 决策 1）。写事务会重新读一遍
	// 文章，以它读到的状态为准。
	var preview updatePreview
	if err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		p, err := s.previewUpdate(ctx, r, cmd)
		preview = p
		return err
	}); err != nil {
		return nil, err
	}

	// 预演说"这次会提交"就在事务外把配额、风险扫描、评分一次做完。
	// 闸门关不关都要扫（票 302 新规则），所以不再分两条路径、也不在事务内补扫。
	if preview.NeedsSubmit {
		res, err := s.reserveSubmitQuota(ctx, cmd.Actor)
		if err != nil {
			return nil, err
		}
		reservation = res

		// 站点「文章免审核」开启时全部直接真发布，同步风险扫描也不拦。
		if !cmd.SkipExamining {
			risk, err := s.scanSubmitRisk(ctx, preview.Title, preview.Content, cmd.ArticleID)
			if err != nil {
				s.releaseSubmitQuota(ctx, reservation)
				return nil, err
			}
			riskRejected = risk.Rejected
		}

		if cmd.QualityGate.Enabled && !cmd.QualityGate.Deferred && !riskRejected {
			gateVerdict, err := s.judgeQuality(ctx, cmd.QualityGate, QualityGateInput{
				ArticleID:     cmd.ArticleID,
				AuthorID:      cmd.Actor.UserID,
				Title:         preview.Title,
				Content:       preview.Content,
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

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeEditedBy(a, cmd.Actor) {
			// 与旧实现一致：不是自己的文章一律当"不存在"，不暴露存在性。
			return domain.ErrArticleNotFound
		}

		// 站点「发布与展示」开关（ADR-0015 决策 2）：
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
		flags, err := a.ApplyEdit(editPatchFromCmd(cmd), now)
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
		// 已发布 + 跳过审核：目标态就是当前态。此时"提交"是幂等的，
		// 既不重建审核任务（否则已发布的文章会被塞进审核队列），也不刷新审核痕迹。
		// 票 302 新规则下免审核仍是"直接真发布"，所以直接看开关，
		// 不再用 ResolveSubmitStatus 反推。
		idempotentSubmit := explicitSubmit && a.Status() == domain.StatusPublished && cmd.SkipExamining

		switch {
		case explicitDraft:
			if err := a.BackToDraft(now); err != nil {
				return err
			}
			if err := cancelPendingReviewTasks(ctx, r, a.ID(), "作者改回草稿", now); err != nil {
				return err
			}
		case idempotentSubmit:
			// 什么都不做：文章已经是发布态，内容由 ApplyEdit 写过了。
		case explicitSubmit || autoResubmit:
			// 系统直接拒是终局（票 302 / ADR-0025「不给改」）：同步风险命中与
			// 合规自动拒都留了一条待处理复盘任务，按来源认。人工驳回的
			// StatusRejected 不在此列——#303 允许作者改稿后重投。
			if err := rejectSystemRejectedResubmit(ctx, r, a.ID()); err != nil {
				return err
			}
			// 配额、同步风险扫描与评分都已在写事务之外完成（见函数开头）；
			// 这里只消费那些结论，不再做任何事务内的端口调用。
			// 预演与写事务读到的是同一状态；若并发下状态变了，末尾的
			// `!result.Resubmitted` 分支会把预占归还。
			outcome := decideSubmit(cmd.SkipExamining, gateRejected, riskRejected)
			if outcome.Rejected {
				// 同步风险命中：直接 REJECTED，不给改。留一条复盘任务供管理员看。
				if err := a.SubmitWithOutcome(domain.StatusRejected, domain.VisibilityVisible, now); err != nil {
					return err
				}
				if err := cancelPendingReviewTasks(ctx, r, a.ID(), "内容检查未通过", now); err != nil {
					return err
				}
				if err := saveRiskRejectionTask(ctx, s, r, a, now); err != nil {
					return err
				}
				resubmitted = true
				break
			}
			if gateRejected {
				// 质量不达标：不放行，文章落草稿（spec 决策 24）。即使站点开启了
				// "跳过审核"也不直接发布 —— 灰带与低分都意味着"先改再投"。
				if err := a.BackToDraft(now); err != nil {
					return err
				}
				if err := cancelPendingReviewTasks(ctx, r, a.ID(), "文章质量未达标", now); err != nil {
					return err
				}
				break
			}

			if err := a.SubmitWithOutcome(outcome.Status, outcome.Visibility, now); err != nil {
				return err
			}
			// 干净文章不再在同步路径建人工任务：人工队列只由异步模型判定产生。
			// 但要撤销旧的待处理任务（例如这篇文章之前因模型存疑被改回审核中）。
			if err := cancelPendingReviewTasks(ctx, r, a.ID(), "作者重新提交", now); err != nil {
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

		return emit(ctx, r,
			submitEventName(a.Status(), riskRejected, explicitDraft, false), a.ID())
	})

	if err != nil {
		// 提交因系统故障失败：归还额度（票 304 验收标准）。
		s.releaseSubmitQuota(ctx, reservation)
		return nil, err
	}
	if gateRejected {
		// 文章已落草稿（改动保留），因此返回"带数据的业务拒绝"，不是静默失败。
		// 这次预占对应一次真实的提交动作（评分 + 落草稿），照旧计一次额度。
		return nil, newQualityRejection(cmd.ArticleID, verdict, cmd.QualityGate.PassScore)
	}
	if riskRejected {
		// 文章已以 REJECTED 落库（不给改），返回固定文案的业务拒绝。
		return nil, newRiskRejection()
	}
	// 预演说"这次会提交"、写事务却发现"其实没提交"（并发下文章已被改成已发布，
	// 或者这次改动没有任何字段变化）：预占还在，但没有任何提交动作对应它。
	// 不在这里归还，就是"一次没提交也扣掉一天额度"——而这个窗口恰恰是评分
	// 那 5–30 秒，最容易有并发的第二个人在改同一篇。
	if reservation != nil && !result.Resubmitted {
		s.releaseSubmitQuota(ctx, reservation)
	}
	return result, nil
}

// previewUpdate 是闸门开启时在事务外评分之前的只读预演。
//
// 它只回答两个问题：这次更新会不会真的提交？提交的标题与正文是什么？
// 不写库、不预占配额、也不校验分类与标签（那些仍由写事务负责并会重新读库）。
func (s *Service) previewUpdate(ctx context.Context, r Repos, cmd UpdateArticleCmd) (updatePreview, error) {
	a, err := r.Articles().Find(ctx, cmd.ArticleID)
	if err != nil {
		return updatePreview{}, err
	}
	if !domain.CanBeEditedBy(a, cmd.Actor) {
		return updatePreview{}, domain.ErrArticleNotFound
	}
	if err := domain.CheckWriteOnUpdate(a, cmd.Actor, cmd.TargetStatus, cmd.Policy); err != nil {
		return updatePreview{}, err
	}

	flags, err := a.ApplyEdit(editPatchFromCmd(cmd), s.clock.Now())
	if err != nil {
		return updatePreview{}, err
	}

	explicitDraft := cmd.TargetStatus != nil && *cmd.TargetStatus == domain.StatusDraft
	explicitSubmit := cmd.TargetStatus != nil && *cmd.TargetStatus == domain.StatusExamining
	autoResubmit := a.NeedsReReview(flags, cmd.SkipExamining)
	idempotentSubmit := explicitSubmit &&
		a.Status() == domain.StatusPublished &&
		domain.ResolveSubmitStatus(cmd.SkipExamining) == domain.StatusPublished

	if explicitDraft || idempotentSubmit || !(explicitSubmit || autoResubmit) {
		return updatePreview{}, nil
	}
	return updatePreview{NeedsSubmit: true, Title: a.Title(), Content: a.Content()}, nil
}

// editPatchFromCmd 把更新入参投影成领域层能应用的补丁。
func editPatchFromCmd(cmd UpdateArticleCmd) domain.EditPatch {
	return domain.EditPatch{
		Title:          cmd.Title,
		Abstract:       cmd.Abstract,
		Content:        cmd.Content,
		CategoryID:     cmd.CategoryID,
		CategorySet:    cmd.CategorySet,
		Cover:          cmd.Cover,
		CommentsToggle: cmd.CommentsToggle,
		Visibility:     cmd.Visibility,
	}
}
