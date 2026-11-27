package app

import (
	"context"
	"encoding/json"
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

// ForceDemote 管理员把文章强制退回草稿（运营纠错）。
//
// reason 必填：它会原样出现在作者的站内信里，也是这次处置唯一的审计依据；
// 空串按参数错误返回（apperr.CodeArticleForceDemoteReasonRequired），
// 而不是静默落一条空原因 —— 作者收到"你被退回了"却说不出为什么，比不退回更糟。
//
// 幂等语义：文章已经是草稿时返回 nil（"目的已达成"），不写库、不留档、不发通知。
// 客户端重试或管理员连点两次因此不会产生第二条站内信；并发重复请求由通知实现
// 的去重兜住（两笔事务可能都看到"已发布"，各自都会发一条事件）。
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

	// 通知走**发件箱事件**而不是事务外同步发送（票 481 的 code-review 修复）。
	//
	// 原写法有一个真实的漏通知：站内信在事务提交之后发，发失败就把 error 返给前端；
	// 管理员重试时文章已经是草稿，用例按"目的已达成"提前返回、根本不构造通知 ——
	// 作者永远收不到那条"为什么被退回"。改成事件之后，通知由中继投递、失败会重试，
	// 与"这次请求成功与否"彻底解耦；去重仍由通知实现按（接收者 + 链接 + 正文）做。
	var demoted *forceDemotedFacts

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

		demoted = &forceDemotedFacts{
			ArticleID:  a.ID(),
			AuthorID:   a.AuthorID(),
			Title:      a.Title(),
			Reason:     reason,
			OperatorID: operatorID,
		}
		// 与作者自己改回草稿共用同一条事件名：对下游来说语义相同（文章回到草稿、
		// 不再对读者露出），不需要再按"是谁撤的"分叉。
		if err := emit(ctx, r, EventArticleBackToDraft, a.ID()); err != nil {
			return err
		}
		return emitForceDemoted(ctx, r, *demoted)
	})
	if err != nil {
		return err
	}
	_ = demoted
	return nil
}

// forceDemotedFacts 是「强制退回」事件要带的全部事实。
//
// 订阅者只拿这些字段就能拼出一条站内信：文章 ID、标题、作者、原因、操作人。
// 不让订阅者回头查库是因为"事后查到的标题可能已经被作者改过" —— 通知要写的是
// 处置那一刻的事实。
type forceDemotedFacts struct {
	ArticleID  domain.ArticleID
	AuthorID   domain.UserID
	Title      string
	Reason     string
	OperatorID domain.UserID
}

// emitForceDemoted 发一条「管理员强制退回草稿」事件（票 481）。
func emitForceDemoted(ctx context.Context, r Repos, facts forceDemotedFacts) error {
	// ID 一律用字符串：雪花 ID 是 uint64，写成 JSON 数字会超出 JS 的安全整数范围，
	// 订阅者（或以后的任何消费者）一旦在 JS 里解析就会静默丢精度。
	payload, err := json.Marshal(map[string]any{
		"article_id":  facts.ArticleID.String(),
		"author_id":   facts.AuthorID.String(),
		"operator_id": facts.OperatorID.String(),
		"title":       facts.Title,
		"reason":      facts.Reason,
	})
	if err != nil {
		return err
	}
	return r.Outbox().Emit(ctx, Event{
		Name:          EventArticleForceDemoted,
		Payload:       payload,
		AggregateID:   facts.ArticleID,
		AggregateType: "article",
	})
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
