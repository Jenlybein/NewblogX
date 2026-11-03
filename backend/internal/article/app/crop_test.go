package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	sitedomain "myblogx/internal/site/domain"
)

// 取景参数体系已在 #203 全量退场：本模块不再写、也不再读取景参数。
// 唯一留下来的取景表行为是"删文章时物理清掉它的孤儿行"——表本身留到 #205
// 才物理下线，期间这段清理不该悄悄消失。

func TestDeleteRemovesCoverCrop(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1004), author, domain.StatusDraft)
	repos.FakeCrops().Seed(sitedomain.CropTargetArticle, a.ID().Uint64())

	if _, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: a.ID()}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if repos.FakeCrops().Has(sitedomain.CropTargetArticle, a.ID().Uint64()) {
		t.Fatal("删除文章应一并清掉它的取景行")
	}
}

// 清孤儿行只碰文章主体的那一行：通用表按 (target_type, target_id) 分主体，
// 同一个数值 ID 的轮播行不该被文章删除顺手抹掉。
func TestDeleteRemovesOnlyArticleCropRows(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	a := seedArticle(t, repos, ctype.NewID(1005), author, domain.StatusDraft)
	repos.FakeCrops().Seed(sitedomain.CropTargetArticle, a.ID().Uint64())
	repos.FakeCrops().Seed(sitedomain.CropTargetBanner, a.ID().Uint64())

	if _, err := svc.Delete(ctx, app.DeleteCmd{Actor: authorViewer(), ArticleID: a.ID()}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if repos.FakeCrops().Has(sitedomain.CropTargetArticle, a.ID().Uint64()) {
		t.Fatal("删除文章应清掉 target_type=article 的取景行")
	}
	if !repos.FakeCrops().Has(sitedomain.CropTargetBanner, a.ID().Uint64()) {
		t.Fatal("删除文章不该碰 target_type=banner 的取景行")
	}
}
