package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/social/app"
	"myblogx/internal/social/domain"
)

// 本文件是关系用例层的契约测试（票 327）。
//
// 为什么这一层要测：三个视图"取哪一端当对方"与"可见性用哪个开关"都是**判断**，
// 不是 SQL；写错了不会报错，只会安静地把粉丝显示成关注的人，或者让别人的私密列表
// 在互关视图里漏出来。仓储用有状态假实现（ADR-0006），断言的是用例层的选择。

// fakeRepo 是有状态的关注关系假实现。
type fakeRepo struct {
	rows []domain.Follow
	// 记录最后一次列表调用的视图，用来证明用例层把 kind 传下去了。
	lastKind   domain.RelationKind
	lastOwner  uint64
	unfollowed [][2]uint64
	// unfollowFound 决定下一次 Unfollow 的返回值。
	unfollowFound bool
	relations     map[uint64]domain.Relation
	counts        domain.RelationCounts
}

func (r *fakeRepo) Follow(context.Context, *domain.Follow) (bool, error) { return true, nil }

func (r *fakeRepo) Unfollow(_ context.Context, followedID, fansID uint64) (bool, error) {
	r.unfollowed = append(r.unfollowed, [2]uint64{followedID, fansID})
	return r.unfollowFound, nil
}

func (r *fakeRepo) ListRelations(_ context.Context, ownerID uint64, kind domain.RelationKind, _ domain.Page) ([]domain.Follow, bool, error) {
	r.lastOwner = ownerID
	r.lastKind = kind
	return r.rows, false, nil
}

func (r *fakeRepo) CountRelations(context.Context, uint64) (domain.RelationCounts, error) {
	return r.counts, nil
}

func (r *fakeRepo) RelationBatch(context.Context, uint64, []uint64) (map[uint64]domain.Relation, error) {
	return r.relations, nil
}

// statDelta 是一次统计回写的三个参数（ID 用 ctype.ID：R12 要求 ID 字段不得是裸整型）。
type statDelta struct {
	fansID     ctype.ID
	followedID ctype.ID
	delta      int
}

// fakeStats 记录统计增量（顺序敏感：第一个参数是粉丝、第二个是被关注者）。
type fakeStats struct {
	deltas []statDelta
}

func (s *fakeStats) ApplyFollowDelta(_ context.Context, fansID, followedID uint64, delta int) error {
	s.deltas = append(s.deltas, statDelta{
		fansID:     ctype.NewID(fansID),
		followedID: ctype.NewID(followedID),
		delta:      delta,
	})
	return nil
}

// fakeUsers 提供展示快照，用于验证"快照为空时补齐"。
type fakeUsers struct {
	displays map[uint64]domain.Display
}

func (u *fakeUsers) Displays(context.Context, []uint64) (map[uint64]domain.Display, error) {
	return u.displays, nil
}

// fakePrivacy 的可见性由字段决定，两个开关分开记，用来证明"粉丝视图用粉丝开关"。
type fakePrivacy struct {
	followVisible bool
	fansVisible   bool
	followAsked   bool
	fansAsked     bool
}

func (p *fakePrivacy) FollowVisible(context.Context, uint64) (bool, error) {
	p.followAsked = true
	return p.followVisible, nil
}

func (p *fakePrivacy) FansVisible(context.Context, uint64) (bool, error) {
	p.fansAsked = true
	return p.fansVisible, nil
}

func newService(repo *fakeRepo, stats *fakeStats, users *fakeUsers, privacy *fakePrivacy) *app.Service {
	if repo.relations == nil {
		repo.relations = map[uint64]domain.Relation{}
	}
	return app.New(repo, stats, users, privacy, nil)
}

// bothSides 造一条"我关注 B"的行：两侧快照都填上，方便断言取的是哪一端。
func bothSides(id uint64) domain.Follow {
	return domain.Follow{
		FollowedUserID:   ctype.NewID(id),
		FollowedNickname: "被关注者",
		FollowedAvatar:   "b.png",
		FollowedAbstract: "被关注者的简介",
		FansUserID:       ctype.NewID(9),
		FansNickname:     "粉丝",
		FansAvatar:       "f.png",
		FansAbstract:     "粉丝的简介",
	}
}

func wantCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	code, ok := apperr.CodeOf(err)
	if !ok || code != want {
		t.Fatalf("期望错误码 %s，实际 %v", want, err)
	}
}

func TestListRelationsProjectsCounterpartByKind(t *testing.T) {
	repo := &fakeRepo{rows: []domain.Follow{bothSides(200)}}
	privacy := &fakePrivacy{followVisible: true, fansVisible: true}
	svc := newService(repo, &fakeStats{}, &fakeUsers{}, privacy)

	// 关注视图：对方是被关注者。
	items, _, err := svc.ListRelations(context.Background(), 9, 0, domain.KindFollow, domain.Page{})
	if err != nil {
		t.Fatalf("列关注失败: %v", err)
	}
	if len(items) != 1 || items[0].Nickname != "被关注者" || items[0].Avatar != "b.png" {
		t.Fatalf("关注视图应取被关注者那一端，实际 %+v", items)
	}

	// 粉丝视图：对方是粉丝。同一行数据必须换另一端。
	items, _, err = svc.ListRelations(context.Background(), 9, 0, domain.KindFans, domain.Page{})
	if err != nil {
		t.Fatalf("列粉丝失败: %v", err)
	}
	if len(items) != 1 || items[0].Nickname != "粉丝" || items[0].Avatar != "f.png" {
		t.Fatalf("粉丝视图应取粉丝那一端，实际 %+v", items)
	}

	// kind 必须原样传到仓储：互关的过滤发生在仓储（SQL）里，用例层不许自己筛。
	if _, _, err := svc.ListRelations(context.Background(), 9, 0, domain.KindMutual, domain.Page{}); err != nil {
		t.Fatalf("列互关失败: %v", err)
	}
	if repo.lastKind != domain.KindMutual {
		t.Fatalf("kind 没有传到仓储，实际 %q", repo.lastKind)
	}
}

func TestListRelationsHydratesMissingSnapshots(t *testing.T) {
	row := bothSides(200)
	row.FollowedNickname = ""
	row.FollowedAvatar = ""
	repo := &fakeRepo{rows: []domain.Follow{row}}
	users := &fakeUsers{displays: map[uint64]domain.Display{
		200: {Nickname: "补出来的昵称", Avatar: "h.png"},
	}}
	svc := newService(repo, &fakeStats{}, users, &fakePrivacy{followVisible: true})

	items, _, err := svc.ListRelations(context.Background(), 9, 0, domain.KindFollow, domain.Page{})
	if err != nil {
		t.Fatalf("列关注失败: %v", err)
	}
	if items[0].Nickname != "补出来的昵称" || items[0].Avatar != "h.png" {
		t.Fatalf("空快照应由展示读补齐，实际 %+v", items)
	}
}

func TestListRelationsUsesPerViewVisibility(t *testing.T) {
	// 看别人的列表：关注开关关、粉丝开关开。
	repo := &fakeRepo{rows: []domain.Follow{}}
	privacy := &fakePrivacy{followVisible: false, fansVisible: true}
	svc := newService(repo, &fakeStats{}, &fakeUsers{}, privacy)

	_, _, err := svc.ListRelations(context.Background(), 9, 300, domain.KindFollow, domain.Page{})
	wantCode(t, err, apperr.CodeFollowListPrivate)

	// 粉丝视图用的是粉丝开关（这里开着），所以不报错。
	if _, _, err := svc.ListRelations(context.Background(), 9, 300, domain.KindFans, domain.Page{}); err != nil {
		t.Fatalf("粉丝视图应放行: %v", err)
	}

	// 互关 ⊆ 关注，所以互关视图沿用关注开关。
	privacy.followVisible = false
	_, _, err = svc.ListRelations(context.Background(), 9, 300, domain.KindMutual, domain.Page{})
	wantCode(t, err, apperr.CodeFollowListPrivate)

	// 看自己的列表不走可见性判断。
	privacy.followVisible = false
	if _, _, err := svc.ListRelations(context.Background(), 9, 9, domain.KindFollow, domain.Page{}); err != nil {
		t.Fatalf("看自己的列表不该被可见性拦住: %v", err)
	}
}

func TestCountRelationsHonoursVisibility(t *testing.T) {
	repo := &fakeRepo{counts: domain.RelationCounts{Follow: 3, Fans: 2, Mutual: 1}}
	privacy := &fakePrivacy{followVisible: true}

	svc := newService(repo, &fakeStats{}, &fakeUsers{}, privacy)
	counts, err := svc.CountRelations(context.Background(), 9, 0)
	if err != nil {
		t.Fatalf("取计数失败: %v", err)
	}
	if counts.Follow != 3 || counts.Fans != 2 || counts.Mutual != 1 {
		t.Fatalf("计数不该被改写，实际 %+v", counts)
	}

	// 别人的列表不可见时，计数也不给：数字同样是关系规模。
	privacy.followVisible = false
	_, err = svc.CountRelations(context.Background(), 9, 300)
	wantCode(t, err, apperr.CodeFollowListPrivate)
}

func TestRemoveFanDeletesOnlyIncomingRelation(t *testing.T) {
	repo := &fakeRepo{unfollowFound: true}
	stats := &fakeStats{}
	svc := newService(repo, stats, &fakeUsers{}, &fakePrivacy{})

	if err := svc.RemoveFan(context.Background(), 9, 300); err != nil {
		t.Fatalf("移除粉丝失败: %v", err)
	}

	// 删的是"300 关注我(9)"这一条：followed=我、fans=他。
	if len(repo.unfollowed) != 1 || repo.unfollowed[0] != [2]uint64{9, 300} {
		t.Fatalf("移除粉丝应删 followed=我 fans=他，实际 %v", repo.unfollowed)
	}

	// 统计按同一方向回滚：他的关注数 -1、我的粉丝数 -1。
	if len(stats.deltas) != 1 {
		t.Fatalf("应且只回写一次统计，实际 %v", stats.deltas)
	}
	if got := stats.deltas[0]; got.fansID.Uint64() != 300 || got.followedID.Uint64() != 9 || got.delta != -1 {
		t.Fatalf("统计应为 fans=300 followed=9 delta=-1，实际 %+v", got)
	}
}

func TestRemoveFanRejectsUnknownAndSelf(t *testing.T) {
	repo := &fakeRepo{unfollowFound: false}
	svc := newService(repo, &fakeStats{}, &fakeUsers{}, &fakePrivacy{})

	// 他本来就不是我的粉丝。
	wantCode(t, svc.RemoveFan(context.Background(), 9, 300), apperr.CodeFollowFanNotFound)
	// 匿名。
	wantCode(t, svc.RemoveFan(context.Background(), 0, 300), apperr.CodeAuthRequired)
	// 移除自己。
	wantCode(t, svc.RemoveFan(context.Background(), 9, 9), apperr.CodeFollowSelf)
}

func TestRemoveFanTreatsStatsFailureAsSideEffect(t *testing.T) {
	repo := &fakeRepo{unfollowFound: true}
	svc := app.New(repo, failingStats{}, &fakeUsers{}, &fakePrivacy{}, nil)

	// 统计写失败不能把已经成立的移除判成失败：关系那一步已经落库了。
	if err := svc.RemoveFan(context.Background(), 9, 300); err != nil {
		t.Fatalf("统计失败不该让移除失败: %v", err)
	}
}

type failingStats struct{}

func (failingStats) ApplyFollowDelta(context.Context, uint64, uint64, int) error {
	return errors.New("stat write failed")
}
