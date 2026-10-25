//go:build integration

package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	articlehttp "myblogx/internal/article/adapter/http"
	articlemysql "myblogx/internal/article/adapter/mysql"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	redis_service "myblogx/internal/platform/redis"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/jwts"
)

// testClock 是固定时钟。
type testClock struct{ at time.Time }

func (c testClock) Now() time.Time { return c.at }

// testIDs 是固定 ID 序列，保证断言可复现。
type testIDs struct{ n uint64 }

func (g *testIDs) next() uint64 { g.n++; return g.n }

func (g *testIDs) NextArticleID() domain.ArticleID { return domain.ArticleID(g.next()) }
func (g *testIDs) NextReviewTaskID() uint64        { return g.next() }
func (g *testIDs) NextFolderID() favorite.ID       { return favorite.ID(g.next()) }

func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	if err := db.AutoMigrate(
		&models.UserModel{},
		&models.UserConfModel{},
		&models.UserStatModel{},
		&models.UserSessionModel{},
		&models.ArticleModel{},
		&models.ArticleTagModel{},
		&models.TagModel{},
		&models.CategoryModel{},
		&models.ArticleDiggModel{},
		&models.FavoriteModel{},
		&models.UserArticleFavorModel{},
		&models.UserTopArticleModel{},
		&models.UserArticleViewHistoryModel{},
		&models.ArticleReviewTaskModel{},
		&models.CommentModel{},
		// 写侧会把领域事件插进 outbox 表，缺表会让整笔事务失败。
		&models.OutboxEventModel{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// newRouter 装一套真实链路（真 MySQL + 新 handler），只把鉴权换成固定身份。
func newRouter(t *testing.T, db *gorm.DB, viewer domain.Viewer) *gin.Engine {
	t.Helper()
	return newRouterWithIDs(t, db, viewer, 100)
}

// newRouterWithIDs 允许指定 ID 起点：golden 要可复现，ID 必须可预测。
func newRouterWithIDs(t *testing.T, db *gorm.DB, viewer domain.Viewer, idStart uint64) *gin.Engine {
	t.Helper()

	uow := articlemysql.NewTxManager(db, redis_service.Options{})
	svc := articleapp.New(uow, testClock{at: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}, &testIDs{n: idStart})

	h := articlehttp.NewHandler(svc, nil, nil, func(*gin.Context) domain.Viewer { return viewer }, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	// 假鉴权：真实 JWT 中间件不在测试范围内。这里注入固定 claims，
	// 因为 AdminMiddleware 会直接读上下文里的 claims（MustGet 不存在会 panic）。
	// 实际操作者仍由上面的 viewer 闭包决定，两者互不影响。
	auth := func(c *gin.Context) {
		c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: ctype.ID(900), Role: enum.RoleAdmin}})
		c.Next()
	}
	h.Register(api, auth, auth)
	return r
}

// rawRequest 返回原始响应字节。
//
// golden 必须比字节：结构体比较会掩盖字段顺序、数字/字符串、null 与缺省等差异。
func rawRequest(t *testing.T, r *gin.Engine, method, path string, body []byte) []byte {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	reader := httptest.NewRecorder()
	r.ServeHTTP(reader, req)
	return reader.Body.Bytes()
}

// seedAuthor 插入作者行（article.author_id 有外键）。
func seedAuthor(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	u := models.UserModel{}
	u.ID = ctype.ID(id)
	u.Username = "author"
	u.Nickname = "作者"
	u.Password = "x"
	// 固定时间：作者创建时间会进详情响应（author_created_time），
	// 不定死的话 golden 每次都变。
	u.CreatedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	u.UpdatedAt = u.CreatedAt
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
}

func do(t *testing.T, r *gin.Engine, method, path string, body []byte) map[string]any {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	reader := httptest.NewRecorder()
	r.ServeHTTP(reader, req)

	var out map[string]any
	if err := json.Unmarshal(reader.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON（%s %s）: %s", method, path, reader.Body.String())
	}
	return out
}

// TestDetailEndToEnd 验证新链路能真正查出一篇文章并返回旧格式字段。
func TestDetailEndToEnd(t *testing.T) {
	t.Parallel()

	db := migrate(t)

	// 造作者
	u := models.UserModel{}
	u.ID = ctype.ID(100)
	u.Username = "author"
	u.Nickname = "作者"
	u.Password = "x"
	// 固定时间：作者创建时间会进详情响应（author_created_time），
	// 不定死的话 golden 每次都变。
	u.CreatedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	u.UpdatedAt = u.CreatedAt
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	// 造一篇已发布文章（走真实 app 用例）
	r := newRouter(t, db, domain.NormalUser(100))
	createResp := do(t, r, http.MethodPost, "/api/articles", []byte(`{
		"title":"端到端文章","content":"正文","status":2,"visibility_status":"visible"
	}`))
	if code, _ := createResp["code"].(float64); code != 0 {
		t.Fatalf("创建失败: %v", createResp)
	}

	data, ok := createResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("创建响应缺 data: %v", createResp)
	}
	// ctype.ID 自定义了 MarshalJSON：雪花 ID 超出 JS 安全整数，统一序列化成字符串。
	// 这正是 golden 要守住的格式之一（数字变字符串会让前端拼 URL 出错）。
	idStr, ok := data["id"].(string)
	if !ok || idStr == "" {
		t.Fatalf("创建后应有字符串 ID: %v", data)
	}
	if _, err := strconv.ParseUint(idStr, 10, 64); err != nil {
		t.Fatalf("ID 应是纯数字字符串，实际 %q", idStr)
	}

	// 作者读自己的草稿：应可见。
	authorRouter := newRouter(t, db, domain.NormalUser(100))
	detail := do(t, authorRouter, http.MethodGet, "/api/articles/"+idStr, nil)

	t.Logf("detail 响应: %#v", detail)
	d, ok := detail["data"].(map[string]any)
	if !ok {
		t.Fatalf("作者应能读自己的文章，实际: %v", detail)
	}
	if d["title"] != "端到端文章" {
		t.Fatalf("标题不符: %v", d["title"])
	}

	// 关键字段必须都在（前端零改动的前提）
	for _, key := range []string{"id", "created_at", "author_id", "publish_status", "visibility_status", "is_digg", "is_favor"} {
		if _, ok := d[key]; !ok {
			t.Fatalf("响应缺字段 %q: %v", key, d)
		}
	}

	// 匿名读同一篇：审核中的文章对匿名一律不可见（与旧实现一致）。
	anonRouter := newRouter(t, db, domain.Anonymous())
	anonResp := do(t, anonRouter, http.MethodGet, "/api/articles/"+idStr, nil)
	if code, _ := anonResp["code"].(float64); code == 0 {
		t.Fatalf("匿名不应看到审核中的文章，实际: %v", anonResp)
	}
}
