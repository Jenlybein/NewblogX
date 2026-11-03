package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// 这一组用例钉住"陌生人开关是站点级策略"这条规则（#202）。
//
// 判定点必须落在用例层：WS/HTTP 适配器只是把错误码翻成响应，
// 换一个入口不该改变"谁能给谁发"的结论。

type fakeRepo struct {
	sent []*domain.Message
	err  error
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

func (f *fakeRepo) ListSessions(context.Context, ctype.ID, bool, domain.Page) ([]domain.Session, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) FindSession(context.Context, string, ctype.ID, bool) (domain.Session, error) {
	return domain.Session{}, nil
}

func (f *fakeRepo) ListMessages(context.Context, domain.MessageQuery) ([]domain.Message, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) UserDeletedAt(context.Context, ctype.ID, string, []ctype.ID) (map[ctype.ID]time.Time, error) {
	return nil, nil
}

func (f *fakeRepo) MarkRead(context.Context, ctype.ID, []ctype.ID) ([]domain.Message, error) {
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

// fakeStranger 是站点级陌生人私信开关的快照。
//
// 它没有"查哪个用户"的参数：这正是 #202 的语义——开关属于站点，
// 不属于某个人的配置行。
type fakeStranger struct {
	enabled bool
	err     error
}

func (f fakeStranger) StrangerEnabled(context.Context) (bool, error) { return f.enabled, f.err }

func newService(t *testing.T, relation int8, strangerEnabled bool) (*app.Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	svc := app.New(
		repo,
		repo,
		fakeRelations{relation: relation},
		fakeUsers{},
		fakeStranger{enabled: strangerEnabled},
		nil,
		nil,
	)
	return svc, repo
}

func TestSend_陌生人开关关闭时拒绝并说明是站点策略(t *testing.T) {
	svc, repo := newService(t, 1, false)

	_, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if err == nil {
		t.Fatal("站点关闭陌生人私信时，陌生人之间不应发出消息")
	}
	if !apperr.IsCode(err, apperr.CodeChatStrangerOff) {
		t.Fatalf("应返回既有错误码 %s，实际 %v", apperr.CodeChatStrangerOff, err)
	}
	if repo.err != nil || len(repo.sent) != 0 {
		t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
	}

	// 读者侧显示的就是 err.Error()（WS 失败帧的 msg）。文案必须点明是
	// 站点策略、并告诉用户去找管理员开哪个开关，否则用户会以为是自己
	// 账号被限制或网络出了问题。
	msg := err.Error()
	if !strings.Contains(msg, "站点") || !strings.Contains(msg, "陌生人") {
		t.Fatalf("拒绝原因必须说明是站点开关，实际 %q", msg)
	}
	if !strings.Contains(msg, "管理员") {
		t.Fatalf("拒绝原因应提示联系管理员开启，实际 %q", msg)
	}
}

func TestSend_陌生人开关打开时放行(t *testing.T) {
	svc, repo := newService(t, 1, true)

	msg, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "你好", "")
	if err != nil {
		t.Fatalf("站点开启陌生人私信时应允许发送，实际报错: %v", err)
	}
	if msg == nil || len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d 条", len(repo.sent))
	}
}

func TestSend_已关注或互关的人不受站点开关影响(t *testing.T) {
	// 开关只管"陌生人"（relation 1）。已关注 / 粉丝 / 互关都不该被它拦住，
	// 否则关掉陌生人私信会把熟人之间的对话也一并断掉。
	for _, relation := range []int8{2, 3, 4} {
		svc, repo := newService(t, relation, false)
		if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(200), domain.TypeText, "在吗", ""); err != nil {
			t.Fatalf("relation=%d 时不应被陌生人开关拦截，实际报错: %v", relation, err)
		}
		if len(repo.sent) != 1 {
			t.Fatalf("relation=%d 时应落库 1 条，实际 %d", relation, len(repo.sent))
		}
	}
}

func TestSend_自己发给自己不受站点开关影响(t *testing.T) {
	// 自发送没有"陌生人"这层关系，不该被站点开关拦下。
	svc, repo := newService(t, 1, false)

	if _, err := svc.Send(context.Background(), ctype.NewID(100), ctype.NewID(100), domain.TypeText, "给自己", ""); err != nil {
		t.Fatalf("自己发给自己不应被陌生人开关拦截，实际报错: %v", err)
	}
	if len(repo.sent) != 1 {
		t.Fatalf("应落库 1 条消息，实际 %d", len(repo.sent))
	}
}
