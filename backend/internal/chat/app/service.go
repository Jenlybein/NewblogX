// Package app 是私信模块的用例层。
package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// Service 是私信应用服务。
type Service struct {
	repo      domain.Repository
	relations domain.RelationReader
	users     domain.UserDirectory
	stranger  domain.StrangerPolicy
	limiter   domain.Limiter
	hub       domain.Hub
	logger    domain.Logger
	now       func() time.Time
}

// New 构造服务。
func New(repo domain.Repository, relations domain.RelationReader, users domain.UserDirectory, stranger domain.StrangerPolicy, limiter domain.Limiter, hub domain.Hub, logger domain.Logger) *Service {
	if repo == nil {
		panic("chat/app: 会话仓储未注入")
	}
	if users == nil {
		panic("chat/app: 用户展示读未注入")
	}
	return &Service{
		repo: repo, relations: relations, users: users, stranger: stranger,
		limiter: limiter, hub: hub, logger: logger, now: time.Now,
	}
}

// Send 发送一条消息。
//
// 顺序：校验接收人 → 关系权限 → 限流预占 → 落库 → 提交预占；落库失败必须回滚预占。
func (s *Service) Send(ctx context.Context, senderID, receiverID ctype.ID, msgType domain.MsgType, content string) (*domain.Message, error) {
	switch msgType {
	case domain.TypeText, domain.TypeImage, domain.TypeMarkdown:
	default:
		return nil, apperr.New(apperr.CodeChatTypeUnsupported)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, apperr.New(apperr.CodeChatContentEmpty)
	}
	// 图片消息沿用旧契约：Content 存 JSON，图片地址放在 image_url。
	if msgType == domain.TypeImage {
		payload, err := json.Marshal(map[string]string{"kind": "image", "image_url": content})
		if err != nil {
			return nil, err
		}
		content = string(payload)
	}
	if senderID == 0 || receiverID == 0 {
		return nil, apperr.New(apperr.CodeChatUserNotFound)
	}

	displays, err := s.users.Displays(ctx, []ctype.ID{senderID, receiverID})
	if err != nil {
		return nil, err
	}
	receiver, ok := displays[receiverID]
	if !ok {
		return nil, apperr.New(apperr.CodeChatUserNotFound)
	}
	sender := displays[senderID]

	relation := int8(1)
	if senderID != receiverID && s.relations != nil {
		got, err := s.relations.Batch(ctx, senderID, []ctype.ID{receiverID})
		if err != nil {
			return nil, err
		}
		if rel, ok := got[receiverID]; ok {
			relation = rel
		}
	}
	// 陌生人需要对方开启陌生私信。
	if senderID != receiverID && relation == 1 {
		enabled, err := s.stranger.StrangerEnabled(ctx, receiverID)
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, apperr.New(apperr.CodeChatStrangerOff)
		}
	}

	var reservation domain.Reservation
	if s.limiter != nil {
		reservation, err = s.limiter.Reserve(ctx, senderID, receiverID, relation)
		if err != nil {
			return nil, err
		}
	}

	msg := &domain.Message{
		SenderID:   senderID,
		ReceiverID: receiverID,
		SessionID:  domain.BuildSessionID(senderID, receiverID),
		Content:    content,
		SendTime:   s.now(),
		MsgStatus:  domain.StatusSent,
		MsgType:    msgType,
	}
	if err := s.repo.SendMessage(ctx, msg, sender, receiver, domain.Summary(msgType, content)); err != nil {
		if reservation != nil {
			_ = reservation.Rollback()
		}
		return nil, err
	}
	if reservation != nil {
		if err := reservation.Commit(); err != nil && s.logger != nil {
			s.logger.Warnf("提交聊天发送额度失败: sender=%d receiver=%d err=%v", senderID, receiverID, err)
		}
	}
	return msg, nil
}

// Sessions 会话列表。
func (s *Service) Sessions(ctx context.Context, viewerID, targetID ctype.ID, admin bool, page domain.Page) ([]domain.SessionItem, int, error) {
	if targetID == 0 {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return nil, 0, apperr.New(apperr.CodeChatNoPermission)
	}

	rows, total, err := s.repo.ListSessions(ctx, targetID, admin, page)
	if err != nil {
		return nil, 0, err
	}

	// 补齐接收人快照。
	need := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.ReceiverNickname == "" || row.ReceiverAvatar == "" {
			need = append(need, row.ReceiverID)
		}
	}
	displays := map[ctype.ID]domain.Display{}
	if len(need) > 0 {
		if got, err := s.users.Displays(ctx, need); err == nil {
			displays = got
		}
	}
	receiverIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		receiverIDs = append(receiverIDs, row.ReceiverID)
	}
	relations := s.relationBatch(ctx, viewerID, receiverIDs)

	items := make([]domain.SessionItem, 0, len(rows))
	for _, row := range rows {
		nickname := row.ReceiverNickname
		avatar := row.ReceiverAvatar
		if user, ok := displays[row.ReceiverID]; ok {
			if nickname == "" {
				nickname = user.Nickname
			}
			if avatar == "" {
				avatar = user.Avatar
			}
		}
		items = append(items, domain.SessionItem{
			SessionID:        row.SessionID,
			ReceiverID:       row.ReceiverID,
			ReceiverNickname: nickname,
			ReceiverAvatar:   avatar,
			Relation:         relations[row.ReceiverID],
			LastMsgContent:   row.LastMsgContent,
			LastMsgTime:      row.LastMsgTime,
			UnreadCount:      row.UnreadCount,
			IsTop:            row.IsTop,
			IsMute:           row.IsMute,
			DeletedAt:        row.DeletedAt,
		})
	}
	return items, total, nil
}

// Messages 消息列表。
func (s *Service) Messages(ctx context.Context, viewerID, targetID ctype.ID, sessionID string, admin bool, page domain.Page) ([]domain.MessageItem, int, error) {
	if targetID == 0 {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return nil, 0, apperr.New(apperr.CodeChatNoPermission)
	}
	session, err := s.repo.FindSession(ctx, sessionID, targetID, admin)
	if err != nil {
		return nil, 0, err
	}
	msgs, total, err := s.repo.ListMessages(ctx, domain.MessageQuery{
		UserID:      targetID,
		SessionID:   sessionID,
		ClearBefore: session.ClearBeforeMsgID,
		Unscoped:    admin,
		Page:        page,
	})
	if err != nil {
		return nil, 0, err
	}

	var deleted map[ctype.ID]time.Time
	if admin {
		ids := make([]ctype.ID, 0, len(msgs))
		for _, msg := range msgs {
			ids = append(ids, msg.ID)
		}
		if got, err := s.repo.UserDeletedAt(ctx, targetID, sessionID, ids); err == nil {
			deleted = got
		}
	}

	items := make([]domain.MessageItem, 0, len(msgs))
	for _, msg := range msgs {
		item := domain.MessageItem{
			ID:         msg.ID,
			SenderID:   msg.SenderID,
			ReceiverID: msg.ReceiverID,
			SessionID:  msg.SessionID,
			Content:    msg.Content,
			SendTime:   msg.SendTime,
			MsgStatus:  msg.MsgStatus,
			MsgType:    msg.MsgType,
			IsSelf:     msg.SenderID == targetID,
			IsRead:     msg.MsgStatus.IsRead(),
		}
		if at, ok := deleted[msg.ID]; ok {
			value := at
			item.DeletedAt = &value
		}
		items = append(items, item)
	}
	return items, total, nil
}

// MarkRead 批量标记已读；返回本次命中的消息（用于 WS 回执）。
func (s *Service) MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, apperr.New(apperr.CodeChatEmptySelection)
	}
	return s.repo.MarkRead(ctx, userID, ids)
}

// DeleteMessages 用户级删除消息。
func (s *Service) DeleteMessages(ctx context.Context, userID ctype.ID, ids []ctype.ID) (int, error) {
	if len(ids) == 0 {
		return 0, apperr.New(apperr.CodeChatEmptySelection)
	}
	msgs, err := s.repo.DeleteMessagesForUser(ctx, userID, ids)
	if err != nil {
		return 0, err
	}
	return len(msgs), nil
}

// DeleteSessions 删除会话（推进清空水位）。
func (s *Service) DeleteSessions(ctx context.Context, userID ctype.ID, sessionIDs []string) (int, error) {
	if len(sessionIDs) == 0 {
		return 0, apperr.New(apperr.CodeChatEmptySelection)
	}
	deleted, err := s.repo.DeleteSessions(ctx, userID, sessionIDs)
	if err != nil {
		return 0, err
	}
	return len(deleted), nil
}

// Hub 返回在线连接池（WS 层用）。
func (s *Service) Hub() domain.Hub { return s.hub }

func (s *Service) relationBatch(ctx context.Context, viewerID ctype.ID, ids []ctype.ID) map[ctype.ID]int8 {
	out := make(map[ctype.ID]int8, len(ids))
	if viewerID == 0 || s.relations == nil {
		for _, id := range ids {
			out[id] = 1
		}
		return out
	}
	got, err := s.relations.Batch(ctx, viewerID, ids)
	if err != nil {
		for _, id := range ids {
			out[id] = 1
		}
		return out
	}
	for id, rel := range got {
		out[id] = rel
	}
	return out
}
