package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 这一组用例钉住"陌生人判定看谁"这条规则（#243）。
//
// 判定点必须落在用例层：WS/HTTP 适配器只是把错误码翻成响应，
// 换一个入口不该改变"谁能给谁发"的结论。真正两级（站点 AND 接收方）
// 的取值在装配层，见 app/chat_wiring_test.go。

type fakeRepo struct {
	sent []*domain.Message
	err  error
	// existingSession 为 true 时表示双方已有会话（#243 的"会话关系"一级）。
	existingSession bool
	sessions        []domain.Session
	lastQuery       domain.SessionQuery
}

func (f *fakeRepo) Send(_ context.Context, cmd domain.SendCommand, _, _ domain.Display, _ string) (domain.Message, bool, error) {
	if f.err != nil {
		return domain.Message{}, false, f.err
	}
	msg := domain.Message{
		ID:         ctype.NewID(uint64(len(f.sent) + 1)),
		SenderID:   cmd.SenderID,
		ReceiverID: cmd.ReceiverID,
		SessionID:  domain.BuildSessionID(cmd.SenderID, cmd.ReceiverID),
		Content:    cmd.Content,
		SendTime:   cmd.SendTime,
		MsgStatus:  domain.StatusSent,
		MsgType:    cmd.MsgType,
	}
	f.sent = append(f.sent, &msg)
	return msg, false, nil
}

func (f *fakeRepo) ListSessions(_ context.Context, q domain.SessionQuery) ([]domain.Session, int, error) {
	f.lastQuery = q
	return f.sessions, len(f.sessions), nil
}

func (f *fakeRepo) FindSession(context.Context, string, ctype.ID, bool) (domain.Session, error) {
	if f.existingSession {
		return domain.Session{}, nil
	}
	return domain.Session{}, apperr.New(apperr.CodeChatSessionNotFound)
}

func (f *fakeRepo) ListMessages(context.Context, domain.MessageQuery) (domain.MessageQueryResult, error) {
	return domain.MessageQueryResult{}, nil
}

func (f *fakeRepo) UserDeletedAt(context.Context, ctype.ID, string, []ctype.ID) (map[ctype.ID]time.Time, error) {
	return nil, nil
}

func (f *fakeRepo) MarkRead(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) MarkReadSession(context.Context, ctype.ID, string) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) DeleteMessagesForUser(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeRepo) DeleteSessions(context.Context, ctype.ID, []string) ([]string, error) {
	return nil, nil
}

type fakeRelations struct {
	relation int8
}

func (f fakeRelations) Batch(_ context.Context, _ ctype.ID, ids []ctype.ID) (map[ctype.ID]int8, error) {
	out := make(map[ctype.ID]int8, len(ids))
	for _, id := range ids {
		out[id] = f.relation
	}
	return out, nil
}

type fakeUsers struct{}

func (fakeUsers) Displays(_ context.Context, ids []ctype.ID) (map[ctype.ID]domain.Display, error) {
	out := make(map[ctype.ID]domain.Display, len(ids))
	for _, id := range ids {
		out[id] = domain.Display{Nickname: "用户" + id.String(), Avatar: ""}
	}
	return out, nil
}

// fakeStranger 是**有效陌生人判定**的替身：站点总闸与接收方个人开关的
// 交集由装配层的 strangerBridge 算好，用例层只消费结论（#247 起结论带原因）。
//
// receiverIDs 记录判定时传入的接收方：接收方是谁必须参与结论，否则
// 个人级开关就没有接线点（#243）。
type fakeStranger struct {
	verdict     domain.StrangerVerdict
	err         error
	receiverIDs []ctype.ID
}

func (f *fakeStranger) CheckStranger(_ context.Context, receiverID ctype.ID) (domain.StrangerVerdict, error) {
	f.receiverIDs = append(f.receiverIDs, receiverID)
	if f.err != nil {
		return domain.StrangerAllowed, f.err
	}
	return f.verdict, nil
}

func newService(t *testing.T, relation int8, verdict domain.StrangerVerdict) (*app.Service, *fakeRepo, *fakeStranger) {
	t.Helper()
	repo := &fakeRepo{}
	stranger := &fakeStranger{verdict: verdict}
	svc := app.New(repo, repo, fakeRelations{relation: relation}, fakeUsers{}, stranger,
		permitLimiter{}, &allowOwnedImage{}, &fakeBlocks{}, nil)
	return svc, repo, stranger
}

// permitLimiter 是"额度永远够"的限流替身：本文件的用例关注陌生人 / 屏蔽 /
// 图片来源，不关注按对限流本身。
type permitLimiter struct{}

func (permitLimiter) Reserve(context.Context, ctype.ID, ctype.ID, int8) (domain.Reservation, error) {
	return permitReservation{}, nil
}

type permitReservation struct{}

func (permitReservation) Commit() error   { return nil }
func (permitReservation) Rollback() error { return nil }

// fakeBlocks 让用例层能验证"屏蔽 / 拉黑"确实在发送时生效。
// 会话列表的拉黑过滤已经下推到查库层，不在这个端口里。
type fakeBlocks struct {
	outgoing domain.BlockLevel
	incoming domain.BlockLevel
}

func (f *fakeBlocks) Level(_ context.Context, owner, target ctype.ID) (domain.BlockLevel, error) {
	if owner == ctype.NewID(100) && target == ctype.NewID(200) {
		return f.outgoing, nil
	}
	if owner == ctype.NewID(200) && target == ctype.NewID(100) {
		return f.incoming, nil
	}
	return domain.BlockLevelNone, nil
}

// allowOwnedImage 让用例层能发图片；"是不是本站图片"的判定逻辑在装配层
// （chatImageVerifier），这里只验用例层**有没有调用这个校验**。
type allowOwnedImage struct {
	calls int
	owned bool
	err   error
}

func (v *allowOwnedImage) IsOwnedImageURL(_ context.Context, _ string) (bool, error) {
	v.calls++
	return v.owned, v.err
}

// TestSend_陌生人被站点总闸关闭时拒绝并说明站点原因 站点级拒绝保留原错误码：
// 这不是接收方的意愿，读者应去找管理员打开站点开关。
func TestSend_陌生人被站点总闸关闭时拒绝并说明站点原因(t *testing.T) {
	svc, repo, _ := newService(t, 1, domain.StrangerBlockedBySite)

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if err == nil {
		t.Fatal("站点总闸关闭时，陌生人之间不应发出消息")
	}
	if !apperr.IsCode(err, apperr.CodeChatStrangerOff) {
		t.Fatalf("站点级拒绝应返回既有错误码 %s，实际 %v", apperr.CodeChatStrangerOff, err)
	}
	if repo.err != nil || len(repo.sent) != 0 {
		t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
	}

	// 读者侧显示的就是 err.Error()（WS 失败帧的 msg）。文案必须点明是
	// 站点私信策略、并告诉用户去找管理员开哪个开关，否则用户会以为是自己
	// 账号被限制或网络出了问题。
	msg := err.Error()
	if !strings.Contains(msg, "站点") || !strings.Contains(msg, "陌生人") {
		t.Fatalf("站点级拒绝原因必须说明是站点开关，实际 %q", msg)
	}
	if !strings.Contains(msg, "管理员") {
		t.Fatalf("站点级拒绝原因应提示联系管理员开启，实际 %q", msg)
	}
}

// TestSend_陌生人被接收方个人开关关闭时提示对方设置 #247：同样是拒绝，
// 但原因在接收方的个人开关上。文案必须指向接收方，不能把用户引向管理员
// （管理员查站点开关只会查到"已开启"，帮不上忙）。
func TestSend_陌生人被接收方个人开关关闭时提示对方设置(t *testing.T) {
	svc, repo, _ := newService(t, 1, domain.StrangerBlockedByRecipient)

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if err == nil {
		t.Fatal("接收方关闭陌生人私信时，陌生人之间不应发出消息")
	}
	if !apperr.IsCode(err, apperr.CodeChatRecipientStrangerOff) {
		t.Fatalf("接收方级拒绝应返回 %s，实际 %v", apperr.CodeChatRecipientStrangerOff, err)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
	}

	msg := err.Error()
	if !strings.Contains(msg, "对方") {
		t.Fatalf("接收方级拒绝应提示是对方的设置，实际 %q", msg)
	}
	if strings.Contains(msg, "管理员") {
		t.Fatalf("接收方级拒绝不该把用户引向管理员（管理员查不到问题），实际 %q", msg)
	}
}

func TestSend_陌生人有效开关打开时放行(t *testing.T) {
	svc, repo, _ := newService(t, 1, domain.StrangerAllowed)

	msg, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if err != nil {
		t.Fatalf("有效开关开启时应允许发送，实际报错: %v", err)
	}
	if msg == nil || len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d 条", len(repo.sent))
	}
}

// TestSend_陌生人判定传的是接收方 钉住 #243 的接线点：判定必须拿到接收方 ID，
// 而不是只看站点级开关。传成发送方 / 不传都会让"接收方个人开关"永远不生效。
func TestSend_陌生人判定传的是接收方(t *testing.T) {
	svc, _, stranger := newService(t, 1, domain.StrangerAllowed)

	if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", ""); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if len(stranger.receiverIDs) != 1 || stranger.receiverIDs[0] != ctype.NewID(200) {
		t.Fatalf("陌生人判定应传入接收方 200，实际 %v", stranger.receiverIDs)
	}
}

// TestSend_陌生人判定错误上抛 读接收方偏好失败不能被吞成"允许"。
func TestSend_陌生人判定错误上抛(t *testing.T) {
	svc, repo, stranger := newService(t, 1, domain.StrangerAllowed)
	boom := errors.New("读接收方偏好失败")
	stranger.err = boom

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if !errors.Is(err, boom) {
		t.Fatalf("应原样上抛接收方偏好读取错误，实际 %v", err)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("判定失败时不能落库，实际写入 %d 条", len(repo.sent))
	}
}

func TestSend_已存在会话的双方不受陌生人开关影响(t *testing.T) {
	// "陌生人"的反面不只关注关系：已经聊过的双方再发消息，不该被个人开关
	// 半路切断（#243 的口径是"有关注/会话关系都不判定"）。
	svc, repo, stranger := newService(t, 1, domain.StrangerBlockedByRecipient)
	repo.existingSession = true

	if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "继续聊", ""); err != nil {
		t.Fatalf("已有会话的双方不应被陌生人开关拦截，实际报错: %v", err)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d", len(repo.sent))
	}
	if len(stranger.receiverIDs) != 0 {
		t.Fatalf("已有会话时不应做陌生人判定，实际查询了 %v", stranger.receiverIDs)
	}
}

func TestSend_已关注或互关的人不受陌生人开关影响(t *testing.T) {
	// 开关只管"陌生人"（relation 1）。已关注 / 粉丝 / 互关都不该被它拦住，
	// 否则关掉陌生人私信会把熟人之间的对话也一并断掉。
	for _, relation := range []int8{2, 3, 4} {
		svc, repo, stranger := newService(t, relation, domain.StrangerBlockedByRecipient)
		if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "在吗", ""); err != nil {
			t.Fatalf("relation=%d 时不应被陌生人开关拦截，实际报错: %v", relation, err)
		}
		if len(repo.sent) != 1 {
			t.Fatalf("relation=%d 时应落库 1 条，实际 %d", relation, len(repo.sent))
		}
		if len(stranger.receiverIDs) != 0 {
			t.Fatalf("relation=%d 时不应做陌生人判定，实际查询了 %v", relation, stranger.receiverIDs)
		}
	}
}

func TestSend_自己发给自己不受陌生人开关影响(t *testing.T) {
	// 自发送没有"陌生人"这层关系，不该被陌生人开关拦下。
	svc, repo, stranger := newService(t, 1, domain.StrangerBlockedByRecipient)

	if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(100), domain.TypeText, "给自己", ""); err != nil {
		t.Fatalf("自己发给自己不应被陌生人开关拦截，实际报错: %v", err)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d", len(repo.sent))
	}
	if len(stranger.receiverIDs) != 0 {
		t.Fatalf("自己发给自己不应做陌生人判定，实际查询了 %v", stranger.receiverIDs)
	}
}

// 票 267：图片私信的 content 是通用字符串，用例层必须先问装配层"是不是本站上传产物"，
// 再包成 {"kind":"image","image_url":...}。文本消息完全不走这条校验。
func TestSend_图片消息必须校验来源(t *testing.T) {
	repo := &fakeRepo{}
	check := &allowOwnedImage{owned: false}
	svc := app.New(repo, repo, fakeRelations{relation: 4}, fakeUsers{},
		&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, check, &fakeBlocks{}, nil)

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200),
		domain.TypeImage, "https://evil.example.com/pixel.png", "")
	if !apperr.IsCode(err, apperr.CodeChatImageNotOwned) {
		t.Fatalf("外链图片必须被拒，实际 err=%v", err)
	}
	if check.calls != 1 {
		t.Fatalf("图片消息应调用一次来源校验，实际 %d", check.calls)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("被拒的图片消息不得落库，实际 %d 条", len(repo.sent))
	}
}

func TestSend_本站图片可发且文本不触发校验(t *testing.T) {
	repo := &fakeRepo{}
	check := &allowOwnedImage{owned: true}
	svc := app.New(repo, repo, fakeRelations{relation: 4}, fakeUsers{},
		&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, check, &fakeBlocks{}, nil)

	if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200),
		domain.TypeText, "你好", ""); err != nil {
		t.Fatalf("文本消息不应失败: %v", err)
	}
	if check.calls != 0 {
		t.Fatalf("文本消息不该触发图片来源校验，实际 %d 次", check.calls)
	}

	msg, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200),
		domain.TypeImage, "https://cdn.example.com/myblogx/images/20261104/abc.png", "")
	if err != nil {
		t.Fatalf("本站图片应可发送: %v", err)
	}
	if !strings.Contains(msg.Content, `"kind":"image"`) ||
		!strings.Contains(msg.Content, "https://cdn.example.com/myblogx/images/20261104/abc.png") {
		t.Fatalf("图片消息应保持既有 JSON 契约，实际 %s", msg.Content)
	}
	if check.calls != 1 {
		t.Fatalf("图片消息应调用一次来源校验，实际 %d", check.calls)
	}
}

func TestSend_未配置图片校验时失败关闭(t *testing.T) {
	repo := &fakeRepo{}
	svc := app.New(repo, repo, fakeRelations{relation: 4}, fakeUsers{},
		&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, nil, &fakeBlocks{}, nil)

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200),
		domain.TypeImage, "https://cdn.example.com/myblogx/images/20261104/abc.png", "")
	if !apperr.IsCode(err, apperr.CodeChatImageNotOwned) {
		t.Fatalf("没有校验端口时必须失败关闭，实际 err=%v", err)
	}
	if len(repo.sent) != 0 {
		t.Fatalf("失败关闭时不得落库，实际 %d 条", len(repo.sent))
	}
}

// 票 268 / ADR-0026：屏蔽 / 拉黑在发送与会话列表两处生效，且方向独立。
func TestSend_屏蔽或拉黑后双方都不能发(t *testing.T) {
	for _, tc := range []struct {
		name     string
		outgoing domain.BlockLevel
		incoming domain.BlockLevel
	}{
		{"我屏蔽对方", domain.BlockLevelSoft, domain.BlockLevelNone},
		{"我拉黑对方", domain.BlockLevelHard, domain.BlockLevelNone},
		{"对方拉黑我", domain.BlockLevelNone, domain.BlockLevelHard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := app.New(repo, repo, fakeRelations{relation: 4}, fakeUsers{},
				&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, &allowOwnedImage{},
				&fakeBlocks{outgoing: tc.outgoing, incoming: tc.incoming}, nil)

			_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200),
				domain.TypeText, "在吗", "")
			if !apperr.IsCode(err, apperr.CodeChatBlocked) {
				t.Fatalf("屏蔽 / 拉黑关系下应拒绝发送，实际 err=%v", err)
			}
			if len(repo.sent) != 0 {
				t.Fatalf("被拒的消息不得落库，实际 %d 条", len(repo.sent))
			}
		})
	}
}

func TestSessions_拉黑过滤由查库层完成而用例层不二次扣减(t *testing.T) {
	// 查库层已经滤掉拉黑对象 200，只返回 300 与过滤后的真值总数 1。
	// 旧实现会在此基础上再减去"全部拉黑目标数"，把 total 错扣成 0。
	repo := &fakeRepo{sessions: []domain.Session{
		{SessionID: "chat:100:300", ReceiverID: ctype.NewID(300)},
	}}
	svc := app.New(repo, repo, fakeRelations{}, fakeUsers{},
		&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, &allowOwnedImage{}, &fakeBlocks{}, nil)

	items, total, err := svc.Sessions(context.Background(), ctype.NewID(100), ctype.NewID(100), false, domain.Page{})
	if err != nil {
		t.Fatalf("会话列表失败: %v", err)
	}
	if len(items) != 1 || items[0].SessionID != "chat:100:300" || total != 1 {
		t.Fatalf("总数必须是过滤后的真值，实际 items=%v total=%d", items, total)
	}
	if !repo.lastQuery.HideBlocked || repo.lastQuery.Unscoped {
		t.Fatalf("读者视图应让查库层过滤拉黑且不看已删会话，实际 %+v", repo.lastQuery)
	}
}

// TestSessions_管理员视图不过滤拉黑是有意例外 运维排查需要看全量会话；
// 这条例外与读者视图不同，必须显式保持（#280 AC 1 / #272 User Story 15）。
func TestSessions_管理员视图不过滤拉黑是有意例外(t *testing.T) {
	repo := &fakeRepo{sessions: []domain.Session{
		{SessionID: "chat:100:200", ReceiverID: ctype.NewID(200)},
	}}
	svc := app.New(repo, repo, fakeRelations{}, fakeUsers{},
		&fakeStranger{verdict: domain.StrangerAllowed}, permitLimiter{}, &allowOwnedImage{}, &fakeBlocks{}, nil)

	items, total, err := svc.Sessions(context.Background(), ctype.NewID(1), ctype.NewID(100), true, domain.Page{})
	if err != nil {
		t.Fatalf("管理员视图失败: %v", err)
	}
	if len(items) != 1 || total != 1 {
		t.Fatalf("管理员视图应看全量会话，实际 items=%v total=%d", items, total)
	}
	if repo.lastQuery.HideBlocked || !repo.lastQuery.Unscoped {
		t.Fatalf("管理员视图必须不过滤拉黑、并连已删会话一起看，实际 %+v", repo.lastQuery)
	}
}
