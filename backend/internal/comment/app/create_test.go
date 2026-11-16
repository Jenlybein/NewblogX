package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/app"
	"myblogx/internal/comment/domain"
	"myblogx/internal/kernel/ctype"
)

// 评论创建的用例（票 400：评论审核退场之后）。
//
// 这一层钉的是外部可观察行为：
//   - 发表评论**立即发布**，响应状态是已发布、计数与通知都发生；
//   - 没有任何"拦截"分支 —— 不认识的调用点也能一眼看出顺序只有
//     过滤 → 落库 → 计数/通知；
//   - 输入校验（未登录 / 空内容 / 评论关闭）仍然照旧拒绝。

func TestCreatePublishesImmediately(t *testing.T) {
	f := newFixture(t)

	result, err := f.create(t, "写得很清楚，学到了")
	if err != nil {
		t.Fatalf("发表评论不该失败: %v", err)
	}

	// 状态是**已发布**：没有「审核中」，也没有事后再改一次状态的动作。
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("评论应直接发布，实际状态 %d", result.Status)
	}
	if len(f.repo.created) != 1 {
		t.Fatalf("应有 1 条评论入库，实际 %d", len(f.repo.created))
	}
	if got := f.repo.created[0].Status; got != domain.StatusPublished {
		t.Fatalf("入库状态应是已发布，实际 %d", got)
	}
	// 发布即计入文章评论数与通知作者：这两件事过去绑在"判定通过"分支里，
	// 退场后必须无条件发生（否则评论看得见、计数却不动）。
	if f.cache.added != 1 {
		t.Fatalf("发布后应回写一次文章评论数，实际 %d", f.cache.added)
	}
	if f.notifier.count != 1 {
		t.Fatalf("发布后应通知文章作者，实际 %d 次", f.notifier.count)
	}
}

func TestCreateRejectsEmptyContent(t *testing.T) {
	f := newFixture(t)

	_, err := f.create(t, "   ")
	if !apperr.IsCode(err, apperr.CodeCommentContentEmpty) {
		t.Fatalf("空内容应返回 comment.content_empty，实际 %v", err)
	}
	if len(f.repo.created) != 0 {
		t.Fatal("被拒的评论不该入库")
	}
}

func TestCreateRejectsAnonymous(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.Create(context.Background(), domain.Viewer{}, app.CreateInput{
		Content:   "路人甲",
		ArticleID: ctype.NewID(100),
	})
	if !apperr.IsCode(err, apperr.CodeAuthRequired) {
		t.Fatalf("未登录应返回鉴权错误，实际 %v", err)
	}
}

func TestCreateRejectsClosedComments(t *testing.T) {
	f := &fixture{
		repo:     newFakeRepo(),
		cache:    &fakeCache{},
		notifier: &fakeNotifier{},
		logger:   &fakeLogger{},
	}
	f.svc = app.New(
		f.repo, fakeDiggs{}, fakeCounters{}, fakeRelations{}, fakeUsers{},
		fakeArticles{title: "关闭评论的文章", comments: false},
		f.cache, f.notifier, fakeIDs{}, f.logger,
	)

	_, err := f.create(t, "还能说话吗")
	if !apperr.IsCode(err, apperr.CodeCommentClosed) {
		t.Fatalf("评论关闭时应返回 comment.closed，实际 %v", err)
	}
}

// TestCreateReplyKeepsParentLink 守住回复链路：退场只删审核，不能顺手改坏回复。
func TestCreateReplyKeepsParentLink(t *testing.T) {
	f := newFixture(t)
	rootID := ctype.NewID(42)
	f.repo.published[rootID.Uint64()] = domain.Comment{
		ID:           rootID,
		UserID:       ctype.NewID(9),
		UserNickname: "楼主",
		ArticleID:    ctype.NewID(100),
		Status:       domain.StatusPublished,
	}

	replyID := rootID
	result, err := f.svc.Create(context.Background(), domain.Viewer{ID: ctype.NewID(7)}, app.CreateInput{
		Content:   "回复楼主",
		ArticleID: ctype.NewID(100),
		ReplyID:   &replyID,
	})
	if err != nil {
		t.Fatalf("回复不该失败: %v", err)
	}
	if result.ReplyID != rootID {
		t.Fatalf("reply_id 应指向被回复的评论 %v，实际 %v", rootID, result.ReplyID)
	}
	if result.RootID != rootID {
		t.Fatalf("回复根评论时 root_id 应指回根，实际 %v", result.RootID)
	}
	if result.Status != int8(domain.StatusPublished) {
		t.Fatalf("回复同样直接发布，实际状态 %d", result.Status)
	}
}
