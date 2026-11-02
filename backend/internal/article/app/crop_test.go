package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	sitedomain "myblogx/internal/site/domain"
)

// 文章封面取景随文章读写：写入口（创建/更新）落通用取景表，读出口（详情）
// 回填到视图；"没有取景记录"必须退化成居中、缩放 1，而不是报错或留空。

func articleCrop() sitedomain.Crop {
	return sitedomain.Crop{FocusX: 0.2, FocusY: 0.8, Zoom: 1.5}
}

func TestCreateStoresCoverCropUnderArticleTarget(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	crop := articleCrop()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "带取景的文章",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
		Crop:         &crop,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, ok := repos.FakeCrops().Get(sitedomain.CropTargetArticle, a.ID().Uint64())
	if !ok {
		t.Fatal("创建时没有写入取景记录")
	}
	if got != crop {
		t.Fatalf("取景未按提交值落库：want %+v got %+v", crop, got)
	}

	view, err := svc.Detail(ctx, a.ID(), authorViewer(), false)
	if err != nil {
		t.Fatalf("详情读取失败: %v", err)
	}
	if view.Crop != crop {
		t.Fatalf("详情没有读回取景：want %+v got %+v", crop, view.Crop)
	}
}

func TestCreateWithoutCropLeavesNoRecord(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "没有取景",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if _, ok := repos.FakeCrops().Get(sitedomain.CropTargetArticle, a.ID().Uint64()); ok {
		t.Fatal("没有提交取景时不应写取景记录")
	}
}

func TestUpdateStoresCoverCropEvenWhenOnlyCropChanges(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1001), author, domain.StatusDraft)
	crop := sitedomain.Crop{FocusX: 0.7, FocusY: 0.3, Zoom: 1.2}

	result, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: a.ID(),
		Policy:    domain.OpenWritePolicy(),
		Crop:      &crop,
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if !result.Changed {
		t.Fatal("只有取景变化时也应视为有变化，不能走空操作分支")
	}

	got, ok := repos.FakeCrops().Get(sitedomain.CropTargetArticle, a.ID().Uint64())
	if !ok {
		t.Fatal("更新时没有写入取景记录")
	}
	if got != crop {
		t.Fatalf("更新后的取景不对：want %+v got %+v", crop, got)
	}

	// 更新写进去的取景必须能在详情里读回：只写不读等于没生效。
	view, err := svc.Detail(ctx, a.ID(), authorViewer(), false)
	if err != nil {
		t.Fatalf("更新后详情读取失败: %v", err)
	}
	if view.Crop != crop {
		t.Fatalf("详情没有读回更新后的取景：want %+v got %+v", crop, view.Crop)
	}
}

func TestDetailDefaultsCropForLegacyArticle(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1002), author, domain.StatusPublished)

	view, err := svc.Detail(ctx, a.ID(), domain.Anonymous(), false)
	if err != nil {
		t.Fatalf("存量文章详情读取失败（不应报错）: %v", err)
	}
	if view.Crop != sitedomain.DefaultCrop {
		t.Fatalf("存量文章应回落到居中、缩放 1：want %+v got %+v", sitedomain.DefaultCrop, view.Crop)
	}
}

func TestCreateRejectsOutOfRangeCrop(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)
	ctx := context.Background()

	outOfRange := sitedomain.Crop{FocusX: -0.01, FocusY: 0.5, Zoom: 1}
	_, err := svc.Create(ctx, app.CreateArticleCmd{
		Actor:        authorViewer(),
		Policy:       domain.OpenWritePolicy(),
		Title:        "越界取景",
		Content:      "正文",
		TargetStatus: domain.StatusDraft,
		Crop:         &outOfRange,
	})
	if err == nil {
		t.Fatal("越界取景应在写库前被拒绝")
	}
	if !apperr.IsCode(err, apperr.CodeArticleCropInvalid) {
		t.Fatalf("越界取景应返回文章取景错误码，实际 %v", err)
	}
}

func TestUpdateRejectsOutOfRangeCrop(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1003), author, domain.StatusDraft)

	outOfRange := sitedomain.Crop{FocusX: 0.5, FocusY: 0.5, Zoom: 2.5}
	_, err := svc.Update(ctx, app.UpdateArticleCmd{
		Actor:     authorViewer(),
		ArticleID: a.ID(),
		Policy:    domain.OpenWritePolicy(),
		Crop:      &outOfRange,
	})
	if err == nil {
		t.Fatal("越界取景应在写库前被拒绝")
	}
	if !apperr.IsCode(err, apperr.CodeArticleCropInvalid) {
		t.Fatalf("越界取景应返回文章取景错误码，实际 %v", err)
	}
	if _, ok := repos.FakeCrops().Get(sitedomain.CropTargetArticle, a.ID().Uint64()); ok {
		t.Fatal("被拒绝的更新不应写入取景")
	}
}

func TestDeleteRemovesCoverCrop(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1004), author, domain.StatusDraft)
	if err := repos.FakeCrops().Upsert(ctx, sitedomain.CropTargetArticle, a.ID().Uint64(), articleCrop()); err != nil {
		t.Fatalf("预置取景失败: %v", err)
	}

	if _, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: a.ID()}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, ok := repos.FakeCrops().Get(sitedomain.CropTargetArticle, a.ID().Uint64()); ok {
		t.Fatal("删除文章应一并清掉它的取景行")
	}
}
