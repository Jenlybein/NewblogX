//go:build integration

package http_test

import (
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	articlehttp "myblogx/internal/article/adapter/http"
	articlemysql "myblogx/internal/article/adapter/mysql"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/internal/testkit/schema"
	"myblogx/utils/jwts"
)

// 本文件守 #279 的「读路径只认一个权威值入口」：
//
// 同一个计数在**同一时刻**只有一个权威值。过去不是这样：详情 / 列表 / 收藏内容
// 会叠上尚未并账的 Redis 增量，而首页「热门置顶」与作者置顶卡片直接读库里的列，
// 于是同一篇文章在首页是 5、点进去是 7。两套口径的差别只有在"有增量还没并账"
// 的那段时间里可见，正是用户最容易看到的那段时间。
//
// 这里只经 HTTP 观察外部行为：同一条文章在详情与两处卡片上的浏览量必须相同，
// 且都等于"持久列 + 待并账增量"。不读源码、不越过接口查实现细节。

const authorityArticleID = 1001

// newRouterWithCounters 与 newRouterWithIDs 相同，但把计数端口接到真实 Redis（miniredis）。
//
// 不接 Redis 时计数器退化 no-op，增量永远是 0 —— 那样就看不见"两套口径"的差异。
func newRouterWithCounters(
	t *testing.T,
	db *gorm.DB,
	viewer domain.Viewer,
	client *redis.Client,
) *gin.Engine {
	t.Helper()

	opts := redis_service.Options{Client: client}
	uow := articlemysql.NewTxManager(db, opts, nil)
	svc := articleapp.New(uow, testClock{at: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}, &testIDs{n: 100})
	h := articlehttp.NewHandler(svc, nil, nil, func(*gin.Context) domain.Viewer { return viewer }, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	auth := func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: ctype.NewID(900), Role: schema.RoleAdmin}})
		c.Next()
	}
	h.Register(api, auth, auth)
	return r
}

// TestCountsComeFromOneAuthority 详情与两处置顶卡片给出同一个权威浏览量。
func TestCountsComeFromOneAuthority(t *testing.T) {
	db := migrate(t)
	seedAuthor(t, db, 100)

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            authorityArticleID,
		Title:         "计数权威值文章",
		AuthorID:      100,
		ViewCount:     5,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     at,
		UpdatedAt:     at,
	})

	// 两条运营位都挂上这篇文章：首页热门置顶 + 作者置顶。
	schema.InsertArticleHotTop(t, db, 5001, authorityArticleID, 1, at)
	schema.InsertUserTopArticle(t, db, 5002, 100, authorityArticleID, at)

	// 待并账的增量：浏览 +2、点赞 +1（还没到并账周期）。
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	opts := redis_service.Options{Client: client}
	if err := redis_article.SetCacheView(opts, ctype.NewID(authorityArticleID), 2); err != nil {
		t.Fatalf("写入浏览增量失败: %v", err)
	}
	if err := redis_article.SetCacheDigg(opts, ctype.NewID(authorityArticleID), 1); err != nil {
		t.Fatalf("写入点赞增量失败: %v", err)
	}

	reader := newRouterWithCounters(t, db, domain.Anonymous(), client)

	detail := mustData(t, do(t, reader, "GET", "/api/articles/1001", nil))
	if got := int(detail["view_count"].(float64)); got != 7 {
		t.Fatalf("详情页浏览量应为 5+2=7，实际 %d", got)
	}
	if got := int(detail["digg_count"].(float64)); got != 1 {
		t.Fatalf("详情页点赞数应为 0+1=1，实际 %d", got)
	}

	hotCard := firstCard(t, do(t, reader, "GET", "/api/articles/hot-top", nil))
	if got := int(hotCard["view_count"].(float64)); got != 7 {
		t.Fatalf("首页热门置顶卡片的浏览量必须与详情同一权威值 7，实际 %d", got)
	}

	topCard := firstCard(t, do(t, reader, "GET", "/api/articles/top?user_id=100", nil))
	if got := int(topCard["view_count"].(float64)); got != 7 {
		t.Fatalf("作者置顶卡片的浏览量必须与详情同一权威值 7，实际 %d", got)
	}
}

// firstCard 从列表响应里取第一张卡片。
func firstCard(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	list, ok := mustData(t, resp)["list"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("卡片列表应有 1 条，实际 %v", resp)
	}
	card, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("卡片形状不符: %v", list[0])
	}
	return card
}
