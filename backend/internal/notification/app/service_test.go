package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/notification/app"
	"myblogx/internal/notification/apptest"
	"myblogx/internal/notification/domain"
)

// fixedClock 固定时间，保证过期窗口判断可复现。
type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// newService 构造带固定时钟的服务。
func newService(at time.Time) (*app.Service, *apptest.Env) {
	env := apptest.NewEnv()
	env.Globals = apptest.NewGlobals(func() time.Time { return at })
	svc := app.New(env.Messages, env.Globals, env.Confs, env.Users, env.Chat, fixedClock{at: at}, nil)
	env.Service = svc
	return svc, env
}

func TestNotifyCommentSnapshotsActionUser(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	env.Users.Put(7, apptest.User{Nickname: "小明", Avatar: "a.png"})

	err := svc.NotifyComment(ctx, app.CommentNotice{
		CommentID:    ctype.NewID(11),
		Content:      "写得不错",
		Receiver:     1,
		ActionUser:   7,
		ArticleID:    ctype.NewID(100),
		ArticleTitle: "标题",
	})
	if err != nil {
		t.Fatalf("发送评论通知失败: %v", err)
	}
	rows := env.Messages.All()
	if len(rows) != 1 {
		t.Fatalf("应生成 1 条消息，得到 %d", len(rows))
	}
	got := rows[0]
	if got.Type != domain.TypeCommentArticle || got.Receiver != 1 || got.ActionUser != 7 {
		t.Fatalf("消息字段错误: %+v", got)
	}
	if got.ActionUserNickname != "小明" || got.ActionUserAvatar != "a.png" {
		t.Fatalf("操作人快照缺失: %+v", got)
	}
}

func TestNotifyArticleDiggDeduplicates(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	in := app.InteractionNotice{Receiver: 1, ActionUser: 7, ArticleID: ctype.NewID(100), ArticleTitle: "标题"}

	if err := svc.NotifyArticleDigg(ctx, in); err != nil {
		t.Fatalf("首次点赞通知失败: %v", err)
	}
	// 取消点赞后再点赞会再触发一次，同一个人同一篇文章不该产生第二条。
	if err := svc.NotifyArticleDigg(ctx, in); err != nil {
		t.Fatalf("重复点赞通知失败: %v", err)
	}
	if got := len(env.Messages.All()); got != 1 {
		t.Fatalf("去重后应只有 1 条，得到 %d", got)
	}
}

func TestNotifySkipsZeroReceiver(t *testing.T) {
	svc, env := newService(time.Now())
	if err := svc.NotifyComment(context.Background(), app.CommentNotice{Content: "x"}); err != nil {
		t.Fatalf("空接收者不应报错: %v", err)
	}
	if got := len(env.Messages.All()); got != 0 {
		t.Fatalf("空接收者不应落库，得到 %d 条", got)
	}
	if err := svc.NotifySystem(context.Background(), app.SystemNotice{Content: "x"}); err == nil {
		t.Fatal("系统通知缺少接收者应报错")
	}
}

func TestUnreadCountsAggregatesGroups(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	receiver := uint64(1)

	for _, typ := range []domain.MessageType{domain.TypeCommentArticle, domain.TypeCommentReply, domain.TypeDiggArticle, domain.TypeSystem, domain.TypeSystem} {
		if err := env.Messages.Create(ctx, &domain.Message{Type: typ, Receiver: receiver}); err != nil {
			t.Fatalf("构造消息失败: %v", err)
		}
	}
	// 已读的不计数。
	if err := env.Messages.Create(ctx, &domain.Message{Type: domain.TypeSystem, Receiver: receiver, IsRead: true}); err != nil {
		t.Fatalf("构造已读消息失败: %v", err)
	}
	env.Chat.Set(receiver, 5)
	env.Users.Put(receiver, apptest.User{CreatedAt: time.Now().Add(-48 * time.Hour)})
	if err := env.Globals.Create(ctx, &domain.GlobalNotif{
		VisibleRule: domain.VisibleAll,
		ExpireAt:    time.Now().Add(time.Hour),
		Title:       "公告",
	}); err != nil {
		t.Fatalf("构造公告失败: %v", err)
	}

	counts, err := svc.UnreadCounts(ctx, receiver)
	if err != nil {
		t.Fatalf("统计未读失败: %v", err)
	}
	if counts.Comment != 2 || counts.DiggFavor != 1 || counts.System != 2 {
		t.Fatalf("站内信分组统计错误: %+v", counts)
	}
	if counts.Private != 5 || counts.Global != 1 {
		t.Fatalf("私信/公告统计错误: %+v", counts)
	}
}

func TestMarkAndDeleteGroup(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	receiver := uint64(1)
	for _, typ := range []domain.MessageType{domain.TypeDiggArticle, domain.TypeFavorArticle, domain.TypeSystem} {
		if err := env.Messages.Create(ctx, &domain.Message{Type: typ, Receiver: receiver}); err != nil {
			t.Fatalf("构造消息失败: %v", err)
		}
	}

	n, err := svc.MarkGroupRead(ctx, receiver, domain.GroupDiggFavor)
	if err != nil || n != 2 {
		t.Fatalf("批量已读应命中 2 条，得到 %d err=%v", n, err)
	}
	n, err = svc.DeleteGroup(ctx, receiver, domain.GroupDiggFavor)
	if err != nil || n != 2 {
		t.Fatalf("批量删除应命中 2 条，得到 %d err=%v", n, err)
	}
	left := env.Messages.All()
	if len(left) != 1 || left[0].Type != domain.TypeSystem {
		t.Fatalf("批量删除范围错误: %+v", left)
	}
}

func TestMarkMessageReadTwice(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	msg := domain.Message{Type: domain.TypeSystem, Receiver: 1}
	if err := env.Messages.Create(ctx, &msg); err != nil {
		t.Fatalf("构造消息失败: %v", err)
	}
	if err := svc.MarkMessageRead(ctx, 1, msg.ID.Uint64()); err != nil {
		t.Fatalf("首次已读失败: %v", err)
	}
	if err := svc.MarkMessageRead(ctx, 1, msg.ID.Uint64()); !apperr.IsCode(err, apperr.CodeMessageAlreadyRead) {
		t.Fatalf("重复已读应返回已读错误，得到 %v", err)
	}
	if err := svc.MarkMessageRead(ctx, 2, msg.ID.Uint64()); !apperr.IsCode(err, apperr.CodeMessageNotFound) {
		t.Fatalf("他人消息应不可见，得到 %v", err)
	}
}

func TestCreateGlobalRules(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	svc, env := newService(now)
	ctx := context.Background()

	// 默认过期时间是一周。
	notif, err := svc.CreateGlobal(ctx, 9, app.CreateGlobalInput{Title: "t1", Content: "c1"})
	if err != nil {
		t.Fatalf("创建公告失败: %v", err)
	}
	if !notif.ExpireAt.Equal(now.Add(7 * 24 * time.Hour)) {
		t.Fatalf("默认过期时间错误: %v", notif.ExpireAt)
	}
	if notif.VisibleRule != domain.VisibleRegistered {
		t.Fatalf("默认可见规则错误: %v", notif.VisibleRule)
	}

	// 过期时间不足一天要拒绝。
	tooSoon := now.Add(time.Hour)
	if _, err := svc.CreateGlobal(ctx, 9, app.CreateGlobalInput{Title: "t2", ExpireAt: &tooSoon}); !apperr.IsCode(err, apperr.CodeNotifExpireTooSoon) {
		t.Fatalf("过期时间过短应报错，得到 %v", err)
	}

	// 标题重复要拒绝。
	if _, err := svc.CreateGlobal(ctx, 9, app.CreateGlobalInput{Title: "t1"}); !apperr.IsCode(err, apperr.CodeNotifTitleDuplicated) {
		t.Fatalf("标题重复应报错，得到 %v", err)
	}

	// 管理员删除后列表为空。
	if n, err := svc.DeleteGlobal(ctx, []ctype.ID{notif.ID}); err != nil || n != 1 {
		t.Fatalf("删除公告失败: n=%d err=%v", n, err)
	}
	if len(env.Globals.Rows()) != 0 {
		t.Fatal("公告应已被删除")
	}
}

func TestGlobalReadAndUserDelete(t *testing.T) {
	now := time.Now()
	svc, env := newService(now)
	ctx := context.Background()
	user := uint64(1)
	env.Users.Put(user, apptest.User{CreatedAt: now.Add(-48 * time.Hour)})

	var ids []ctype.ID
	for i := 0; i < 2; i++ {
		n := domain.GlobalNotif{
			VisibleRule: domain.VisibleAll,
			ExpireAt:    now.Add(time.Hour),
			Title:       "公告",
			CreatedAt:   now.Add(-time.Hour),
		}
		if err := env.Globals.Create(ctx, &n); err != nil {
			t.Fatalf("构造公告失败: %v", err)
		}
		ids = append(ids, n.ID)
	}

	if n, err := svc.MarkGlobalRead(ctx, user, ids); err != nil || n != 2 {
		t.Fatalf("标记已读失败: n=%d err=%v", n, err)
	}
	if _, err := svc.MarkGlobalRead(ctx, user, ids); !apperr.IsCode(err, apperr.CodeMessageNothingRead) {
		t.Fatalf("重复标记应返回无可标记，得到 %v", err)
	}

	items, _, err := svc.ListGlobal(ctx, user, false, domain.Page{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(items) != 2 || !items[0].IsRead || !items[1].IsRead {
		t.Fatalf("列表应显示已读: %+v", items)
	}

	if n, err := svc.DeleteGlobalForUser(ctx, user, ids); err != nil || n != 2 {
		t.Fatalf("用户删除公告失败: n=%d err=%v", n, err)
	}
	items, _, err = svc.ListGlobal(ctx, user, false, domain.Page{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("删除后不应再出现: %+v", items)
	}
	// 另一个用户仍然看得到。
	env.Users.Put(2, apptest.User{CreatedAt: now.Add(-48 * time.Hour)})
	other, _, err := svc.ListGlobal(ctx, 2, false, domain.Page{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(other) != 2 {
		t.Fatalf("删除只应影响本人，另一个用户应仍可见，得到 %d", len(other))
	}
}

func TestConfUpdateReplacesAllFields(t *testing.T) {
	svc, env := newService(time.Now())
	ctx := context.Background()
	user := uint64(1)
	env.Confs.Put(user, domain.Conf{})

	if _, err := svc.GetConf(ctx, user); err != nil {
		t.Fatalf("读开关失败: %v", err)
	}
	if err := svc.UpdateConf(ctx, user, domain.Conf{DiggNoticeEnabled: true}); err != nil {
		t.Fatalf("更新开关失败: %v", err)
	}
	conf, err := svc.GetConf(ctx, user)
	if err != nil {
		t.Fatalf("读开关失败: %v", err)
	}
	// 全量替换语义：只传 digg 时其余开关被写回 false。
	if !conf.DiggNoticeEnabled || conf.CommentNoticeEnabled || conf.FavorNoticeEnabled || conf.PrivateChatNoticeEnabled {
		t.Fatalf("开关替换语义错误: %+v", conf)
	}
	if err := svc.UpdateConf(ctx, 99, domain.Conf{}); !apperr.IsCode(err, apperr.CodeMessageConfMissing) {
		t.Fatalf("缺少配置应报错，得到 %v", err)
	}
}
