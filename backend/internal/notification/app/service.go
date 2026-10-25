// Package app 是通知模块的用例层。
package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/domain"
)

// Clock 抽象当前时间，便于用例层测试。
type Clock interface {
	Now() time.Time
}

// Logger 是窄日志接口。
type Logger interface {
	Errorf(format string, args ...any)
}

// Service 是通知服务。
type Service struct {
	messages domain.MessageRepository
	globals  domain.GlobalRepository
	confs    domain.ConfRepository
	users    domain.UserDirectory
	chat     domain.ChatUnreadCounter
	clock    Clock
	logger   Logger
}

// New 构造通知服务。
//
// users / chat 可以为 nil：单元测试里只关心消息本身时不必准备它们，
// 缺失时公告相关能力会退化成"所有用户可见 + 私信未读为 0"。
func New(
	messages domain.MessageRepository,
	globals domain.GlobalRepository,
	confs domain.ConfRepository,
	users domain.UserDirectory,
	chat domain.ChatUnreadCounter,
	clock Clock,
	logger Logger,
) *Service {
	if clock == nil {
		clock = realClock{}
	}
	return &Service{
		messages: messages,
		globals:  globals,
		confs:    confs,
		users:    users,
		chat:     chat,
		clock:    clock,
		logger:   logger,
	}
}

// realClock 是默认时钟。
type realClock struct{}

// Now 实现 Clock。
func (realClock) Now() time.Time { return time.Now() }

// ── 站内信读侧 ──────────────────────────────────────────────────

// ListMessages 按分组分页查站内信。
func (s *Service) ListMessages(ctx context.Context, receiver uint64, group domain.Group, page domain.Page) ([]domain.Message, bool, error) {
	if receiver == 0 {
		return nil, false, apperr.New(apperr.CodeAuthRequired)
	}
	types := group.Types()
	if len(types) == 0 {
		return nil, false, apperr.New(apperr.CodeMessageNotFound)
	}
	return s.messages.List(ctx, receiver, types, page)
}

// MarkMessageRead 把一条站内信标记为已读。
func (s *Service) MarkMessageRead(ctx context.Context, receiver, id uint64) error {
	if receiver == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if id == 0 {
		return apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	msg, err := s.messages.Find(ctx, receiver, id)
	if err != nil {
		return err
	}
	if msg.IsRead {
		return apperr.New(apperr.CodeMessageAlreadyRead)
	}
	return s.messages.MarkRead(ctx, receiver, id)
}

// MarkGroupRead 把某分组下全部未读标记为已读，返回条数。
func (s *Service) MarkGroupRead(ctx context.Context, receiver uint64, group domain.Group) (int64, error) {
	if receiver == 0 {
		return 0, apperr.New(apperr.CodeAuthRequired)
	}
	types := group.Types()
	if len(types) == 0 {
		return 0, apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	return s.messages.MarkGroupRead(ctx, receiver, types)
}

// DeleteMessage 删除一条站内信。
func (s *Service) DeleteMessage(ctx context.Context, receiver, id uint64) error {
	if receiver == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if id == 0 {
		return apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	if _, err := s.messages.Find(ctx, receiver, id); err != nil {
		return err
	}
	return s.messages.Delete(ctx, receiver, id)
}

// DeleteGroup 删除某分组下全部站内信，返回条数。
func (s *Service) DeleteGroup(ctx context.Context, receiver uint64, group domain.Group) (int64, error) {
	if receiver == 0 {
		return 0, apperr.New(apperr.CodeAuthRequired)
	}
	types := group.Types()
	if len(types) == 0 {
		return 0, apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	return s.messages.DeleteGroup(ctx, receiver, types)
}

// Counts 汇总各入口的未读数。
type Counts struct {
	Comment   int
	DiggFavor int
	System    int
	Private   int
	Global    int
}

// UnreadCounts 返回消息中心的角标数字。
func (s *Service) UnreadCounts(ctx context.Context, userID uint64) (Counts, error) {
	if userID == 0 {
		return Counts{}, apperr.New(apperr.CodeAuthRequired)
	}
	raw, err := s.messages.CountUnread(ctx, userID)
	if err != nil {
		return Counts{}, err
	}
	out := Counts{}
	for typ, n := range raw {
		switch typ {
		case domain.TypeCommentArticle, domain.TypeCommentReply:
			out.Comment += int(n)
		case domain.TypeDiggArticle, domain.TypeDiggComment, domain.TypeFavorArticle:
			out.DiggFavor += int(n)
		case domain.TypeSystem:
			out.System += int(n)
		}
	}
	if s.chat != nil {
		private, err := s.chat.UnreadTotal(ctx, userID)
		if err != nil {
			return Counts{}, err
		}
		out.Private = int(private)
	}
	createdAt, err := s.userCreatedAt(ctx, userID)
	if err != nil {
		return Counts{}, err
	}
	global, err := s.globals.CountUnread(ctx, userID, createdAt)
	if err != nil {
		return Counts{}, err
	}
	out.Global = int(global)
	return out, nil
}

// GetConf 读用户通知开关。
func (s *Service) GetConf(ctx context.Context, userID uint64) (domain.Conf, error) {
	if userID == 0 {
		return domain.Conf{}, apperr.New(apperr.CodeAuthRequired)
	}
	return s.confs.Find(ctx, userID)
}

// UpdateConf 覆盖用户通知开关。
func (s *Service) UpdateConf(ctx context.Context, userID uint64, conf domain.Conf) error {
	if userID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	return s.confs.Replace(ctx, userID, conf)
}

// ── 全局公告 ────────────────────────────────────────────────────

// GlobalItem 是列表里的一条公告（带该用户的已读状态）。
type GlobalItem struct {
	domain.GlobalNotif
	IsRead bool
}

// ListGlobal 分页查全局公告。
//
// admin=true 时走管理员视角（全部公告）；否则只返回该用户可见的。
func (s *Service) ListGlobal(ctx context.Context, userID uint64, admin bool, page domain.Page) ([]GlobalItem, bool, error) {
	var (
		rows    []domain.GlobalNotif
		hasMore bool
		err     error
	)
	if admin {
		rows, hasMore, err = s.globals.ListAll(ctx, page)
	} else {
		if userID == 0 {
			return nil, false, apperr.New(apperr.CodeAuthRequired)
		}
		createdAt, cerr := s.userCreatedAt(ctx, userID)
		if cerr != nil {
			return nil, false, cerr
		}
		rows, hasMore, err = s.globals.ListVisible(ctx, userID, createdAt, page)
	}
	if err != nil {
		return nil, false, err
	}
	items := make([]GlobalItem, 0, len(rows))
	if len(rows) == 0 {
		return items, hasMore, nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	readSet, err := s.globals.ReadSet(ctx, userID, ids)
	if err != nil {
		return nil, false, err
	}
	for _, row := range rows {
		items = append(items, GlobalItem{GlobalNotif: row, IsRead: readSet[row.ID]})
	}
	return items, hasMore, nil
}

// MarkGlobalRead 批量标记公告已读，返回条数。
func (s *Service) MarkGlobalRead(ctx context.Context, userID uint64, ids []uint64) (int, error) {
	if userID == 0 {
		return 0, apperr.New(apperr.CodeAuthRequired)
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	createdAt, err := s.userCreatedAt(ctx, userID)
	if err != nil {
		return 0, err
	}
	visible, err := s.globals.FindVisible(ctx, userID, ids, createdAt)
	if err != nil {
		return 0, err
	}
	if len(visible) == 0 {
		return 0, apperr.New(apperr.CodeNotifNotFound)
	}
	visibleIDs := make([]uint64, 0, len(visible))
	for _, n := range visible {
		visibleIDs = append(visibleIDs, n.ID)
	}
	count, err := s.globals.MarkRead(ctx, userID, visibleIDs)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, apperr.New(apperr.CodeMessageNothingRead)
	}
	return count, nil
}

// DeleteGlobalForUser 对某用户隐藏公告，返回条数。
//
// 用户只能删自己"当前本来就看得见"的公告；不可见/已过期/不存在的一律忽略。
func (s *Service) DeleteGlobalForUser(ctx context.Context, userID uint64, ids []uint64) (int, error) {
	if userID == 0 {
		return 0, apperr.New(apperr.CodeAuthRequired)
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	createdAt, err := s.userCreatedAt(ctx, userID)
	if err != nil {
		return 0, err
	}
	visible, err := s.globals.FindVisible(ctx, userID, ids, createdAt)
	if err != nil {
		return 0, err
	}
	if len(visible) == 0 {
		return 0, nil
	}
	visibleIDs := make([]uint64, 0, len(visible))
	for _, n := range visible {
		visibleIDs = append(visibleIDs, n.ID)
	}
	return s.globals.DeleteForUser(ctx, userID, visibleIDs)
}

// CreateGlobalInput 是创建公告的入参。
type CreateGlobalInput struct {
	ExpireAt    *time.Time
	VisibleRule domain.VisibleRule
	Title       string
	Content     string
	Icon        string
	Href        string
}

// minExpireWindow 是公告最早可设置的过期时间：至少一天以后。
const minExpireWindow = 23 * time.Hour

// defaultExpireWindow 是未指定过期时间时的默认值（一周）。
const defaultExpireWindow = 7 * 24 * time.Hour

// CreateGlobal 发布全局公告。
func (s *Service) CreateGlobal(ctx context.Context, adminID uint64, in CreateGlobalInput) (domain.GlobalNotif, error) {
	now := s.clock.Now()
	expireAt := now.Add(defaultExpireWindow)
	if in.ExpireAt != nil {
		if in.ExpireAt.Before(now.Add(minExpireWindow)) {
			return domain.GlobalNotif{}, apperr.New(apperr.CodeNotifExpireTooSoon)
		}
		expireAt = *in.ExpireAt
	}
	exists, err := s.globals.ExistsTitle(ctx, in.Title)
	if err != nil {
		return domain.GlobalNotif{}, err
	}
	if exists {
		return domain.GlobalNotif{}, apperr.New(apperr.CodeNotifTitleDuplicated)
	}
	notif := domain.GlobalNotif{
		VisibleRule: in.VisibleRule.Effective(),
		ExpireAt:    expireAt,
		ActionUser:  adminID,
		Title:       in.Title,
		Content:     in.Content,
		Icon:        in.Icon,
		Href:        in.Href,
	}
	if err := s.globals.Create(ctx, &notif); err != nil {
		return domain.GlobalNotif{}, err
	}
	return notif, nil
}

// DeleteGlobal 管理员批量删除公告。
func (s *Service) DeleteGlobal(ctx context.Context, ids []uint64) (int, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, apperr.New(apperr.CodeMessageSelectionEmpty)
	}
	count, err := s.globals.DeleteMany(ctx, ids)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, apperr.New(apperr.CodeNotifNotFound)
	}
	return count, nil
}

// userCreatedAt 读注册时间；目录缺失时退化为"很久以前注册"。
//
// 退化只影响公告可见性的边界（老用户规则），不会让接口报错。
func (s *Service) userCreatedAt(ctx context.Context, userID uint64) (time.Time, error) {
	if s.users == nil {
		return time.Time{}, nil
	}
	return s.users.CreatedAt(ctx, userID)
}

// normalizeIDs 去零去重。
func normalizeIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
