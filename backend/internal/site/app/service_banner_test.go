package app_test

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/app"
	"myblogx/internal/site/domain"
)

func TestListBannersFiltersByOptionalShow(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/shown.png")
	env.Images.Put(12, "/hidden.png")

	if _, err := env.Service.CreateBanner(ctx, apptestInput(11, "/shown", true)); err != nil {
		t.Fatalf("创建展示中的轮播图失败: %v", err)
	}
	if _, err := env.Service.CreateBanner(ctx, apptestInput(12, "/hidden", false)); err != nil {
		t.Fatalf("创建隐藏轮播图失败: %v", err)
	}

	shown := true
	hidden := false
	cases := []struct {
		name       string
		show       *bool
		wantShown  int
		wantHidden int
	}{
		{name: "不传时不过滤", show: nil, wantShown: 1, wantHidden: 1},
		{name: "传真时只看展示中", show: &shown, wantShown: 1},
		{name: "传假时只看隐藏", show: &hidden, wantHidden: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, hasMore, err := env.Service.ListBanners(ctx, domain.BannerQuery{Show: tc.show})
			if err != nil {
				t.Fatalf("查轮播图失败: %v", err)
			}
			if hasMore {
				t.Fatal("两条种子数据不应有下一页")
			}
			var gotShown, gotHidden int
			for _, row := range rows {
				if row.Show {
					gotShown++
				} else {
					gotHidden++
				}
			}
			if gotShown != tc.wantShown || gotHidden != tc.wantHidden {
				t.Fatalf("筛选结果错误: 展示中=%d 隐藏=%d", gotShown, gotHidden)
			}
		})
	}
}

func TestBannerLifecycle(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	env.Images.Put(12, "/b.png")

	created, err := env.Service.CreateBanner(ctx, apptestInput(11, "/a", true))
	if err != nil {
		t.Fatalf("创建轮播图失败: %v", err)
	}
	if created.Cover != "/a.png" || created.ID.IsZero() {
		t.Fatalf("创建结果错误: %+v", created)
	}

	if _, err := env.Service.CreateBanner(ctx, apptestInput(12, "/b", true)); err != nil {
		t.Fatalf("创建第二张轮播图失败: %v", err)
	}

	rows, hasMore, err := env.Service.ListBanners(ctx, domain.BannerQuery{Show: boolFilter(true)})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 2 || hasMore {
		t.Fatalf("展示中的轮播图应有 2 条，得到 %d hasMore=%v", len(rows), hasMore)
	}
	if rows[0].CoverImageID.IsZero() {
		t.Fatalf("列表应回填封面图片 ID: %+v", rows[0])
	}

	// 隐藏后不再出现在 show=true 的列表里。
	if err := env.Service.UpdateBanner(ctx, created.ID.Uint64(), apptestInput(12, "/b2", false)); err != nil {
		t.Fatalf("更新轮播图失败: %v", err)
	}
	rows, _, err = env.Service.ListBanners(ctx, domain.BannerQuery{Show: boolFilter(true)})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("隐藏后应只剩 1 条展示中的，得到 %d", len(rows))
	}

	n, err := env.Service.RemoveBanners(ctx, []uint64{created.ID.Uint64()})
	if err != nil || n != 1 {
		t.Fatalf("删除轮播图失败: n=%d err=%v", n, err)
	}
	if len(env.Banners.All()) != 1 {
		t.Fatalf("删除后应剩 1 条，得到 %d", len(env.Banners.All()))
	}
}

func TestBannerRejectsUnavailableImage(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	env.Images.MarkUnusable(11)

	if _, err := env.Service.CreateBanner(ctx, apptestInput(11, "/a", true)); !apperr.IsCode(err, apperr.CodeImageUnavailable) {
		t.Fatalf("不可用图片应报错，得到 %v", err)
	}
	if _, err := env.Service.CreateBanner(ctx, apptestInput(0, "/a", true)); !apperr.IsCode(err, apperr.CodeImageUnavailable) {
		t.Fatalf("缺少封面应报错，得到 %v", err)
	}
}

func TestBannerUpdateMissing(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")
	if err := env.Service.UpdateBanner(ctx, 999, apptestInput(11, "/a", true)); !apperr.IsCode(err, apperr.CodeBannerNotFound) {
		t.Fatalf("不存在的轮播图应报错，得到 %v", err)
	}
}

// apptestInput 构造轮播图入参。
func apptestInput(imageID uint64, href string, show bool) app.BannerInput {
	return app.BannerInput{CoverImageID: ctype.NewID(imageID), Href: href, Show: show}
}

func boolFilter(value bool) *bool { return &value }

// ── 轮播标题：纯文本、可空、可编辑 ──────────────────────────────

// TestCreateBannerWithoutTitleIsAccepted 钉住"标题可空、不传不报错"。
func TestCreateBannerWithoutTitleIsAccepted(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")

	created, err := env.Service.CreateBanner(ctx, apptestInput(11, "/a", true))
	if err != nil {
		t.Fatalf("不带标题创建应成功: %v", err)
	}
	if created.Title != "" {
		t.Fatalf("不带标题时应存空串，得到 %q", created.Title)
	}
}

// TestBannerTitleStaysPlainText 钉住"只存纯文本"：HTML 标签原样存，不解析不剥离。
func TestBannerTitleStaysPlainText(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")

	raw := `<b>粗体</b> & "引号"`
	in := apptestInput(11, "/a", true)
	in.Title = raw
	created, err := env.Service.CreateBanner(ctx, in)
	if err != nil {
		t.Fatalf("带 HTML 标签的标题应接受: %v", err)
	}
	if created.Title != raw {
		t.Fatalf("标题应原样保存 %q，得到 %q", raw, created.Title)
	}
}

// TestBannerTitleTrimsAndCapsToColumnWidth 钉住落库形状：首尾空白去掉、
// 超长按字符截到 varchar(256)，避免写库时炸列宽。
func TestBannerTitleTrimsAndCapsToColumnWidth(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")

	padded := apptestInput(11, "/a", true)
	padded.Title = "  带空白标题  "
	created, err := env.Service.CreateBanner(ctx, padded)
	if err != nil {
		t.Fatalf("带空白的标题应成功: %v", err)
	}
	if created.Title != "带空白标题" {
		t.Fatalf("首尾空白应去掉，得到 %q", created.Title)
	}

	long := apptestInput(11, "/b", true)
	long.Title = strings.Repeat("好", domain.MaxTitleLength+10)
	capped, err := env.Service.CreateBanner(ctx, long)
	if err != nil {
		t.Fatalf("超长标题不该报错（按列宽截断）: %v", err)
	}
	if runes := []rune(capped.Title); len(runes) != domain.MaxTitleLength {
		t.Fatalf("超长标题应截到 %d 字，得到 %d", domain.MaxTitleLength, len(runes))
	}
}

// TestUpdateBannerTitle 钉住"可编辑已有轮播的标题"，含清空。
func TestUpdateBannerTitle(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	env.Images.Put(11, "/a.png")

	first := apptestInput(11, "/a", true)
	first.Title = "旧标题"
	created, err := env.Service.CreateBanner(ctx, first)
	if err != nil {
		t.Fatalf("创建轮播图失败: %v", err)
	}

	next := apptestInput(11, "/a", true)
	next.Title = "新标题"
	if err := env.Service.UpdateBanner(ctx, created.ID.Uint64(), next); err != nil {
		t.Fatalf("改标题失败: %v", err)
	}
	rows, _, err := env.Service.ListBanners(ctx, domain.BannerQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "新标题" {
		t.Fatalf("列表应回传新标题，得到 %+v", rows)
	}

	cleared := apptestInput(11, "/a", true)
	if err := env.Service.UpdateBanner(ctx, created.ID.Uint64(), cleared); err != nil {
		t.Fatalf("清空标题失败: %v", err)
	}
	rows, _, err = env.Service.ListBanners(ctx, domain.BannerQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "" {
		t.Fatalf("清空后标题应为空串，得到 %+v", rows)
	}
}

// ── 新增的五个配置段 ────────────────────────────────────────────
