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
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 私信模块的九个协作者只从这一个构造点进入用例层（#280）：
//   - 结构必需：read / write / users —— 缺了用例层无法成立，构造时直接失败；
//   - 发送资格：blocks / relations / stranger / limiter / imageCheck —— 允许
//     为零值，但发送时一律失败关闭（见 sendGuard）。这样装配遗漏不会静默
//     放行，而是变成一个可观测的拒绝。
//
// 判定所需的五个协作者在 Service 内部收进 sendGuard：它们是模块内部接缝，
// 不再出现在每个用例方法的视线里。具体实现由合成根构造后注入；用例层不
// import 其它模块。
type Service struct {
	read   domain.Query
	write  domain.Command
	users  domain.UserDirectory
	logger domain.Logger
	now    func() time.Time
	guard  sendGuard
}

// New 构造服务。
//
// read 与 write 是两个端口而不是一个仓储：读路径不持有事务，写路径才有事务与
// 发件箱副作用，用例层从构造上就分得清"这个方法会不会改状态"。
//
// 发送资格的五项依赖不在这里 panic：它们缺失时 Send 会失败关闭（#280），
// 用"构造失败"表达同一件事会掩盖"哪个依赖没上场"。
func New(read domain.Query, write domain.Command, relations domain.RelationReader, users domain.UserDirectory, stranger domain.StrangerPolicy, limiter domain.Limiter, imageCheck domain.ImageURLVerifier, blocks domain.BlockPolicy, logger domain.Logger) *Service {
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
		read:   read,
		write:  write,
		users:  users,
		logger: logger,
		now:    time.Now,
		guard: sendGuard{
			read:       read,
			blocks:     blocks,
			relations:  relations,
			users:      users,
			stranger:   stranger,
			limiter:    limiter,
			imageCheck: imageCheck,
		},
	}
}

// Send 发送一条消息。
//
// 顺序：一次资格判定（图片来源 → 屏蔽 / 拉黑 → 关系与陌生人 → 按对限流）
// → 落库（HTTP 与 WS 都走这里，落库是唯一真相）→ 提交预占；落库失败必须回滚预占。
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
	if senderID.IsZero() || receiverID.IsZero() {
		return nil, apperr.New(apperr.CodeChatUserNotFound)
	}

	plan, err := s.guard.check(ctx, senderID, receiverID, msgType, content)
	if err != nil {
		return nil, err
	}

	msg, duplicated, err := s.write.Send(ctx, domain.SendCommand{
		SenderID:    senderID,
		ReceiverID:  receiverID,
		MsgType:     msgType,
		Content:     plan.content,
		ClientMsgID: strings.TrimSpace(clientMsgID),
		SendTime:    s.now(),
	}, plan.sender, plan.receiver, domain.Summary(msgType, plan.content))
	if err != nil {
		plan.rollback()
		return nil, err
	}

	// 幂等命中：这一次没有真正写入，额度必须释放，否则重试会把配额白白吃掉。
	if duplicated {
		plan.rollback()
		return &msg, nil
	}
	plan.commit(s.logger)
	return &msg, nil
}

// Sessions 会话列表。
//
// 拉黑（level=2）的过滤在**查库层**完成（domain.SessionQuery.HideBlocked），
// 因此 total 就是过滤后的真值：不再"先分页、再按全部拉黑目标数扣总数"。
// 屏蔽（level=1）不影响可见性 —— 历史会话仍要看得到（ADR-0026）。
//
// 管理员视图不过滤拉黑是**有意例外**：运维排查需要看全量会话，
// 这条规则与读者视图不同，不是漏判（#280 AC 1 / #272 User Story 15）。
func (s *Service) Sessions(ctx context.Context, viewerID, targetID ctype.ID, admin bool, page domain.Page) ([]domain.SessionItem, int, error) {
	if targetID.IsZero() {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return nil, 0, apperr.New(apperr.CodeChatNoPermission)
	}

	rows, total, err := s.read.ListSessions(ctx, domain.SessionQuery{
		UserID: targetID,
		// 管理员：连软删会话一起看，且不过滤拉黑（有意例外）。
		Unscoped:    admin,
		HideBlocked: !admin,
		Page:        page,
	})
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

// Messages 消息列表：按 (send_time, id) 倒序，带复合游标与 has_more。
//
// 权限校验、清空水位、IsSelf/IsRead/DeletedAt 富化都保持原样；变化只在读端口
// 现在一次返回"一页 + 是否还有更早 + 下一页游标"，handler 不再自己拼装分页。
func (s *Service) Messages(ctx context.Context, viewerID, targetID ctype.ID, sessionID string, admin bool, page domain.Page, cursor domain.MessageCursor) (domain.MessagePage, error) {
	if targetID.IsZero() {
		targetID = viewerID
	}
	if targetID != viewerID && !admin {
		return domain.MessagePage{}, apperr.New(apperr.CodeChatNoPermission)
	}
	session, err := s.read.FindSession(ctx, sessionID, targetID, admin)
	if err != nil {
		return domain.MessagePage{}, err
	}
	got, err := s.read.ListMessages(ctx, domain.MessageQuery{
		UserID:      targetID,
		SessionID:   sessionID,
		ClearBefore: session.ClearBeforeMsgID,
		Unscoped:    admin,
		Page:        page,
		Cursor:      cursor,
	})
	if err != nil {
		return domain.MessagePage{}, err
	}
	msgs := got.Messages

	var deleted map[ctype.ID]time.Time
	if admin {
		ids := make([]ctype.ID, 0, len(msgs))
		for _, msg := range msgs {
			ids = append(ids, msg.ID)
		}
		if gotDeleted, err := s.read.UserDeletedAt(ctx, targetID, sessionID, ids); err == nil {
			deleted = gotDeleted
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
	return domain.MessagePage{
		Items:      items,
		Total:      got.Total,
		HasMore:    got.HasMore,
		NextCursor: got.NextCursor,
	}, nil
}

// MarkRead 批量标记已读；返回本次命中的消息（用于 WS 回执）。
func (s *Service) MarkRead(ctx context.Context, userID ctype.ID, ids []ctype.ID) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, apperr.New(apperr.CodeChatEmptySelection)
	}
	return s.write.MarkRead(ctx, userID, ids)
}

// MarkReadSession 把整条会话里发给自己的未读消息标记已读。
func (s *Service) MarkReadSession(ctx context.Context, userID ctype.ID, sessionID string) ([]domain.Message, error) {
	return s.write.MarkReadSession(ctx, userID, sessionID)
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

// relationBatch 补齐会话列表里的关系级别。
//
// 只影响列表展示（relation 列），不参与发送资格判定，所以关系端口缺失或查询
// 出错时退化成"陌生人（1）"是保守显示，不放行任何写操作；发送资格在
// sendGuard 里失败关闭，与这里不同。
func (s *Service) relationBatch(ctx context.Context, viewerID ctype.ID, ids []ctype.ID) map[ctype.ID]int8 {
	out := make(map[ctype.ID]int8, len(ids))
	if viewerID.IsZero() || s.guard.relations == nil {
		for _, id := range ids {
			out[id] = 1
		}
		return out
	}
	got, err := s.guard.relations.Batch(ctx, viewerID, ids)
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
