package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/article_review_enum"
)

// 管理员「强制退回草稿」是运营纠错工具，不是审核流程的一环。
//
// 线上真的会出现"文章已经对读者可见，但事后发现它本来就不该被放行"（2026-11-27
// 那篇 6 分文章，闸门与合规判定并发时漏放）。此时需要的动作是"立刻撤下来、通知
// 作者、留下一条能复盘是谁撤的、为什么撤的记录"，而不是等作者自己改稿。
// 所以这条用例的三件事都在这里：状态复位（domain）、作废旧任务 + 留档（本文件）、
// 站内信（事务外，通过端口交给装配层）。

// ForceDemoteNotice 是「文章被管理员强制退回草稿」这条站内信的内容。
//
// 由用例层填好再交给端口：装配层只负责"怎么发"，不该回头去库里拼标题与作者。
type ForceDemoteNotice struct {
	ArticleID  domain.ArticleID
	AuthorID   domain.UserID
	Title      string
	Reason     string
	OperatorID domain.UserID
}

// Notifier 是文章模块的站内信出口。
//
// 只声明当前真实需要的一条：app 层不认识通知模块（跨模块不 import），
// 形状由使用方定义、实现放在装配层（backend/app/article_wiring.go），
// 与 AuthorStatsPort / ActivityPort 同一做法。
type Notifier interface {
	// NotifyForceDemoted 通知作者"文章已被管理员退回草稿"，正文里带原因。
	//
	// 实现必须**幂等**：同一篇文章 + 同一条原因只发一条（并发重复请求也会走到这里）。
	NotifyForceDemoted(ctx context.Context, notice ForceDemoteNotice) error
}

// ForceDemote 管理员把文章强制退回草稿（运营纠错）。
//
// reason 必填：它会原样出现在作者的站内信里，也是这次处置唯一的审计依据；
// 空串按参数错误返回（apperr.CodeArticleForceDemoteReasonRequired），
// 而不是静默落一条空原因 —— 作者收到"你被退回了"却说不出为什么，比不退回更糟。
//
// 幂等语义：文章已经是草稿时返回 nil（"目的已达成"），不写库、不留档、不发通知。
// 客户端重试或管理员连点两次因此不会产生第二条站内信；并发重复请求由通知端口的
// 去重兜住（两笔事务可能都看到"已发布"，各自都发一次）。
func (s *Service) ForceDemote(
	ctx context.Context, articleID domain.ArticleID, operatorID domain.UserID, reason string,
) error {
	if articleID.IsZero() {
		return domain.ErrArticleNotFound
	}
	// 角色闸门在 HTTP 层（AdminMiddleware，本端点挂在管理员分组下）；这里只挡
	// "没有操作人"的调用：没有操作人就没有可写的 reviewed_by，等于没有留痕。
	if operatorID.IsZero() {
		return domain.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.ErrForceDemoteReasonRequired
	}
	// 按**字符**算而不是字节（理由同 app.reviewReason）：原因是中文，按字节算会
	// 把上限缩成"一百多个字"，而 reason 列 varchar(500) 是按字符的。
	if utf8.RuneCountInString(reason) > review.ReasonNoteMaxLen {
		return domain.ErrReviewReasonTooLong
	}

	var notice *ForceDemoteNotice

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		before := a.Status()
		if err := a.ForceDemote(now); err != nil {
			if errors.Is(err, domain.ErrInvalidTransition) {
				// 已经是草稿：目的已达成。什么都不写（不刷 updated_at、不留档、
				// 不发通知），这样"连点两次"在库里不留任何痕迹。
				return nil
			}
			return err
		}

		// 待处理任务一律作废：文章已经不在发布链路上了，留着只会让审核队列里
		// 出现一条"审了也没用"的任务。复用作者退回草稿时的同一个 helper，
		// 原因同样落进任务的 reason 列（那里是既有的审计通道，零 DDL）。
		if err := cancelPendingReviewTasks(ctx, r, a.ID(), forceDemoteReasonText(reason), now); err != nil {
			return err
		}
		// 留档：一条**创建即已撤销**的审核任务行（来源 force_demote）。
		//
		// 为什么不复用 createReviewTask 建一条待处理任务：那会把它塞进人工审核
		// 队列，逼管理员再去"处理"一次自己刚做的处置。这里要的是档案，不是待办。
		if err := recordForceDemoteTrace(ctx, s, r, a, before, operatorID, reason, now); err != nil {
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}

		notice = &ForceDemoteNotice{
			ArticleID:  a.ID(),
			AuthorID:   a.AuthorID(),
			Title:      a.Title(),
			Reason:     reason,
			OperatorID: operatorID,
		}
		// 与作者自己改回草稿共用同一条事件名：对下游来说语义相同（文章回到草稿、
		// 不再对读者露出），不需要再按"是谁撤的"分叉。
		return emit(ctx, r, EventArticleBackToDraft, a.ID())
	})
	if err != nil {
		return err
	}
	if notice == nil || s.notifier == nil {
		return nil
	}

	// 事务外发送：站内信不是这次处置的一部分，失败也不该让文章退回这个事实回滚。
	// 返回错误是为了让调用方知道"作者没收到通知"（重试不会再改状态，只会重发通知）。
	return s.notifier.NotifyForceDemoted(ctx, *notice)
}

// recordForceDemoteTrace 给一次强制退回留一条已撤销的审核任务行。
//
// 起点状态（before）记在 article_publish_status 上：复盘时要知道这次是"从已发布
// 撤下来的"还是"从审核中撤下来的"，两者的严重程度不同。reviewed_by 记执行人，
// reviewed_at 记时间，所以后台只看一张表就能回答"谁、什么时候、为什么"。
func recordForceDemoteTrace(
	ctx context.Context, s *Service, r Repos, a *domain.Article,
	before domain.Status, operatorID domain.UserID, reason string, now time.Time,
) error {
	// 昵称只是列表展示用的快照，取不到就留空——不要因为用户模块读失败让整次处置失败。
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
		PublishStatus: before,
		// 来源单独一个值：复盘要能一眼分出"这是管理员纠错"，而不是作者改稿。
		Source: review.Source(article_review_enum.SourceForceDemote),
		Now:    now,
	})
	// 先撤销再落库：这一行生来就是"已处理"，不经过 Save 再 Update 两步。
	if err := task.CancelBy(operatorID, forceDemoteReasonText(reason), now); err != nil {
		return err
	}
	return r.Reviews().Save(ctx, task)
}

// forceDemoteReasonText 是写进任务行 reason 列的处置说明。
//
// 带上前缀而不是只写管理员填的那句话：任务行的 reason 列里还有"作者改回草稿"
// "作者重新提交"这些既有撤销原因，前缀让"管理员纠错"在同一列里一眼可分。
// 发给作者的那句话只用管理员填的原文（不带这个内部前缀），见 ForceDemoteNotice。
func forceDemoteReasonText(reason string) string {
	return "管理员强制退回草稿：" + reason
}
