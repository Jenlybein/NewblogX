package app

import (
	"context"
	"strings"
	"unicode/utf8"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
)

// reviewReason 校验并规范化审核动作的驳回原因。
//
// 通过不需要原因（带了也丢弃）；驳回必须从固定枚举里选一条，"其他原因"还必须
// 补一句自由文本——否则作者收到的只有"其他原因"，等于没说。
//
// 这条规则放用例层而不是只放请求层：管理后台、脚本、将来的批量审核都走同一条
// 用例，绕过 HTTP 不能绕过它。返回的是**规范化后的枚举与补充说明**，编码进
// reason 列由 review.Task.Reject 一处完成（零 DDL：既有表不能加列，枚举值编在 reason 里）。
//
// 长度按**字符**算而不是字节（utf8.RuneCountInString）：原因是中文，按字节算会让
// 上限变成"一百多个字"，与 varchar(500) 的字符语义对不上。
func reviewReason(approve bool, code review.ReasonCode, note string) (review.ReasonCode, string, error) {
	if approve {
		return "", "", nil
	}
	if review.ReasonLabel(code) == "" {
		return "", "", domain.ErrReviewReasonInvalid
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > review.ReasonNoteMaxLen {
		return "", "", domain.ErrReviewReasonTooLong
	}
	if code == review.ReasonOther && note == "" {
		return "", "", domain.ErrReviewReasonRequired
	}
	return code, note, nil
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
	reasonCode, reasonNote, err := reviewReason(cmd.Approve, cmd.ReasonCode, cmd.ReasonNote)
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
				if err := t.Reject(cmd.Actor.UserID, reasonCode, reasonNote, now); err != nil {
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
