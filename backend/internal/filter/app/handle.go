package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
)

// 本文件是风险页的处置用例（ADR-0032 决策 7 / 票 398）。
//
// 一次处置 = 判动作 → 取记录 → （删除时）软删内容 → 条件更新记录状态。
// 三处顺序都不能换：
//   - 先软删内容再改状态：内容删失败时记录仍是待处理，管理员重试即可；
//     反过来会留下"记录说已删除、内容还在公开面"的错位。
//   - 已处置的记录直接返回当前状态（幂等），不再动内容 —— 重试、双击、
//     两个人同时点都不该删第二次。
//   - 状态更新用条件更新（只改 pending 行），并发处置时后到的人拿到的是
//     第一个人的处理痕迹，而不是把它覆盖掉。

// HandleInput 是单条处置的输入。
type HandleInput struct {
	// ID 是命中记录的 ID。
	ID ctype.ID
	// Action 是处置动作；非法值在用例层被拒（不静默当成 ignore）。
	Action domain.Action
	// MuteDays 是禁言天数（只有 mute 动作读它）：1–365，0 表示"没传"按默认 7 天。
	// 越界由 user 模块校验并返回 user.mute_days_invalid（单一来源）。
	MuteDays int
	// Note 是可选备注（≤ domain.MaxNoteLen 字）。
	Note string
	// HandlerID 是发起处置的管理员 ID，写进记录行的处理人。
	HandlerID ctype.ID
}

// HandleResult 是一次处置的结果。
type HandleResult struct {
	// Hit 是处置后的记录（含处理人 / 时间 / 备注）。
	Hit domain.Hit
	// AlreadyHandled 表示这条记录在本次处置之前就已经被处置过，
	// 本次只是原样读回了当前状态（内容没有再被删）。
	AlreadyHandled bool
}

// Handle 处置一条关键词风险记录。
func (s *Service) Handle(ctx context.Context, in HandleInput) (HandleResult, error) {
	action, err := normalizeAction(in.Action)
	if err != nil {
		return HandleResult{}, err
	}
	note, err := domain.NormalizeNote(in.Note)
	if err != nil {
		return HandleResult{}, err
	}
	if in.ID.IsZero() {
		return HandleResult{}, apperr.New(apperr.CodeFilterHitNotFound)
	}
	if s.hits == nil {
		return HandleResult{}, apperr.New(apperr.CodeFilterDisposeUnavailable)
	}

	hit, err := s.hits.FindByID(ctx, in.ID)
	if err != nil {
		return HandleResult{}, err
	}
	// 幂等：已处置的记录不再动内容，直接返回当前状态。
	if hit.Status != domain.StatusPending {
		return HandleResult{Hit: hit, AlreadyHandled: true}, nil
	}

	if action.IsAccountAction() {
		if err := s.disposeAccount(ctx, hit, action, in.HandlerID, in.MuteDays); err != nil {
			return HandleResult{}, err
		}
	} else if action == domain.ActionDelete {
		if err := s.dispose(ctx, hit, in.HandlerID); err != nil {
			return HandleResult{}, err
		}
	}

	handledAt := time.Now()
	updated, err := s.hits.MarkHandled(ctx, hit.ID, domain.Handle{
		Status:    action.StatusFor(),
		HandlerID: in.HandlerID,
		HandledAt: handledAt,
		Note:      note,
	})
	if err != nil {
		return HandleResult{}, err
	}
	if !updated {
		// 并发下已被别人处置：重读当前状态并幂等返回（不覆盖先到者的痕迹）。
		current, findErr := s.hits.FindByID(ctx, hit.ID)
		if findErr != nil {
			return HandleResult{}, findErr
		}
		return HandleResult{Hit: current, AlreadyHandled: true}, nil
	}

	hit.Status = action.StatusFor()
	hit.HandlerID = in.HandlerID
	hit.HandledAt = handledAt
	hit.Note = note
	return HandleResult{Hit: hit}, nil
}

// BatchHandleInput 是批量处置的输入。
type BatchHandleInput struct {
	// IDs 是要处置的命中记录 ID。
	IDs []ctype.ID
	// Action / MuteDays / Note / HandlerID 与单条处置同一口径。
	Action    domain.Action
	MuteDays  int
	Note      string
	HandlerID ctype.ID
}

// BatchHandleItem 是批量处置里的一条结果。
type BatchHandleItem struct {
	ID            ctype.ID
	Hit           domain.Hit
	AlreadyHandled bool
	Err           error
}

// BatchHandleResult 是批量处置的结果：**逐条结果**而不是"全成功 / 全失败"。
type BatchHandleResult struct {
	Items []BatchHandleItem
}

// HandleBatch 逐条处置一批记录。
//
// 语义是"逐条结果"而不是"要么全成功"：一篇文章与一条评论分属不同模块的软删
// 事务，跨模块回滚做不到也不该做（删掉一半再回滚一半只会更难收拾）。所以：
//   - 请求级非法（动作错 / 备注超长 / 一条没选）在**动手之前**整体拒绝；
//   - 通过校验后逐条执行，每条结果各自如实返回 —— 成功的已生效，失败的
//     记录保持待处理，重试该条即可。前端按逐条结果提示"成功 N 条、失败 M 条"。
func (s *Service) HandleBatch(ctx context.Context, in BatchHandleInput) (BatchHandleResult, error) {
	action, err := normalizeAction(in.Action)
	if err != nil {
		return BatchHandleResult{}, err
	}
	note, err := domain.NormalizeNote(in.Note)
	if err != nil {
		return BatchHandleResult{}, err
	}
	if len(in.IDs) == 0 {
		return BatchHandleResult{}, apperr.New(apperr.CodeFilterBatchEmpty)
	}

	items := make([]BatchHandleItem, 0, len(in.IDs))
	for _, id := range in.IDs {
		got, handleErr := s.Handle(ctx, HandleInput{
			ID:        id,
			Action:    action,
			MuteDays:  in.MuteDays,
			Note:      note,
			HandlerID: in.HandlerID,
		})
		items = append(items, BatchHandleItem{
			ID:             id,
			Hit:            got.Hit,
			AlreadyHandled: got.AlreadyHandled,
			Err:            handleErr,
		})
	}
	return BatchHandleResult{Items: items}, nil
}

// WithDisposer 注入处置端口（软删内容 / 资料重置的实现）。
//
// 用可选项而不是构造参数：处置端口由装配层在内容模块装配完成后才能构造
// （它要拿 article / comment / user 的用例服务），而本服务在更早的
// InitContentFilter 里就已经建好了；顺序决定了只能"先建服务、后接线"。
func (s *Service) WithDisposer(disposer domain.Disposer) *Service {
	s.disposer = disposer
	return s
}

// WithAccountDisposer 注入账号处置端口（封禁 / 禁言，票 399）。
func (s *Service) WithAccountDisposer(disposer domain.AccountDisposer) *Service {
	s.accountDisposer = disposer
	return s
}

// disposeAccount 按动作分派账号处置（封禁 / 禁言）。
//
// 与 dispose 同一顺序口径：先动账号、再改记录状态（改状态放在 Handle 里）。
// 账号动失败时记录仍是待处理，管理员重试即可；反过来会留下"记录说已处置、
// 账号其实没被处罚"的错位。
func (s *Service) disposeAccount(ctx context.Context, hit domain.Hit, action domain.Action, actor ctype.ID, muteDays int) error {
	if s.accountDisposer == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	switch action {
	case domain.ActionBan:
		return s.accountDisposer.BanUser(ctx, actor, hit.UserID)
	case domain.ActionMute:
		return s.accountDisposer.MuteUser(ctx, actor, hit.UserID, muteDays)
	default:
		return apperr.New(apperr.CodeFilterActionInvalid)
	}
}

// dispose 按文本面分派删除动作。
//
// 私信面是唯一一个"命中但没有可删实体"的面：私信消息没有软删语义（也没有
// 后台删除入口），静默返回成功会让管理员以为消息没了 —— 必须明确报错。
// 昵称与简介共享同一份"资料重置"：spec 决策 7 把两者合并成一个处置动作
// （改昵称 + 清简介），所以哪个面命中都重置整份资料。
func (s *Service) dispose(ctx context.Context, hit domain.Hit, actor ctype.ID) error {
	if s.disposer == nil {
		return apperr.New(apperr.CodeFilterDisposeUnavailable)
	}
	switch hit.Surface {
	case domain.SurfaceArticle:
		return s.disposer.DeleteArticle(ctx, actor, hit.ContentID)
	case domain.SurfaceComment:
		return s.disposer.DeleteComment(ctx, actor, hit.ContentID)
	case domain.SurfaceCollection:
		return s.disposer.DeleteCollection(ctx, actor, hit.ContentID)
	case domain.SurfaceNickname, domain.SurfaceAbstract:
		return s.disposer.ResetProfile(ctx, actor, hit.UserID, domain.ProblemNickname(hit.UserID))
	case domain.SurfaceChat:
		return apperr.New(apperr.CodeFilterSurfaceNoContent)
	default:
		return apperr.New(apperr.CodeFilterSurfaceInvalid)
	}
}

// normalizeAction 校验动作取值。
func normalizeAction(raw domain.Action) (domain.Action, error) {
	if !raw.Valid() {
		return "", apperr.New(apperr.CodeFilterActionInvalid)
	}
	return raw, nil
}
