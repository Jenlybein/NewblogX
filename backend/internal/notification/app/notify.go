package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/notification/domain"
)

// ── 通知生产侧 ──────────────────────────────────────────────────
//
// 这些方法由其他模块（评论、outbox 中继）调用，负责把"业务事件"翻译成站内信。
// 与旧实现的差异：错误不再吞掉只记日志，而是返回给调用方，
// 让 outbox 中继能据此重试；去重逻辑原样保留，所以重试不会产生重复消息。

// CommentNotice 是"文章收到评论"的入参。
type CommentNotice struct {
	CommentID    ctype.ID
	Content      string
	Receiver     uint64
	ActionUser   uint64
	ArticleID    ctype.ID
	ArticleTitle string
}

// ReplyNotice 是"评论收到回复"的入参。
type ReplyNotice struct {
	CommentID    ctype.ID
	Content      string
	Receiver     uint64
	ActionUser   uint64
	ArticleID    ctype.ID
	ArticleTitle string
}

// CommentDiggNotice 是"评论被点赞"的入参。
type CommentDiggNotice struct {
	CommentID    ctype.ID
	Content      string
	Receiver     uint64
	ActionUser   uint64
	ArticleID    ctype.ID
	ArticleTitle string
}

// InteractionNotice 是"文章被点赞/收藏"的入参。
type InteractionNotice struct {
	Receiver     uint64
	ActionUser   uint64
	ArticleID    ctype.ID
	ArticleTitle string
}

// SystemNotice 是系统通知的入参。
type SystemNotice struct {
	Receiver   uint64
	ActionUser uint64 // 0 表示没有操作人
	Content    string
	LinkTitle  string
	LinkHref   string
}

// NotifyComment 通知文章作者"有人评论了你的文章"。
func (s *Service) NotifyComment(ctx context.Context, in CommentNotice) error {
	return s.deliver(ctx, domain.Message{
		Type:         domain.TypeCommentArticle,
		Receiver:     in.Receiver,
		ActionUser:   in.ActionUser,
		Content:      in.Content,
		CommentID:    in.CommentID,
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	}, false, 0)
}

// NotifyReply 通知被回复的人。
func (s *Service) NotifyReply(ctx context.Context, in ReplyNotice) error {
	return s.deliver(ctx, domain.Message{
		Type:         domain.TypeCommentReply,
		Receiver:     in.Receiver,
		ActionUser:   in.ActionUser,
		Content:      in.Content,
		CommentID:    in.CommentID,
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	}, false, 0)
}

// NotifyCommentDigg 通知评论作者"有人赞了你的评论"。
func (s *Service) NotifyCommentDigg(ctx context.Context, in CommentDiggNotice) error {
	return s.deliver(ctx, domain.Message{
		Type:         domain.TypeDiggComment,
		Receiver:     in.Receiver,
		ActionUser:   in.ActionUser,
		Content:      in.Content,
		CommentID:    in.CommentID,
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	}, true, in.CommentID.Uint64())
}

// NotifyArticleDigg 通知作者"有人赞了你的文章"。
func (s *Service) NotifyArticleDigg(ctx context.Context, in InteractionNotice) error {
	return s.deliver(ctx, domain.Message{
		Type:         domain.TypeDiggArticle,
		Receiver:     in.Receiver,
		ActionUser:   in.ActionUser,
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	}, true, 0)
}

// NotifyArticleFavor 通知作者"有人收藏了你的文章"。
func (s *Service) NotifyArticleFavor(ctx context.Context, in InteractionNotice) error {
	return s.deliver(ctx, domain.Message{
		Type:         domain.TypeFavorArticle,
		Receiver:     in.Receiver,
		ActionUser:   in.ActionUser,
		ArticleID:    in.ArticleID,
		ArticleTitle: in.ArticleTitle,
	}, true, 0)
}

// NotifySystem 发一条系统通知。
func (s *Service) NotifySystem(ctx context.Context, in SystemNotice) error {
	if in.Receiver == 0 {
		return apperr.New(apperr.CodeMessageNotFound)
	}
	return s.deliver(ctx, domain.Message{
		Type:       domain.TypeSystem,
		Receiver:   in.Receiver,
		ActionUser: in.ActionUser,
		Content:    in.Content,
		LinkTitle:  in.LinkTitle,
		LinkHref:   in.LinkHref,
	}, false, 0)
}

// deliver 统一处理：补操作人快照、去重、落库。
//
// dedupe=true 时按 (操作人, 类型, 文章/评论) 判重——旧实现的语义是
// "同一个人对同一个对象点过赞/收过藏就不再通知"，避免点赞-取消-再点赞刷屏。
func (s *Service) deliver(ctx context.Context, msg domain.Message, dedupe bool, commentID uint64) error {
	if msg.Receiver == 0 {
		return nil
	}
	if dedupe && msg.ActionUser != 0 {
		exists, err := s.messages.ExistsDigg(ctx, msg.ActionUser, msg.Type, msg.ArticleID.Uint64(), commentID)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	if msg.ActionUser != 0 && s.users != nil {
		nickname, avatar, err := s.users.Profile(ctx, msg.ActionUser)
		if err != nil {
			// 快照失败不该让通知整体失败：消息本身仍然有价值。
			s.logError("读取通知操作人信息失败: user=%d err=%v", msg.ActionUser, err)
		} else {
			msg.ActionUserNickname = nickname
			msg.ActionUserAvatar = avatar
		}
	}
	return s.messages.Create(ctx, &msg)
}

// logError 写一条错误日志。
func (s *Service) logError(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Errorf(format, args...)
}
