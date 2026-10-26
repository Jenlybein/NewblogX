package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"myblogx/conf"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v7"
)

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}
	if _, err = w.WriteString(input); err != nil {
		t.Fatalf("写入 stdin 数据失败: %v", err)
	}
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
	})
	fn()
}

func TestParse(t *testing.T) {
	oldCommandLine := flag.CommandLine
	oldArgs := os.Args
	defer func() {
		flag.CommandLine = oldCommandLine
		os.Args = oldArgs
	}()

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Args = []string{"cmd", "-f", "custom.yaml", "-db"}

	op := Parse()
	if op.File != "custom.yaml" {
		t.Fatalf("File 解析错误: %s", op.File)
	}
	if !op.DB {
		t.Fatal("DB 标志解析错误")
	}
}

func TestRunNoOp(t *testing.T) {
	testutil.InitGlobals()
	if _, err := Run(&FlagOptions{}, Options{}); err != nil {
		t.Fatalf("Run no-op 返回错误: %v", err)
	}
}

func TestFlagESIndexNoOp(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_idx"},
	})

	withStdin(t, "3\n3\n", func() {
		if err := FlagESIndex(Options{ESIndex: testutil.Config().ES.Index}); err == nil {
			t.Fatal("ESClient 为空时应返回错误")
		}
	})
}

func TestFlagUserCreateInvalidRoleAndExistsUser(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.UserRow{}, &schema.UserConfRow{})

	t.Run("非法角色直接返回", func(t *testing.T) {
		withStdin(t, "0\n", func() {
			u := FlagUser{}
			u.Create(db, nil, UserCreateOptions{})
		})
		var cnt int64
		_ = db.Model(&schema.UserRow{}).Count(&cnt).Error
		if cnt != 0 {
			t.Fatalf("非法角色不应创建用户, cnt=%d", cnt)
		}
	})

	t.Run("用户名已存在直接返回", func(t *testing.T) {
		exists := schema.UserRow{
			Username: "exists_u",
			Password: "x",
		}
		if err := db.Create(&exists).Error; err != nil {
			t.Fatalf("创建已有用户失败: %v", err)
		}

		withStdin(t, "1\nexists_u\n", func() {
			u := FlagUser{}
			u.Create(db, nil, UserCreateOptions{})
		})

		var cnt int64
		_ = db.Model(&schema.UserRow{}).Where("username = ?", "exists_u").Count(&cnt).Error
		if cnt != 1 {
			t.Fatalf("已存在用户分支不应重复创建, cnt=%d", cnt)
		}
	})
}

func TestFlagUserCreateNonInteractive(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.UserRow{}, &schema.UserConfRow{})
	u := FlagUser{}

	u.Create(db, nil, UserCreateOptions{
		Role:     "admin",
		Username: "cli_admin",
		Password: "123456",
		Nickname: "CLI管理员",
		Email:    "cli_admin@example.com",
	})

	var created schema.UserRow
	if err := db.Take(&created, "username = ?", "cli_admin").Error; err != nil {
		t.Fatalf("非交互创建用户失败: %v", err)
	}
	if created.Role != enum.RoleAdmin {
		t.Fatalf("角色不正确: got=%v want=%v", created.Role, enum.RoleAdmin)
	}
	if created.Nickname != "CLI管理员" {
		t.Fatalf("昵称不正确: got=%q", created.Nickname)
	}
	if created.Email == nil || *created.Email != "cli_admin@example.com" {
		t.Fatalf("邮箱不正确: got=%v", created.Email)
	}
	if created.Password == "" || created.Password == "123456" {
		t.Fatalf("密码应加密存储, got=%q", created.Password)
	}
}

func setupMockESClient(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	server := httptest.NewServer(handler)
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("创建 mock ES 客户端失败: %v", err)
	}

	oldClient := testutil.ESClient()
	testutil.SetESClient(client)
	t.Cleanup(func() {
		testutil.SetESClient(oldClient)
		server.Close()
	})
}

func writeESJSON(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Elastic-Product", "Elasticsearch")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}

func TestSyncArticleDocuments(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.UserRow{}, &schema.UserConfRow{}, &schema.ArticleRow{}, &schema.TagRow{}, &schema.ArticleTagRow{}, &schema.UserTopArticleRow{})
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES:    conf.ES{Index: "article_index"},
		River: conf.River{BulkSize: 2},
	})

	admin := schema.UserRow{Username: "admin", Password: "x", Role: enum.RoleAdmin}
	author1 := schema.UserRow{Username: "author1", Password: "x", Role: enum.RoleUser}
	author2 := schema.UserRow{Username: "author2", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(&author1).Error; err != nil {
		t.Fatalf("创建作者1失败: %v", err)
	}
	if err := db.Create(&author2).Error; err != nil {
		t.Fatalf("创建作者2失败: %v", err)
	}

	tagGo := schema.TagRow{Title: "Go", IsEnabled: true}
	tagBackend := schema.TagRow{Title: "后端", IsEnabled: true}
	tagRedis := schema.TagRow{Title: "Redis", IsEnabled: true}
	if err := db.Create(&tagGo).Error; err != nil {
		t.Fatalf("创建标签 Go 失败: %v", err)
	}
	if err := db.Create(&tagBackend).Error; err != nil {
		t.Fatalf("创建标签 后端 失败: %v", err)
	}
	if err := db.Create(&tagRedis).Error; err != nil {
		t.Fatalf("创建标签 Redis 失败: %v", err)
	}

	articles := []schema.ArticleRow{
		{
			Title:          "第一篇",
			Abstract:       "摘要1",
			Content:        "# 第一篇\na",
			AuthorID:       author1.ID,
			ViewCount:      10,
			DiggCount:      2,
			CommentCount:   3,
			FavorCount:     4,
			CommentsToggle: true,
			PublishStatus:  3,
		},
		{
			Title:          "第二篇",
			Abstract:       "摘要2",
			Content:        "# 第二篇\nb",
			AuthorID:       author2.ID,
			ViewCount:      20,
			DiggCount:      5,
			CommentCount:   6,
			FavorCount:     7,
			CommentsToggle: false,
			PublishStatus:  2,
		},
	}
	if err := db.Create(&articles).Error; err != nil {
		t.Fatalf("创建测试文章失败: %v", err)
	}
	if err := db.Create(&schema.ArticleTagRow{ArticleID: articles[0].ID, TagID: tagGo.ID}).Error; err != nil {
		t.Fatalf("创建文章1-Go 标签关系失败: %v", err)
	}
	if err := db.Create(&schema.ArticleTagRow{ArticleID: articles[0].ID, TagID: tagBackend.ID}).Error; err != nil {
		t.Fatalf("创建文章1-后端 标签关系失败: %v", err)
	}
	if err := db.Create(&schema.ArticleTagRow{ArticleID: articles[1].ID, TagID: tagRedis.ID}).Error; err != nil {
		t.Fatalf("创建文章2-Redis 标签关系失败: %v", err)
	}
	if err := db.Create(&schema.UserTopArticleRow{UserID: admin.ID, ArticleID: articles[0].ID}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}
	if err := db.Create(&schema.UserTopArticleRow{UserID: author1.ID, ArticleID: articles[0].ID}).Error; err != nil {
		t.Fatalf("创建作者置顶失败: %v", err)
	}

	var bulkDocs []map[string]any
	setupMockESClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeESJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.Method == http.MethodHead && r.URL.Path == "/article_index":
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/article_index/_bulk":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("读取 bulk body 失败: %v", err)
			}
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Bytes()
				if len(strings.TrimSpace(string(line))) == 0 {
					continue
				}
				if lineNo%2 == 0 {
					var doc map[string]any
					if err = json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeESJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeESJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	total, err := syncArticleDocuments(db, testutil.ESClient(), "article_index", 2, nil)
	if err != nil {
		t.Fatalf("同步文章到 ES 失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("同步文章数量不正确: got=%d want=2", total)
	}
	if len(bulkDocs) != 2 {
		t.Fatalf("写入 ES 的文档数不正确: got=%d want=2", len(bulkDocs))
	}

	first := bulkDocs[0]
	if _, ok := first["content"]; ok {
		t.Fatal("bulk 文档中不应包含 content 字段")
	}
	if got, ok := first["comments_toggle"].(bool); !ok || !got {
		t.Fatalf("comments_toggle 应写成 true, got=%#v", first["comments_toggle"])
	}
	tags, ok := first["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("tags 应以数组写入 ES, got=%#v", first["tags"])
	}
	author, ok := first["author"].(map[string]any)
	if !ok || author["id"] != author1.ID.String() {
		t.Fatalf("作者字段应正确写入 ES, got=%#v", first["author"])
	}
	if first["admin_top"] != true || first["author_top"] != true {
		t.Fatalf("置顶字段应正确写入 ES, got admin=%#v author=%#v", first["admin_top"], first["author_top"])
	}
}
