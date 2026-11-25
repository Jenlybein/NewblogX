package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// 处置动作（票 398 / ADR-0032 决策 7）要钉的是四件事：
//   - 删除时先删内容、再改记录状态（顺序反了会留下"记录说删了、内容还在"）；
//   - 已处置的记录再点一次是幂等的，不会删第二次；
//   - 私信面没有可删实体，delete 明确报错而不是静默成功；
//   - 备注超长在动手之前被拒（不截断、不静默丢）。

// handleRepo 是带处置能力的仓储替身：在 fakeRepo 之上补状态流转。
type handleRepo struct {
	fakeRepo
	row    domain.Hit
	found  bool
	marked []domain.Handle
	// updated 为 false 模拟"并发下已被别人先处置"。
	updated   bool
	markErr   error
	findErr   error
	updateErr error
}

func (r *handleRepo) FindByID(_ context.Context, id ctype.ID) (domain.Hit, error) {
	if r.findErr != nil {
		return domain.Hit{}, r.findErr
	}
	if !r.found || r.row.ID != id {
		return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
	}
	// 返回副本：用例层改了 hit 的字段不该回写"库里的行"。
	return r.row, nil
}

func (r *handleRepo) MarkHandled(_ context.Context, _ ctype.ID, h domain.Handle) (bool, error) {
	if r.markErr != nil {
		return false, r.markErr
	}
	r.marked = append(r.marked, h)
	if r.updated {
		r.row.Status = h.Status
		r.row.HandlerID = h.HandlerID
		r.row.HandledAt = h.HandledAt
		r.row.Note = h.Note
	}
	return r.updated, r.updateErr
}

// fakeDisposer 记录"要删什么"，并可以让某次删除失败。
type fakeDisposer struct {
	deletedArticles    []ctype.ID
	deletedComments    []ctype.ID
	deletedCollections []ctype.ID
	resetProfiles      []ctype.ID
	resetNicknames     []string
	failErr            error
}

func (d *fakeDisposer) DeleteArticle(_ context.Context, _, id ctype.ID) error {
	d.deletedArticles = append(d.deletedArticles, id)
	return d.failErr
}

func (d *fakeDisposer) DeleteComment(_ context.Context, _, id ctype.ID) error {
	d.deletedComments = append(d.deletedComments, id)
	return d.failErr
}

func (d *fakeDisposer) DeleteCollection(_ context.Context, _, id ctype.ID) error {
	d.deletedCollections = append(d.deletedCollections, id)
	return d.failErr
}

func (d *fakeDisposer) ResetProfile(_ context.Context, _, userID ctype.ID, nickname string) error {
	d.resetProfiles = append(d.resetProfiles, userID)
	d.resetNicknames = append(d.resetNicknames, nickname)
	return d.failErr
}

// newHandleService 构造一个带处置端口的服务，并预置一条待处理记录。
func newHandleService(t *testing.T, surface domain.Surface, contentID ctype.ID) (*app.Service, *handleRepo, *fakeDisposer) {
	t.Helper()
	repo := &handleRepo{
		row: domain.Hit{
			ID:        ctype.NewID(700),
			Surface:   surface,
			ContentID: contentID,
			UserID:    ctype.NewID(900),
			Status:    domain.StatusPending,
			Words:     []domain.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		},
		found:   true,
		updated: true,
	}
	disposer := &fakeDisposer{}
	svc := app.New(nil, repo, &fakeLogger{}).WithDisposer(disposer)
	return svc, repo, disposer
}

func TestHandleDeleteRemovesContentThenMarksHit(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	got, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		Note:      "确认违规",
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("处置应成功: %v", err)
	}
	if len(disposer.deletedArticles) != 1 || disposer.deletedArticles[0].Uint64() != 42 {
		t.Fatalf("应删除命中记录指向的文章，实际 %v", disposer.deletedArticles)
	}
	if got.Hit.Status != domain.StatusDeleted {
		t.Fatalf("记录状态应为已删除，实际 %s", got.Hit.Status)
	}
	if got.Hit.HandlerID.Uint64() != 1 || got.Hit.Note != "确认违规" {
		t.Fatalf("处置痕迹没写全: handler=%v note=%q", got.Hit.HandlerID, got.Hit.Note)
	}
}

func TestHandleIgnoreKeepsContent(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	if _, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionIgnore,
		HandlerID: ctype.NewID(1),
	}); err != nil {
		t.Fatalf("忽略应成功: %v", err)
	}
	if len(disposer.deletedArticles) != 0 {
		t.Fatalf("忽略不该删内容，实际删了 %v", disposer.deletedArticles)
	}
}

func TestHandleIsIdempotentOnHandledRecord(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	repo.row.Status = domain.StatusDeleted

	got, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("重复处置应返回当前状态而不是报错: %v", err)
	}
	if !got.AlreadyHandled {
		t.Fatal("应标记为已处置")
	}
	if len(disposer.deletedArticles) != 0 {
		t.Fatalf("已处置的记录不该再删一次，实际 %v", disposer.deletedArticles)
	}
}

func TestHandleChatSurfaceRejectsDelete(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newHandleService(t, domain.SurfaceChat, ctype.NewID(42))
	_, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	})
	if err == nil {
		t.Fatal("私信面没有可删实体，delete 应报错而不是静默成功")
	}
	if !apperr.IsCode(err, apperr.CodeFilterSurfaceNoContent) {
		t.Fatalf("错误码应为 filter.surface_no_content，实际 %v", err)
	}
}

func TestHandleRejectsTooLongNoteBeforeTouchingContent(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	long := make([]rune, domain.MaxNoteLen+1)
	for i := range long {
		long[i] = '备'
	}
	_, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		Note:      string(long),
		HandlerID: ctype.NewID(1),
	})
	if err == nil {
		t.Fatal("超长备注应在动手之前被拒")
	}
	if !apperr.IsCode(err, apperr.CodeFilterNoteTooLong) {
		t.Fatalf("错误码应为 filter.note_too_long，实际 %v", err)
	}
	if len(disposer.deletedArticles) != 0 {
		t.Fatalf("参数被拒时不该删任何内容，实际 %v", disposer.deletedArticles)
	}
}

func TestHandleProfileResetUsesProblemNickname(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceNickname, ctype.NewID(900))
	if _, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	}); err != nil {
		t.Fatalf("重置资料应成功: %v", err)
	}
	// 昵称面与简介面共用同一个"资料重置"，昵称格式是「问题昵称」+ uid 末 4 位。
	want := domain.ProblemNickname(ctype.NewID(900))
	if len(disposer.resetNicknames) != 1 || disposer.resetNicknames[0] != want {
		t.Fatalf("应重置为 %q，实际 %v", want, disposer.resetNicknames)
	}
}

func TestHandleBatchReportsPerItemResults(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	disposer.failErr = errors.New("boom")
	got, err := svc.HandleBatch(context.Background(), app.BatchHandleInput{
		IDs:       []ctype.ID{repo.row.ID},
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	})
	if err != nil {
		t.Fatalf("批量处置本身应返回结果而不是整体报错: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Err == nil {
		t.Fatalf("逐条结果应如实报告失败: %+v", got.Items)
	}
}

func TestHandleBatchRejectsEmptySelection(t *testing.T) {
	t.Parallel()

	svc, _, _ := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	if _, err := svc.HandleBatch(context.Background(), app.BatchHandleInput{
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	}); err == nil {
		t.Fatal("一条都没选应在动手之前被拒")
	}
}

// 六个文本面 → 处置方法 的分派（票 398）。
//
// 这张表是"面 → 删什么"的唯一事实：接错面会让管理员删掉另一种内容。
// 隐私面（昵称 / 简介）共用同一个资料重置动作 —— spec 决策 7 的口径。
func TestHandleDispatchesSurfaceToDisposer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		surface domain.Surface
		assert  func(*testing.T, *fakeDisposer)
	}{
		{
			name:    "文章面删文章",
			surface: domain.SurfaceArticle,
			assert: func(t *testing.T, d *fakeDisposer) {
				if len(d.deletedArticles) != 1 {
					t.Fatalf("应删文章，实际 articles=%v", d.deletedArticles)
				}
			},
		},
		{
			name:    "评论面删评论",
			surface: domain.SurfaceComment,
			assert: func(t *testing.T, d *fakeDisposer) {
				if len(d.deletedComments) != 1 {
					t.Fatalf("应删评论，实际 comments=%v", d.deletedComments)
				}
			},
		},
		{
			name:    "合集面删合集",
			surface: domain.SurfaceCollection,
			assert: func(t *testing.T, d *fakeDisposer) {
				if len(d.deletedCollections) != 1 {
					t.Fatalf("应删合集，实际 collections=%v", d.deletedCollections)
				}
			},
		},
		{
			name:    "昵称面重置资料",
			surface: domain.SurfaceNickname,
			assert: func(t *testing.T, d *fakeDisposer) {
				if len(d.resetProfiles) != 1 {
					t.Fatalf("应重置资料，实际 profiles=%v", d.resetProfiles)
				}
			},
		},
		{
			name:    "简介面同样重置资料",
			surface: domain.SurfaceAbstract,
			assert: func(t *testing.T, d *fakeDisposer) {
				if len(d.resetProfiles) != 1 {
					t.Fatalf("应重置资料，实际 profiles=%v", d.resetProfiles)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, repo, disposer := newHandleService(t, tc.surface, ctype.NewID(42))
			if _, err := svc.Handle(context.Background(), app.HandleInput{
				ID:        repo.row.ID,
				Action:    domain.ActionDelete,
				HandlerID: ctype.NewID(1),
			}); err != nil {
				t.Fatalf("处置应成功: %v", err)
			}
			tc.assert(t, disposer)
		})
	}
}

// 删除失败时记录必须仍是待处理：管理员重试即可，不能留下"记录说删了、内容还在"。
func TestHandleKeepsPendingWhenDisposeFails(t *testing.T) {
	t.Parallel()

	svc, repo, disposer := newHandleService(t, domain.SurfaceArticle, ctype.NewID(42))
	disposer.failErr = errors.New("boom")
	if _, err := svc.Handle(context.Background(), app.HandleInput{
		ID:        repo.row.ID,
		Action:    domain.ActionDelete,
		HandlerID: ctype.NewID(1),
	}); err == nil {
		t.Fatal("内容删失败应向上报错")
	}
	if len(repo.marked) != 0 {
		t.Fatalf("内容删失败时不该改记录状态，实际 %+v", repo.marked)
	}
}
