package app

import (
	"context"
	"encoding/json"
	"fmt"

	"myblogx/internal/kernel/ctype"
	notificationapp "myblogx/internal/notification/app"
	outboxrelay "myblogx/internal/platform/cdc/outboxrelay"
)

// forceDemoteNoticeSubscriber 投递「管理员强制退回草稿」的站内信（票 481）。
//
// # 为什么这条通知走发件箱
//
// 一开始它是同步的：用例提交事务之后直接发信，失败就把错误返给前端。这条链藏着一个
// 真实的漏通知 —— 管理员重试时文章已经是草稿，用例按"目的已达成"提前返回，
// 通知再也没机会发出，而作者看到的只是"文章突然不见了"。
//
// 走事件之后，投递失败由中继重试，与那次 HTTP 请求成不成功彻底无关。这正是发件箱
// 存在的理由：**通知是一次副作用，不该和业务事务的成败绑在一起**。
//
// # 幂等
//
// 中继是 at-least-once，同一条事件可能投递多次。去重按（接收者 + 链接 + 正文）判：
// 链接里带文章 ID、正文里带原因，所以这个键等价于「文章 ID + 原因」。消息表不能加
// 去重列（零 DDL）。
func forceDemoteNoticeSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		var payload forceDemotedPayload
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			// 事件体坏了就不可能拼出一条像话的通知：不重试（重试多少次体都是坏的），
			// 记一条日志让人能从死信之外的地方看到它。
			if infra.Logger != nil {
				infra.Logger.Errorf("强制退回通知：事件体解析失败: event=%s payload=%s err=%v", e.ID, e.Payload, err)
			}
			return nil
		}
		if payload.ArticleID.IsZero() || payload.AuthorID.IsZero() {
			return nil
		}
		if notificationService == nil || infra.DB == nil {
			return nil
		}

		content := fmt.Sprintf("您的文章《%s》已被管理员退回草稿：%s", payload.Title, payload.Reason)
		linkHref := fmt.Sprintf("/article/%s", payload.ArticleID)

		var exists int64
		if err := infra.DB.WithContext(ctx).Table(noticeMessageRow{}.TableName()).
			Where("receiver_id = ? AND link_herf = ? AND content = ?", payload.AuthorID, linkHref, content).
			Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}

		// action_user 记发起人（管理员）：作者看到的是"系统通知 + 是谁做的处置"。
		// 与"质量闸门退回"那条不同 —— 那次是系统动作，这里是一次人工干预。
		return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
			Receiver:   payload.AuthorID.Uint64(),
			ActionUser: payload.OperatorID.Uint64(),
			Content:    content,
			LinkTitle:  payload.Title,
			LinkHref:   linkHref,
		})
	}
}

// forceDemotedPayload 是 article.force_demoted 的事件体。
//
// 带全量事实而不是只带 article_id：通知要写的是"处置那一刻"的标题与原因，
// 事后回查库拿到的标题可能已经被作者改过。
type forceDemotedPayload struct {
	ArticleID  ctype.ID `json:"article_id,string"`
	AuthorID   ctype.ID `json:"author_id,string"`
	OperatorID ctype.ID `json:"operator_id,string"`
	Title      string   `json:"title"`
	Reason     string   `json:"reason"`
}
