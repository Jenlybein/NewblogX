package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/moderation/app"
	"myblogx/internal/moderation/domain"
)

// 本文件是 ADR-0035 在用例层的接缝：一条违规从"记下来"到"处罚落地"再到
// "申诉退回"的完整链路。断言的是可观察结果（分数、禁言天数、通知条数、
// 事件状态），不是"内部先调了哪个方法"。

type fakeRepo struct {
	events  []domain.Event
	appeals map[ctype.ID]domain.Appeal
	nextID  uint64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{appeals: map[ctype.ID]domain.Appeal{}, nextID: 1}
}

func (r *fakeRepo) Append(_ context.Context, event domain.Event) (domain.Event, bool, error) {
	for _, existing := range r.events {
		if existing.RefType == event.RefType && existing.RefID == event.RefID && existing.UserID == event.UserID {
			return event, false, nil
		}
	}
	event.ID = ctype.NewID(r.nextID)
	r.nextID++
	if event.Status == "" {
		event.Status = domain.StatusActive
	}
	r.events = append(r.events, event)
	return event, true, nil
}

func (r *fakeRepo) ListByUser(_ context.Context, userID ctype.ID, _ int) ([]domain.Event, error) {
	out := make([]domain.Event, 0, len(r.events))
	for _, event := range r.events {
		if event.UserID == userID {
			out = append(out, event)
		}
	}
	return out, nil
}

func (r *fakeRepo) FindByID(_ context.Context, id ctype.ID) (domain.Event, bool, error) {
	for _, event := range r.events {
		if event.ID == id {
			return event, true, nil
		}
	}
	return domain.Event{}, false, nil
}

func (r *fakeRepo) Revoke(_ context.Context, id, _ ctype.ID, _ string, _ time.Time) error {
	for i := range r.events {
		if r.events[i].ID == id {
			r.events[i].Status = domain.StatusRevoked
		}
	}
	return nil
}

func (r *fakeRepo) Create(_ context.Context, appeal domain.Appeal) (domain.Appeal, error) {
	if _, ok := r.appeals[appeal.EventID]; ok {
		return domain.Appeal{}, errDuplicate
	}
	appeal.ID = ctype.NewID(r.nextID)
	r.nextID++
	appeal.Status = domain.AppealPending
	r.appeals[appeal.EventID] = appeal
	return appeal, nil
}

func (r *fakeRepo) FindByEvent(_ context.Context, eventID ctype.ID) (domain.Appeal, bool, error) {
	appeal, ok := r.appeals[eventID]
	return appeal, ok, nil
}

func (r *fakeRepo) findAppealByID(_ context.Context, id ctype.ID) (domain.Appeal, bool, error) {
	for _, appeal := range r.appeals {
		if appeal.ID == id {
			return appeal, true, nil
		}
	}
	return domain.Appeal{}, false, nil
}

func (r *fakeRepo) List(_ context.Context, status domain.AppealStatus, _ int) ([]domain.Appeal, error) {
	out := make([]domain.Appeal, 0, len(r.appeals))
	for _, appeal := range r.appeals {
		if status == "" || appeal.Status == status {
			out = append(out, appeal)
		}
	}
	return out, nil
}

func (r *fakeRepo) Handle(_ context.Context, id, operator ctype.ID, status domain.AppealStatus, note string, at time.Time) error {
	for key, appeal := range r.appeals {
		if appeal.ID == id {
			appeal.Status = status
			appeal.Note = note
			appeal.HandledBy = operator
			appeal.HandledAt = &at
			r.appeals[key] = appeal
		}
	}
	return nil
}

type duplicateError struct{}

func (duplicateError) Error() string { return "duplicate" }

var errDuplicate = duplicateError{}

// appealRepo 是 AppealRepository 的适配：fakeRepo 已经有同名 FindByID（事件的），
// 而 Go 不允许同一个类型上有两个同名不同签名的方法，所以申诉那几个方法
// 在这里包一层。
type appealRepo struct{ inner *fakeRepo }

func (r appealRepo) Create(ctx context.Context, appeal domain.Appeal) (domain.Appeal, error) {
	return r.inner.Create(ctx, appeal)
}

func (r appealRepo) FindByEvent(ctx context.Context, eventID ctype.ID) (domain.Appeal, bool, error) {
	return r.inner.FindByEvent(ctx, eventID)
}

func (r appealRepo) FindByID(ctx context.Context, id ctype.ID) (domain.Appeal, bool, error) {
	return r.inner.findAppealByID(ctx, id)
}

func (r appealRepo) List(ctx context.Context, status domain.AppealStatus, limit int) ([]domain.Appeal, error) {
	return r.inner.List(ctx, status, limit)
}

func (r appealRepo) Handle(ctx context.Context, id, operator ctype.ID, status domain.AppealStatus, note string, at time.Time) error {
	return r.inner.Handle(ctx, id, operator, status, note, at)
}

// fakeAccount 记录禁言调用。
type fakeAccount struct {
	admin    bool
	muted    bool
	muteDays []int
	unmuted  int
}

func (a *fakeAccount) IsAdmin(context.Context, ctype.ID) (bool, error) { return a.admin, nil }
func (a *fakeAccount) IsMuted(context.Context, ctype.ID) (bool, error) { return a.muted, nil }
func (a *fakeAccount) Mute(_ context.Context, _ ctype.ID, days int) error {
	a.muteDays = append(a.muteDays, days)
	return nil
}
func (a *fakeAccount) Unmute(context.Context, ctype.ID) error {
	a.unmuted++
	return nil
}

// fakeNotifier 记录通知条数。
type fakeNotifier struct{ contents []string }

func (n *fakeNotifier) NotifySystem(_ context.Context, _ ctype.ID, content, _, _ string) error {
	n.contents = append(n.contents, content)
	return nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func newSvc(account *fakeAccount) (*app.Service, *fakeRepo, *fakeNotifier) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	svc := app.New(repo, account, notifier, fixedClock{now: time.Now()}, nil)
	svc.SetAppeals(appealRepo{inner: repo})
	return svc, repo, notifier
}

// TestRecordImage_计分并通知 钉住图片违规 +20 与通知。
func TestRecordImage_计分并通知(t *testing.T) {
	t.Parallel()

	svc, _, notifier := newSvc(&fakeAccount{})
	result, err := svc.Record(context.Background(), app.RecordInput{
		UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine,
		RefType: "image", RefID: ctype.NewID(11),
	})
	if err != nil {
		t.Fatalf("记账失败: %v", err)
	}
	if result.Score != 20 {
		t.Fatalf("一张违规图应得 20 分，实际 %v", result.Score)
	}
	if result.Punishment != domain.PunishmentNone {
		t.Fatalf("20 分不该触发处罚，实际 %s", result.Punishment)
	}
	if len(notifier.contents) != 1 {
		t.Fatalf("应发一条通知，实际 %d 条", len(notifier.contents))
	}
}

// TestRecord_Idempotent 钉住"同一件事只扣一次分"。
func TestRecord_Idempotent(t *testing.T) {
	t.Parallel()

	svc, _, notifier := newSvc(&fakeAccount{})
	in := app.RecordInput{UserID: ctype.NewID(7), Level: domain.LevelKeyword, Source: domain.SourceMachine, RefType: "filter_hit", RefID: ctype.NewID(5)}

	if _, err := svc.Record(context.Background(), in); err != nil {
		t.Fatalf("第一次记账失败: %v", err)
	}
	second, err := svc.Record(context.Background(), in)
	if err != nil {
		t.Fatalf("第二次记账失败: %v", err)
	}
	if second.Inserted {
		t.Fatal("重复的同一件事不该再记一次")
	}
	if len(notifier.contents) != 1 {
		t.Fatalf("重复判定不该重复通知，实际 %d 条", len(notifier.contents))
	}
}

// TestRecord_管理员豁免 钉住"站长自己不被扣分封禁"。
func TestRecord_管理员豁免(t *testing.T) {
	t.Parallel()

	svc, repo, notifier := newSvc(&fakeAccount{admin: true})
	result, err := svc.Record(context.Background(), app.RecordInput{
		UserID: ctype.NewID(1), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(3),
	})
	if err != nil {
		t.Fatalf("记账失败: %v", err)
	}
	if result.Skipped == "" {
		t.Fatal("管理员应被豁免并给出原因")
	}
	if len(repo.events) != 0 || len(notifier.contents) != 0 {
		t.Fatal("管理员不该产生事件或通知")
	}
}

// TestRecord_到阈值禁言 钉住 100 分落 1 天禁言。
func TestRecord_到阈值禁言(t *testing.T) {
	t.Parallel()

	account := &fakeAccount{}
	svc, _, _ := newSvc(account)

	// 5 张违规图 = 100 分。
	for i := uint64(1); i <= 5; i++ {
		if _, err := svc.Record(context.Background(), app.RecordInput{
			UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(i),
		}); err != nil {
			t.Fatalf("第 %d 次记账失败: %v", i, err)
		}
	}
	if len(account.muteDays) != 1 || account.muteDays[0] != 1 {
		t.Fatalf("100 分应禁言 1 天，实际 %v", account.muteDays)
	}
}

// TestRecord_封禁期内再违规升一档 钉住"再犯不原地踏步"。
func TestRecord_封禁期内再违规升一档(t *testing.T) {
	t.Parallel()

	account := &fakeAccount{}
	svc, _, _ := newSvc(account)

	for i := uint64(1); i <= 5; i++ {
		_, _ = svc.Record(context.Background(), app.RecordInput{
			UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(i),
		})
	}
	// 已在禁言期：下一条违规升到 7 天档。
	account.muted = true
	if _, err := svc.Record(context.Background(), app.RecordInput{
		UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(99),
	}); err != nil {
		t.Fatalf("记账失败: %v", err)
	}
	if len(account.muteDays) != 2 || account.muteDays[1] != 7 {
		t.Fatalf("禁言期内再违规应升到 7 天，实际 %v", account.muteDays)
	}
}

// TestAppeal_通过后撤销并解禁 钉住"申诉通过 = 后果真的退回去"。
func TestAppeal_通过后撤销并解禁(t *testing.T) {
	t.Parallel()

	account := &fakeAccount{}
	svc, repo, notifier := newSvc(account)
	ctx := context.Background()

	recorded, err := svc.Record(ctx, app.RecordInput{
		UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(1),
	})
	_ = recorded
	if err != nil {
		t.Fatalf("记账失败: %v", err)
	}
	eventID := repo.events[0].ID

	appeal, err := svc.SubmitAppeal(ctx, ctype.NewID(7), eventID, "这张图不是我的")
	if err != nil {
		t.Fatalf("提交申诉失败: %v", err)
	}

	if _, err := svc.HandleAppeal(ctx, appeal.ID, ctype.NewID(1), true, ""); err != nil {
		t.Fatalf("处置申诉失败: %v", err)
	}
	if repo.events[0].Status != domain.StatusRevoked {
		t.Fatalf("申诉通过后事件应被撤销，实际 %s", repo.events[0].Status)
	}
	if account.unmuted != 1 {
		t.Fatalf("分数降回处罚线以下应解禁，实际解禁 %d 次", account.unmuted)
	}
	if len(notifier.contents) < 2 {
		t.Fatal("申诉结果应通知用户")
	}
}

// TestAppeal_超窗口拒绝 钉住 7 天窗口。
func TestAppeal_超窗口拒绝(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := app.New(repo, &fakeAccount{}, &fakeNotifier{}, fixedClock{now: time.Now()}, nil)
	svc.SetAppeals(appealRepo{inner: repo})

	// 直接塞一条 8 天前的事件。
	event, _, _ := repo.Append(context.Background(), domain.Event{
		UserID: ctype.NewID(7), Level: domain.LevelImage, Score: 20, Source: domain.SourceMachine,
		RefType: "image", RefID: ctype.NewID(1), Status: domain.StatusActive,
		CreatedAt: time.Now().Add(-8 * 24 * time.Hour),
	})

	if _, err := svc.SubmitAppeal(context.Background(), ctype.NewID(7), event.ID, "理由"); err == nil {
		t.Fatal("超过 7 天的处罚不应可申诉")
	}
}

// TestAppeal_驳回必须填理由 钉住"用户要知道为什么"。
func TestAppeal_驳回必须填理由(t *testing.T) {
	t.Parallel()

	account := &fakeAccount{}
	svc, repo, _ := newSvc(account)
	ctx := context.Background()

	if _, err := svc.Record(ctx, app.RecordInput{
		UserID: ctype.NewID(7), Level: domain.LevelImage, Source: domain.SourceMachine, RefType: "image", RefID: ctype.NewID(1),
	}); err != nil {
		t.Fatalf("记账失败: %v", err)
	}
	appeal, err := svc.SubmitAppeal(ctx, ctype.NewID(7), repo.events[0].ID, "理由")
	if err != nil {
		t.Fatalf("提交申诉失败: %v", err)
	}
	if _, err := svc.HandleAppeal(ctx, appeal.ID, ctype.NewID(1), false, ""); err == nil {
		t.Fatal("驳回必须填写理由")
	}
}
