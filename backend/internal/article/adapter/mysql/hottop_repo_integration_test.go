//go:build integration

package mysql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/schema"
)

// 本文件是热门置顶（article_hot_top_models）的持久化契约。
//
// 只跑真 MySQL：这里要验的正是 SQL 行为——软删行能恢复、序号压实、
// JOIN 过滤失效行、唯一索引挡住重复。SQLite 上这些行为不一样（假 Safe）。

var hotTopBaseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// seedHotTopArticle 插入一篇置顶候选文章（作者 100）。
//
// 直接写行而不是走 app 用例：本文件测仓储，种子数据的意图要一眼可见
// （哪个状态、哪个可见性），走用例反而要反推作者/审核链路。
func seedHotTopArticle(t *testing.T, db *gorm.DB, id uint64, status schema.ArticleStatus, visibility schema.ArticleVisibility, categoryID *ctype.ID) {
	t.Helper()
	seedHotTopArticleBy(t, db, id, 100, status, visibility, categoryID)
}

// seedHotTopArticleBy 同 seedHotTopArticle，但指定作者（用来验"只出管理员作者"）。
func seedHotTopArticleBy(t *testing.T, db *gorm.DB, id, authorID uint64, status schema.ArticleStatus, visibility schema.ArticleVisibility, categoryID *ctype.ID) {
	t.Helper()
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:             id,
		Title:          "置顶候选 " + itoa(id),
		Abstract:       "摘要" + itoa(id),
		Content:        "正文",
		CategoryID:     categoryID,
		AuthorID:       authorID,
		ViewCount:      int(id * 10),
		PublishStatus:  status,
		Visibility:     visibility,
		CommentsToggle: true,
		CreatedAt:      hotTopBaseTime,
		UpdatedAt:      hotTopBaseTime,
	})
}

// itoa 把测试用的数值 ID 写成可读标题。
func itoa(v uint64) string { return strconv.FormatUint(v, 10) }

// hotTopRowCount 统计表里的**全部**行（含软删），用来证明"不产生重复行"。
func hotTopRowCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Table("article_hot_top_models").Count(&n).Error; err != nil {
		t.Fatalf("统计置顶行失败: %v", err)
	}
	return n
}

// TestHotTopRepo_SaveAppendsAndDedupes 新置顶排末尾；重复设置不产生第二行。
func TestHotTopRepo_SaveAppendsAndDedupes(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	seedHotTopArticle(t, db, 1, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 2, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()

	for i, id := range []uint64{1, 2} {
		created, err := repo.Save(ctx, ctype.NewID(id))
		if err != nil {
			t.Fatalf("第 %d 条置顶失败: %v", i+1, err)
		}
		if !created {
			t.Fatalf("第 %d 条应是新增", i+1)
		}
	}

	// 重复设置：返回 false（无变化），行数不变。
	created, err := repo.Save(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("重复置顶不应报错: %v", err)
	}
	if created {
		t.Fatal("重复置顶不应算新增")
	}
	if n := hotTopRowCount(t, db); n != 2 {
		t.Fatalf("应只有 2 行置顶，实际 %d", n)
	}

	ids, err := repo.ListIDs(ctx)
	if err != nil {
		t.Fatalf("读置顶 ID 失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != ctype.NewID(1) || ids[1] != ctype.NewID(2) {
		t.Fatalf("新置顶应排在末尾，实际 %v", ids)
	}

	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(items) != 2 || items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应为 1,2，实际 %+v", items)
	}
}

// TestHotTopRepo_DeleteCompactsOrder 取消一条后剩余序号压实成 1..n。
func TestHotTopRepo_DeleteCompactsOrder(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	for i := uint64(1); i <= 3; i++ {
		seedHotTopArticle(t, db, i, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	}

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	for i := uint64(1); i <= 3; i++ {
		if _, err := repo.Save(ctx, ctype.NewID(i)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", i, err)
		}
	}

	// 取消中间那条。
	if err := repo.Delete(ctx, ctype.NewID(2)); err != nil {
		t.Fatalf("取消置顶失败: %v", err)
	}

	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应剩 2 条，实际 %d", len(items))
	}
	if items[0].ArticleID != ctype.NewID(1) || items[0].SortOrder != 1 {
		t.Fatalf("第一条应为文章 1 / 序号 1，实际 %+v", items[0])
	}
	if items[1].ArticleID != ctype.NewID(3) || items[1].SortOrder != 2 {
		t.Fatalf("第二条应为文章 3 / 序号 2（序号压实），实际 %+v", items[1])
	}

	// 幂等：再取消一次不报错。
	if err := repo.Delete(ctx, ctype.NewID(2)); err != nil {
		t.Fatalf("重复取消应幂等，实际 %v", err)
	}

	// Count 只数未软删的行：软删行留在表里（3 行），但当前只有 2 条置顶。
	if n, err := repo.Count(ctx); err != nil || n != 2 {
		t.Fatalf("Count 应为 2，实际 %d (err=%v)", n, err)
	}
	if n := hotTopRowCount(t, db); n != 3 {
		t.Fatalf("表里应有 3 行（含软删），实际 %d", n)
	}
}

// TestHotTopRepo_ResurrectsSoftDeletedRow 取消后再置顶恢复原行，序号排末尾。
//
// 为什么必须有这条：article_id 上有唯一索引，软删行仍占着那个值。
// 如果重新置顶走 INSERT，会撞 1062（而前端看到的只是"设置失败"）。
func TestHotTopRepo_ResurrectsSoftDeletedRow(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	seedHotTopArticle(t, db, 1, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 2, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	if _, err := repo.Save(ctx, ctype.NewID(1)); err != nil {
		t.Fatalf("置顶 1 失败: %v", err)
	}
	if err := repo.Delete(ctx, ctype.NewID(1)); err != nil {
		t.Fatalf("取消置顶失败: %v", err)
	}

	if created, err := repo.Save(ctx, ctype.NewID(2)); err != nil || !created {
		t.Fatalf("置顶 2 失败: created=%v err=%v", created, err)
	}
	created, err := repo.Save(ctx, ctype.NewID(1))
	if err != nil {
		t.Fatalf("重新置顶软删过的文章失败: %v", err)
	}
	if !created {
		t.Fatal("重新置顶应算新增")
	}
	if n := hotTopRowCount(t, db); n != 2 {
		t.Fatalf("重新置顶不应产生第三行，实际 %d 行", n)
	}

	ids, err := repo.ListIDs(ctx)
	if err != nil {
		t.Fatalf("读置顶 ID 失败: %v", err)
	}
	if len(ids) != 2 || ids[0] != ctype.NewID(2) || ids[1] != ctype.NewID(1) {
		t.Fatalf("恢复的行应排末尾，实际 %v", ids)
	}

	// 序号必须从 1 连续：软删行仍占着 sort_order 的话，恢复后会留下空洞。
	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if items[0].SortOrder != 1 || items[1].SortOrder != 2 {
		t.Fatalf("序号应压实为 1,2，实际 %d,%d", items[0].SortOrder, items[1].SortOrder)
	}
}

// TestHotTopRepo_ListCardsSkipsStale 公开列表只含仍公开的文章，按序号升序。
func TestHotTopRepo_ListCardsSkipsStale(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	categoryID := ctype.NewID(500)
	schema.InsertCategory(t, db, 500, 100, "技术", hotTopBaseTime)

	// 1 在架；2 草稿；3 作者隐藏；4 已软删；5 审核中。
	seedHotTopArticle(t, db, 1, schema.ArticleStatusPublished, schema.VisibilityVisible, &categoryID)
	seedHotTopArticle(t, db, 2, schema.ArticleStatusDraft, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 3, schema.ArticleStatusPublished, schema.VisibilityUserHidden, nil)
	seedHotTopArticle(t, db, 4, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 5, schema.ArticleStatusExamining, schema.VisibilityVisible, nil)

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	for i := uint64(1); i <= 5; i++ {
		if _, err := repo.Save(ctx, ctype.NewID(i)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", i, err)
		}
	}
	// 软删第 4 篇：置顶行保留，首页必须过滤掉。
	if err := db.Table("article_models").Where("id = ?", 4).Update("deleted_at", hotTopBaseTime).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	cards, err := repo.ListCards(ctx, false)
	if err != nil {
		t.Fatalf("读公开卡片失败: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("只有 1 篇仍公开，实际返回 %d 张卡片", len(cards))
	}
	card := cards[0]
	if card.ArticleID != ctype.NewID(1) || card.Title != "置顶候选 1" {
		t.Fatalf("卡片不符: %+v", card)
	}
	if card.CategoryTitle != "技术" || card.AuthorNickname != "用户100" || card.ViewCount != 10 {
		t.Fatalf("卡片 join 字段不符: %+v", card)
	}

	// 后台列表仍包含全部 5 行 —— 否则没法清理失效置顶。
	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("后台列表应含全部 5 行（含失效），实际 %d", len(items))
	}
}

// TestHotTopRepo_ListCardsFiltersNonAdminAuthor 「显示非管理员发布文章」关闭时
// 公开卡片只剩管理员作者的文章。
//
// 热门置顶是读者端公开读面（ADR-0017 决策 4），所以它不能只过滤"文章还在不在"：
// 作者不是管理员时，即使文章仍"已发布 + visible"，首页运营位也不该出现它。
// 没有这一条，关掉开关后首页仍会把被隐藏的文章顶在最上面。
func TestHotTopRepo_ListCardsFiltersNonAdminAuthor(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100) // 普通作者
	schema.InsertUser(t, db, schema.UserSpec{
		ID:       900,
		Username: "hot_top_admin",
		Nickname: "管理员作者",
		Role:     schema.RoleAdmin,
	})

	// 两篇都是"已发布 + 可见"：唯一差别在作者角色。
	seedHotTopArticleBy(t, db, 1, 100,
		schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	seedHotTopArticleBy(t, db, 2, 900,
		schema.ArticleStatusPublished, schema.VisibilityVisible, nil)

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	for i := uint64(1); i <= 2; i++ {
		if _, err := repo.Save(ctx, ctype.NewID(i)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", i, err)
		}
	}

	// 开关打开：两篇都出，顺序不变（与改造前一致）。
	all, err := repo.ListCards(ctx, false)
	if err != nil {
		t.Fatalf("开关打开时读公开卡片失败: %v", err)
	}
	if len(all) != 2 || all[0].ArticleID != ctype.NewID(1) || all[1].ArticleID != ctype.NewID(2) {
		t.Fatalf("开关打开时应返回 [1 2]，实际 %+v", all)
	}

	// 开关关闭：只剩管理员作者那篇，且它的 join 字段照旧。
	onlyAdmin, err := repo.ListCards(ctx, true)
	if err != nil {
		t.Fatalf("开关关闭时读公开卡片失败: %v", err)
	}
	if len(onlyAdmin) != 1 {
		t.Fatalf("开关关闭时应只剩管理员作者那篇，实际 %+v", onlyAdmin)
	}
	card := onlyAdmin[0]
	if card.ArticleID != ctype.NewID(2) || card.AuthorNickname != "管理员作者" {
		t.Fatalf("保留下来的卡片不符: %+v", card)
	}

	// 后台列表不受开关影响：运营要能看见并清理全部置顶行。
	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("后台列表应仍是 2 行，实际 %d", len(items))
	}
}

// TestHotTopRepo_ListAdminItemsMarksStale 后台能报出「已失效」及原因。
func TestHotTopRepo_ListAdminItemsMarksStale(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	seedHotTopArticle(t, db, 1, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 2, schema.ArticleStatusDraft, schema.VisibilityVisible, nil)
	seedHotTopArticle(t, db, 3, schema.ArticleStatusPublished, schema.VisibilityAdminHidden, nil)

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	for i := uint64(1); i <= 3; i++ {
		if _, err := repo.Save(ctx, ctype.NewID(i)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", i, err)
		}
	}

	items, err := repo.ListAdminItems(ctx)
	if err != nil {
		t.Fatalf("读后台列表失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("应 3 行，实际 %d", len(items))
	}

	if items[0].Stale() {
		t.Fatalf("已发布+可见的文章不应失效: %+v", items[0])
	}
	if items[0].Title != "置顶候选 1" {
		t.Fatalf("后台行应带标题，实际 %q", items[0].Title)
	}

	wantStates := []struct {
		status     schema.ArticleStatus
		visibility schema.ArticleVisibility
	}{
		{schema.ArticleStatusDraft, schema.VisibilityVisible},
		{schema.ArticleStatusPublished, schema.VisibilityAdminHidden},
	}
	for i, want := range wantStates {
		item := items[i+1]
		if !item.Stale() {
			t.Fatalf("第 %d 行应判为失效: %+v", i+2, item)
		}
		if item.Article == nil {
			t.Fatalf("第 %d 行的文章状态不应为 nil（文章还在，只是不公开）", i+2)
		}
		if item.Article.Status != domain.Status(want.status) ||
			item.Article.Visibility != domain.Visibility(want.visibility) {
			t.Fatalf("第 %d 行应带上文章当前状态（%v/%v），实际 %+v",
				i+2, want.status, want.visibility, item.Article)
		}
	}
}

// TestHotTopRepo_UpdateSortOrder 全量重排后按新序号读出。
func TestHotTopRepo_UpdateSortOrder(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedUser(t, db, 100)
	for i := uint64(1); i <= 3; i++ {
		seedHotTopArticle(t, db, i, schema.ArticleStatusPublished, schema.VisibilityVisible, nil)
	}

	repo := mysql.NewHotTopRepo(db)
	ctx := context.Background()
	for i := uint64(1); i <= 3; i++ {
		if _, err := repo.Save(ctx, ctype.NewID(i)); err != nil {
			t.Fatalf("置顶 %d 失败: %v", i, err)
		}
	}

	if err := repo.UpdateSortOrder(ctx, []domain.ArticleID{
		domain.ArticleID(ctype.NewID(3)),
		domain.ArticleID(ctype.NewID(1)),
		domain.ArticleID(ctype.NewID(2)),
	}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}

	ids, err := repo.ListIDs(ctx)
	if err != nil {
		t.Fatalf("读置顶 ID 失败: %v", err)
	}
	want := []domain.ArticleID{
		domain.ArticleID(ctype.NewID(3)),
		domain.ArticleID(ctype.NewID(1)),
		domain.ArticleID(ctype.NewID(2)),
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("重排后应为 %v，实际 %v", want, ids)
		}
	}

	if n, err := repo.Count(ctx); err != nil || n != 3 {
		t.Fatalf("Count 应为 3，实际 %d (err=%v)", n, err)
	}
}
