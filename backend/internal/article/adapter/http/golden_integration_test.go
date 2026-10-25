//go:build integration

package http_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/testkit"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// goldenDir 是 golden 文件目录（相对本包）。
const goldenDir = "testdata/golden"

// goldenBaseTime 是所有种子行的时间基准。
//
// 写死时间而不是 time.Now()：详情响应里带 created_at / author_created_time，
// 不定死的话 golden 每次都变。用 UTC 与既有 e2e 测试保持一致。
func goldenBaseTime() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

// TestArticleGolden 验证"同一请求，重构前后输出一字不差"。
//
// 口径（S3 决策 4 + S3.10）：
//   - 写死种子：20 篇文章（ID 1001–1020）+ 6 张关联表（用户 / 分类 / 标签 /
//     文章标签 / 收藏 / 置顶 / 浏览历史 / 审核任务）的 ID、时间、计数全部硬编码；
//   - 12 个接口：详情（20 篇逐一 + 匿名 1 条）、作者信息、分类、分类下拉、
//     标签、标签下拉、收藏夹、收藏内容、置顶、浏览历史、审核任务、创建；
//   - 响应逐字节比对，不做归一化。
//
// 并发说明：这里现在可以放心并行。
// 早先 utils/markdown.MdToSafe 并发调用会偶发多输出一个前导换行（同一次运行里
// 时有时无），会让 golden 假红，所以曾临时改成串行规避。
// 该缺陷已修复：根因是 mdrenderer 把渲染状态粘在渲染器实例上、复用必然继承脏状态，
// 改为每次调用新建引擎（详见 utils/markdown/init.go），
// 并发回归测试见 utils/markdown/md_concurrency_test.go。
func TestArticleGolden(t *testing.T) {
	t.Parallel()

	db := migrate(t)
	seedGoldenDataset(t, db)

	authorA := newRouterWithIDs(t, db, domain.NormalUser(100), 100)
	authorB := newRouterWithIDs(t, db, domain.NormalUser(101), 200)
	anon := newRouterWithIDs(t, db, domain.Anonymous(), 300)
	admin := newRouterWithIDs(t, db, domain.AdminUser(900), 400)
	// 创建接口用独立 ID 起点，避免与种子文章 1001–1020 撞号。
	creator := newRouterWithIDs(t, db, domain.NormalUser(100), 2000)

	update := os.Getenv(testkit.UpdateGoldenEnv) == "1"

	type goldenCase struct {
		name   string
		router *gin.Engine
		method string
		path   string
		body   []byte
	}

	cases := make([]goldenCase, 0, 40)

	// ── 接口 1/12：详情，20 篇文章逐一（作者读自己的）──
	for i := 0; i < 20; i++ {
		router := authorA
		if i >= 15 {
			router = authorB
		}
		cases = append(cases, goldenCase{
			name:   fmt.Sprintf("detail_%02d.json", i+1),
			router: router,
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/articles/%d", 1001+i),
		})
	}
	// 匿名读一篇已发布可见的文章：验证公开可见性分支。
	cases = append(cases, goldenCase{
		name:   "detail_published_anonymous.json",
		router: anon,
		method: http.MethodGet,
		path:   "/api/articles/1003",
	})

	// ── 接口 2/12：作者信息 ──
	cases = append(cases, goldenCase{
		name:   "author_info.json",
		router: anon,
		method: http.MethodGet,
		path:   "/api/articles/author_info?author_id=100",
	})

	// ── 接口 3/12：分类列表 ──
	cases = append(cases, goldenCase{
		name:   "category_list.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/category?user_id=100",
	})

	// ── 接口 4/12：分类下拉 ──
	cases = append(cases, goldenCase{
		name:   "category_options.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/category/options?user_id=100",
	})

	// ── 接口 5/12：标签列表（默认只含启用项）──
	cases = append(cases, goldenCase{
		name:   "tags_list.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/tags",
	})

	// ── 接口 6/12：标签下拉 ──
	cases = append(cases, goldenCase{
		name:   "tags_options.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/tags/options",
	})

	// ── 接口 7/12：收藏夹列表 ──
	cases = append(cases, goldenCase{
		name:   "favorite_folders.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/favorite?user_id=100",
	})

	// ── 接口 8/12：收藏夹内容（旧契约字段名是 favorite_id）──
	cases = append(cases, goldenCase{
		name:   "favorite_contents.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/favorite/contents?favorite_id=1",
	})

	// ── 接口 9/12：置顶列表 ──
	cases = append(cases, goldenCase{
		name:   "top_list.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/top?user_id=100",
	})

	// ── 接口 10/12：浏览历史 ──
	cases = append(cases, goldenCase{
		name:   "history_list.json",
		router: authorA,
		method: http.MethodGet,
		path:   "/api/articles/history",
	})

	// ── 接口 11/12：审核任务列表（管理员）──
	cases = append(cases, goldenCase{
		name:   "review_tasks.json",
		router: admin,
		method: http.MethodGet,
		path:   "/api/article-review",
	})

	// ── 接口 12/12：创建文章（ID 起点 2000 → 返回 2001，可复现）──
	cases = append(cases, goldenCase{
		name:   "create_article.json",
		router: creator,
		method: http.MethodPost,
		path:   "/api/articles",
		body:   []byte(`{"title":"新建文章","content":"新建正文","status":1}`),
	})

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := rawRequest(t, c.router, c.method, c.path, c.body)
			testkit.CompareGolden(t, goldenDir, c.name, got, update)
		})
	}
}

// seedGoldenDataset 写入"20 篇文章 × 6 张关联表"的写死种子。
//
// 所有 ID、时间、计数都是确定的；任何一次改动都会让 golden 变红，
// 这正是它要守住的：重构不得改变响应字节。
func seedGoldenDataset(t *testing.T, db *gorm.DB) {
	t.Helper()
	base := goldenBaseTime()

	// 用户：作者 A(100) / 作者 B(101) / 管理员(900)。
	seedGoldenUser(t, db, 100, "author_a", "作者甲", "", "作者甲简介", enum.RoleUser)
	seedGoldenUser(t, db, 101, "author_b", "作者乙", "", "作者乙简介", enum.RoleUser)
	seedGoldenUser(t, db, 900, "admin_golden", "管理员", "", "", enum.RoleAdmin)

	// 分类：用户 100 名下 3 个。
	for i, title := range []string{"后端", "前端", "其他"} {
		cat := models.CategoryModel{
			Model:  models.Model{ID: ctype.ID(i + 1), CreatedAt: base, UpdatedAt: base},
			Title:  title,
			UserID: 100,
		}
		if err := db.Omit("UserModel", "ArticleList").Create(&cat).Error; err != nil {
			t.Fatalf("种子分类 %d 失败: %v", i+1, err)
		}
	}

	// 标签：2 个启用 + 1 个停用（默认列表应只出启用项）。
	tagSeeds := []struct {
		id      uint64
		title   string
		sort    int
		count   int
		enabled bool
	}{
		{1, "Go", 1, 8, true},
		{2, "MySQL", 2, 8, true},
		{3, "已停用", 3, 4, false},
	}
	for _, s := range tagSeeds {
		m := models.TagModel{
			Model:        models.Model{ID: ctype.ID(s.id), CreatedAt: base, UpdatedAt: base},
			Title:        s.title,
			Sort:         s.sort,
			IsEnabled:    s.enabled,
			ArticleCount: s.count,
			CreatedBy:    900,
		}
		if err := db.Omit("ArticleList").Create(&m).Error; err != nil {
			t.Fatalf("种子标签 %d 失败: %v", s.id, err)
		}
		// IsEnabled 带 gorm default:true，零值 false 会被数据库默认值覆盖，
		// 必须显式 UPDATE 一次，否则"停用标签"实际是启用的。
		if !s.enabled {
			if err := db.Model(&models.TagModel{}).Where("id = ?", s.id).
				Update("is_enabled", false).Error; err != nil {
				t.Fatalf("停用标签 %d 失败: %v", s.id, err)
			}
		}
	}

	// 20 篇文章：4 种状态轮转，发布态里再分可见 / 用户隐藏 / 管理员隐藏。
	for i := 0; i < 20; i++ {
		status := []enum.ArticleStatus{
			enum.ArticleStatusDraft,
			enum.ArticleStatusExamining,
			enum.ArticleStatusPublished,
			enum.ArticleStatusRejected,
		}[i%4]

		visibility := enum.ArticleVisibilityVisible
		if status == enum.ArticleStatusPublished {
			switch i {
			case 6:
				visibility = enum.ArticleVisibilityUserHidden
			case 14:
				visibility = enum.ArticleVisibilityAdminHidden
			}
		}

		author := uint64(100)
		if i >= 15 {
			author = 101
		}

		var categoryID *ctype.ID
		if i%4 != 3 {
			id := ctype.ID(i%3 + 1)
			categoryID = &id
		}

		created := base.Add(time.Duration(i) * time.Hour)
		m := models.ArticleModel{
			Model:            models.Model{ID: ctype.ID(1001 + i), CreatedAt: created, UpdatedAt: created.Add(time.Minute)},
			Title:            fmt.Sprintf("文章%02d", i+1),
			Abstract:         fmt.Sprintf("摘要%02d", i+1),
			Content:          fmt.Sprintf("正文%02d", i+1),
			CategoryID:       categoryID,
			AuthorID:         ctype.ID(author),
			ViewCount:        i * 10,
			DiggCount:        i,
			CommentCount:     i % 5,
			FavorCount:       i % 3,
			CommentsToggle:   true,
			PublishStatus:    status,
			VisibilityStatus: visibility,
		}
		if err := db.Omit("Tags", "UserModel", "CategoryModel").Create(&m).Error; err != nil {
			t.Fatalf("种子文章 %d 失败: %v", 1001+i, err)
		}

		// 文章标签：每 3 篇一组，覆盖"多标签 / 单标签 / 无标签"。
		var tagIDs []uint64
		switch i % 3 {
		case 0:
			tagIDs = []uint64{1, 2}
		case 1:
			tagIDs = []uint64{3}
		}
		for _, tagID := range tagIDs {
			// article_tag_models 是 GORM many2many 自动建的纯 join 表（无 id 列），
			// 用结构体插入会带 id 而报 Unknown column；这里只写关系两列。
			if err := db.Exec(
				"INSERT INTO article_tag_models (article_id, tag_id) VALUES (?, ?)",
				uint64(1001+i), tagID,
			).Error; err != nil {
				t.Fatalf("种子文章标签 %d/%d 失败: %v", 1001+i, tagID, err)
			}
		}
	}

	// 收藏夹：用户 100 名下 2 个（一个默认）。
	folderSeeds := []struct {
		id    uint64
		title string
		def   bool
		count int
	}{
		{1, "默认收藏夹", true, 4},
		{2, "技术收藏", false, 0},
	}
	for _, s := range folderSeeds {
		m := models.FavoriteModel{
			Model:        models.Model{ID: ctype.ID(s.id), CreatedAt: base, UpdatedAt: base},
			UserID:       100,
			Title:        s.title,
			IsDefault:    s.def,
			ArticleCount: s.count,
		}
		if err := db.Omit("UserModel", "ArticleList").Create(&m).Error; err != nil {
			t.Fatalf("种子收藏夹 %d 失败: %v", s.id, err)
		}
	}

	// 收藏关系：4 篇发布态文章放进默认收藏夹（含一篇用户隐藏，验证过滤）。
	favoriteArticleIndexes := []int{2, 6, 10, 18}
	for k, i := range favoriteArticleIndexes {
		articleID := uint64(1001 + i)
		m := models.UserArticleFavorModel{
			Model:                 models.Model{ID: ctype.ID(6000 + k), CreatedAt: base, UpdatedAt: base},
			ArticleID:             ctype.ID(articleID),
			UserID:                100,
			FavorID:               1,
			ArticleTitle:          fmt.Sprintf("文章%02d", i+1),
			ArticleAbstract:       fmt.Sprintf("摘要%02d", i+1),
			ArticleStatus:         enum.ArticleStatusPublished,
			ArticleAuthorID:       100,
			ArticleAuthorNickname: "作者甲",
		}
		if err := db.Omit("UserModel", "ArticleModel", "FavoriteModel").Create(&m).Error; err != nil {
			t.Fatalf("种子收藏关系 %d 失败: %v", articleID, err)
		}
	}

	// 置顶：用户 100 置顶 3 篇已发布文章。
	for k, i := range []int{2, 10, 18} {
		m := models.UserTopArticleModel{
			Model:     models.Model{ID: ctype.ID(6400 + k), CreatedAt: base, UpdatedAt: base},
			UserID:    100,
			ArticleID: ctype.ID(1001 + i),
		}
		if err := db.Omit("UserModel", "ArticleModel").Create(&m).Error; err != nil {
			t.Fatalf("种子置顶 %d 失败: %v", i, err)
		}
	}

	// 浏览历史：用户 100 看过 5 篇。
	for k, i := range []int{2, 3, 6, 10, 18} {
		m := models.UserArticleViewHistoryModel{
			Model:     models.Model{ID: ctype.ID(6600 + k), CreatedAt: base, UpdatedAt: base},
			UserID:    100,
			ArticleID: ctype.ID(1001 + i),
		}
		if err := db.Omit("UserModel", "ArticleModel").Create(&m).Error; err != nil {
			t.Fatalf("种子浏览历史 %d 失败: %v", i, err)
		}
	}

	// 审核任务：给 5 篇"审核中"的文章各建一条 pending 任务。
	for k, i := range []int{1, 5, 9, 13, 17} {
		m := models.ArticleReviewTaskModel{
			Model:                models.Model{ID: ctype.ID(7000 + k), CreatedAt: base.Add(time.Duration(k) * time.Minute), UpdatedAt: base},
			ArticleID:            ctype.ID(1001 + i),
			AuthorID:             100,
			ArticleTitle:         fmt.Sprintf("文章%02d", i+1),
			AuthorName:           "作者甲",
			ArticlePublishStatus: enum.ArticleStatusExamining,
			Stage:                models.ArticleReviewTaskStageManual,
			Source:               models.ArticleReviewTaskSourceCreate,
			Status:               models.ArticleReviewTaskPending,
		}
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("种子审核任务 %d 失败: %v", 1001+i, err)
		}
	}
}

// seedGoldenUser 插入一个写死时间的用户。
func seedGoldenUser(t *testing.T, db *gorm.DB, id uint64, username, nickname, avatar, abstract string, role enum.RoleType) {
	t.Helper()
	u := models.UserModel{}
	u.ID = ctype.ID(id)
	u.Username = username
	u.Nickname = nickname
	u.Avatar = avatar
	u.Abstract = abstract
	u.Password = "x"
	u.Role = role
	u.CreatedAt = goldenBaseTime()
	u.UpdatedAt = u.CreatedAt
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("种子用户 %d 失败: %v", id, err)
	}
}
