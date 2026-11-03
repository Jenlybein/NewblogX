// Package app 是私信模块的用例层。
//
// 分层口径（#201）：会话聚合 + 读写分离。
//
//   - 读：会话列表 / 消息列表 / 管理员视图，只走 domain.Query，无副作用；
//   - 写：发送 / 已读 / 删除，只走 domain.Command，事务内写聚合与发件箱事件；
//   - 通知：不再由用例层或 handler 直接改连接池，而是由发件箱中继的订阅者推送
//     （见 app/chat_wiring.go 与 app/outbox_wiring.go）。
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
	read      domain.Query
	write     domain.Command
	relations domain.RelationReader
	users     domain.UserDirectory
	stranger  domain.StrangerPolicy
	limiter   domain.Limiter
	logger    domain.Logger
	now       func() time.Time
}

// New 构造服务。
//
// read 与 write 是两个端口而不是一个仓储：读路径不持有事务，写路径才有事务与
// 发件箱副作用，用例层从构造上就分得清"这个方法会不会改状态"。
func New(read domain.Query, write domain.Command, relations domain.RelationReader, users domain.UserDirectory, stranger domain.StrangerPolicy, limiter domain.Limiter, logger domain.Logger) *Service {
	if read == nil {
		panic("chat/app: 会话读端口未注入")
	}
	if write == nil {
		panic("chat/app: 会话写端口未注入")
	}
	if users == nil {
		panic("chat/app: 用户展示读未注入")
	}
	return &Service{
		read: read, write: write, relations: relations, users: users, stranger: stranger,
		limiter: limiter, logger: logger, now: time.Now,
	}
}

// Send 发送一条消息。
//
// 顺序：校验接收人 → 关系权限 → 限流预占 → 落库（HTTP 与 WS 都走这里，
// 落库是唯一真相）→ 提交预占；落库失败必须回滚预占。
//
// clientMsgID 非空时启用幂等：由（发送人，幂等键）派生主键，重复提交撞主键后
// 直接返回第一次落库的消息（同时回滚本次的额度预占），不会产生第二条消息、
// 也不会把接收方未读再加一次。
func (s *Service) Send(ctx context.Context, senderID, receiverID ctype.ID, msgType domain.MsgType, content, clientMsgID string) (*domain.Message, error) {
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
	if senderID.IsZero() || receiverID.IsZero() {
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
	// 陌生人判定只对"确实没有关系"的双方做：自己、已有关注关系、
	// 或已存在会话的双方都不参与（#243）。
	stranger := senderID != receiverID && relation == 1
	if stranger {
		_, err := s.read.FindSession(ctx, domain.BuildSessionID(senderID, receiverID), senderID, true)
		switch {
		case err == nil:
			// 已经聊过：不再是陌生人，个人开关不该半路切断既有会话。
			stranger = false
		case apperr.IsCode(err, apperr.CodeChatSessionNotFound):
			// 没有会话：继续按陌生人判定。
		default:
			return nil, err
		}
	}
	// 陌生人私信是**两级**策略（#243）：站点总闸 AND 接收方个人开关。
	// 两级取值在装配层的 StrangerPolicy 里合成，用例层只消费结论；
	// 必须传接收方 ID，个人开关才有接线点。
	//
	// #247 起结论带原因：两种拒绝在读者侧对应不同的正确动作 ——
	// 站点级找管理员，接收方级提示"对方不接受陌生人私信"。
	if stranger {
		verdict, err := s.stranger.CheckStranger(ctx, receiverID)
		if err != nil {
			return nil, err
		}
		switch verdict {
		case domain.StrangerAllowed:
		case domain.StrangerBlockedByRecipient:
			return nil, apperr.New(apperr.CodeChatRecipientStrangerOff)
		default:
			// 站点级拒绝（以及未知结论的失败关闭）沿用既有错误码。
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

	msg, duplicated, err := s.write.Send(ctx, domain.SendCommand{
		SenderID:    senderID,
		ReceiverID:  receiverID,
		MsgType:     msgType,
		Content:     content,
		ClientMsgID: strings.TrimSpace(clientMsgID),
		SendTime:    s.now(),
	}, sender, receiver, domain.Summary(msgType, content))
	if err != nil {
		if reservation != nil {
			_ = reservation.Rollback()
		}
		return nil, err
	}

	// 幂等命中：这一次没有真正写入，额度必须释放，否则重试会把配额白白吃掉。
	if duplicated {
		if reservation != nil {
			_ = reservation.Rollback()
		}
		return &msg, nil
	}
	if reservation != nil {
		if err := reservation.Commit(); err != nil && s.logger != nil {
			s.logger.Warnf("提交聊天发送额度失败: sender=%d receiver=%d err=%v", senderID, receiverID, err)
		}
	}
	return &msg, nil
}

// Sessions 会话列表。
func (s *Service) Sessions(ctx context.Context, viewerID, targetID ctype.ID, admin bool, page domain.Page) ([]domain.SessionItem, int, error) {
	if targetID.IsZero() {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return nil, 0, apperr.New(apperr.CodeChatNoPermission)
	}

	rows, total, err := s.read.ListSessions(ctx, targetID, admin, page)
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
	if targetID.IsZero() {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return nil, 0, apperr.New(apperr.CodeChatNoPermission)
	}
	session, err := s.read.FindSession(ctx, sessionID, targetID, admin)
	if err != nil {
		return nil, 0, err
	}
	msgs, total, err := s.read.ListMessages(ctx, domain.MessageQuery{
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
		if got, err := s.read.UserDeletedAt(ctx, targetID, sessionID, ids); err == nil {
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
	return s.write.MarkRead(ctx, userID, ids)
}

// DeleteMessages 用户级删除消息。
func (s *Service) DeleteMessages(ctx context.Context, userID ctype.ID, ids []ctype.ID) (int, error) {
	if len(ids) == 0 {
		return 0, apperr.New(apperr.CodeChatEmptySelection)
	}
	msgs, err := s.write.DeleteMessagesForUser(ctx, userID, ids)
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
	deleted, err := s.write.DeleteSessions(ctx, userID, sessionIDs)
	if err != nil {
		return 0, err
	}
	return len(deleted), nil
}

func (s *Service) relationBatch(ctx context.Context, viewerID ctype.ID, ids []ctype.ID) map[ctype.ID]int8 {
	out := make(map[ctype.ID]int8, len(ids))
	if viewerID.IsZero() || s.relations == nil {
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
