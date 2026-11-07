package app

import (
	"context"
	"strings"

	"myblogx/internal/article/domain"
)

// reviewReason 校验并规范化审核动作的驳回原因。
//
// 通过不需要原因；驳回必须给出一句非空的话（去掉首尾空白后仍非空）。
// 这条规则放用例层而不是只放请求层：管理后台、脚本、将来的批量审核都走同一条
// 用例，绕过 HTTP 不能绕过它。返回的 trimmed 值要原样落库——作者看到的就是它，
// 让手滑打出的首尾换行跟着送达没有任何价值。
func reviewReason(approve bool, reason string) (string, error) {
	if approve {
		return "", nil
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", domain.ErrReviewReasonRequired
	}
	return trimmed, nil
}

// Examine 审核文章。
//
// 权限与状态都由 domain 判断：只有管理员、且文章处于审核中才能审。
// 审核通过/拒绝之后的副作用（评分、通知、收藏快照同步）通过事件交给 adapter 分发，
// 保证 app 不直接依赖 ai / message / read 模块。
func (s *Service) Examine(ctx context.Context, cmd ExamineCmd) (*domain.Article, error) {
	if !cmd.Actor.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	reason, err := reviewReason(cmd.Approve, cmd.Reason)
	if err != nil {
		return nil, err
	}

	var result *domain.Article

	err = s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, cmd.ArticleID)
		if err != nil {
			return err
		}
		if !domain.CanBeExaminedBy(a, cmd.Actor) {
			return domain.ErrInvalidTransition
		}

		now := s.clock.Now()
		if cmd.Approve {
			if err := a.Approve(cmd.Actor.UserID, now); err != nil {
				return err
			}
		} else {
			if err := a.Reject(cmd.Actor.UserID, now); err != nil {
				return err
			}
		}

		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}

		// 撤销该文章所有待处理任务（审核完成即出队）。
		tasks, err := r.Reviews().ListPendingByArticle(ctx, a.ID())
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if cmd.Approve {
				if err := t.Approve(cmd.Actor.UserID, now); err != nil {
					return err
				}
			} else {
				if err := t.Reject(cmd.Actor.UserID, reason, now); err != nil {
					return err
				}
			}
			if err := r.Reviews().Update(ctx, t); err != nil {
				return err
			}
		}

		result = a
		if cmd.Approve {
			return emit(ctx, r, EventArticlePublished, a.ID())
		}
		return emit(ctx, r, EventArticleRejected, a.ID())
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}
