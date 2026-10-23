package flags

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/markdown"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
	if _, err := Run(&FlagOptions{}, Deps{}); err != nil {
		t.Fatalf("Run no-op 返回错误: %v", err)
	}
}

func TestFlagDB(t *testing.T) {
	db := testutil.SetupSQLite(t)
	if err := FlagDB(db, nil); err != nil {
		t.Fatalf("FlagDB 返回错误: %v", err)
	}

	if !db.Migrator().HasTable(&models.UserModel{}) {
		t.Fatal("UserModel 表未迁移")
	}
	if !db.Migrator().HasTable(&models.ArticleModel{}) {
		t.Fatal("ArticleModel 表未迁移")
	}
}

func TestFlagESIndexNoOp(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_idx"},
	})

	withStdin(t, "3\n3\n", func() {
		if err := FlagESIndex(Deps{ESIndex: testutil.Config().ES.Index}); err == nil {
			t.Fatal("ESClient 为空时应返回错误")
		}
	})
}

func TestFlagUserCreateInvalidRoleAndExistsUser(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{})

	t.Run("非法角色直接返回", func(t *testing.T) {
		withStdin(t, "0\n", func() {
			u := FlagUser{}
			u.Create(db, nil, UserCreateOptions{})
		})
		var cnt int64
		_ = db.Model(&models.UserModel{}).Count(&cnt).Error
		if cnt != 0 {
			t.Fatalf("非法角色不应创建用户, cnt=%d", cnt)
		}
	})

	t.Run("用户名已存在直接返回", func(t *testing.T) {
		exists := models.UserModel{
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
		_ = db.Model(&models.UserModel{}).Where("username = ?", "exists_u").Count(&cnt).Error
		if cnt != 1 {
			t.Fatalf("已存在用户分支不应重复创建, cnt=%d", cnt)
		}
	})
}

func TestFlagUserCreateNonInteractive(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{})
	u := FlagUser{}

	u.Create(db, nil, UserCreateOptions{
		Role:     "admin",
		Username: "cli_admin",
		Password: "123456",
		Nickname: "CLI管理员",
		Email:    "cli_admin@example.com",
	})

	var created models.UserModel
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

func TestBuildArticleESDocument(t *testing.T) {
	categoryID := models.Model{ID: 7}.ID
	article := models.ArticleModel{
		Model: models.Model{
			ID:        1,
			CreatedAt: time.Unix(1710000000, 0),
			UpdatedAt: time.Unix(1710003600, 0),
		},
		Title:          "文章标题",
		Abstract:       "文章摘要",
		Content:        "# 标题\n正文",
		CategoryID:     &categoryID,
		CategoryModel:  &models.CategoryModel{Model: models.Model{ID: categoryID}, Title: "后端"},
		Cover:          "/cover.png",
		AuthorID:       9,
		UserModel:      models.UserModel{Model: models.Model{ID: 9}, Nickname: "作者A", Avatar: "/avatar.png"},
		ViewCount:      11,
		DiggCount:      12,
		CommentCount:   13,
		FavorCount:     14,
		CommentsToggle: true,
		PublishStatus:  3,
		Tags: []models.TagModel{
			{Model: models.Model{ID: 1}, Title: "Go"},
			{Model: models.Model{ID: 2}, Title: "Redis"},
		},
	}

	doc := buildArticleESDocument(article, true, false)

	if _, ok := doc["content"]; ok {
		t.Fatal("content 不应被同步到 ES 文档")
	}
	if got, ok := doc["content_head"].(string); !ok || got == "" {
		t.Fatalf("content_head 应被生成, got=%#v", doc["content_head"])
	}
	if parts, ok := doc["content_parts"].([]markdown.ContentPart); !ok || len(parts) == 0 {
		t.Fatalf("content_parts 应被生成, got=%#v", doc["content_parts"])
	} else if strings.Contains(parts[0].Content, "# ") {
		t.Fatalf("content_parts 应存纯文本内容, got=%q", parts[0].Content)
	}
	if got, ok := doc["comments_toggle"].(bool); !ok || !got {
		t.Fatalf("comments_toggle 应按 boolean mapping 转成 true, got=%#v", doc["comments_toggle"])
	}
	tags, ok := doc["tags"].([]models.ESTag)
	if !ok || len(tags) != 2 || tags[0].Title != "Go" || tags[1].Title != "Redis" {
		t.Fatalf("tags 同步结果不正确: %#v", doc["tags"])
	}
	category, ok := doc["category"].(map[string]any)
	if !ok || category["id"] != categoryID.String() || category["title"] != "后端" {
		t.Fatalf("分类字段同步结果不正确: %#v", doc["category"])
	}
	author, ok := doc["author"].(map[string]any)
	if !ok || author["id"] != article.AuthorID.String() || author["nickname"] != "作者A" || author["avatar"] != "/avatar.png" {
		t.Fatalf("作者字段同步结果不正确: %#v", doc["author"])
	}
	if doc["admin_top"] != true || doc["author_top"] != false {
		t.Fatalf("置顶字段同步结果不正确: admin=%#v author=%#v", doc["admin_top"], doc["author_top"])
	}
	const expectedFieldCount = 23
	if len(doc) != expectedFieldCount {
		t.Fatalf("ES 文档字段数不正确, got=%d want=%d", len(doc), expectedFieldCount)
	}
}

func TestSyncArticleDocuments(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.ArticleModel{}, &models.TagModel{}, &models.ArticleTagModel{}, &models.UserTopArticleModel{})
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES:    conf.ES{Index: "article_index"},
		River: conf.River{BulkSize: 2},
	})

	admin := models.UserModel{Username: "admin", Password: "x", Role: enum.RoleAdmin}
	author1 := models.UserModel{Username: "author1", Password: "x", Role: enum.RoleUser}
	author2 := models.UserModel{Username: "author2", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(&author1).Error; err != nil {
		t.Fatalf("创建作者1失败: %v", err)
	}
	if err := db.Create(&author2).Error; err != nil {
		t.Fatalf("创建作者2失败: %v", err)
	}

	tagGo := models.TagModel{Title: "Go", IsEnabled: true}
	tagBackend := models.TagModel{Title: "后端", IsEnabled: true}
	tagRedis := models.TagModel{Title: "Redis", IsEnabled: true}
	if err := db.Create(&tagGo).Error; err != nil {
		t.Fatalf("创建标签 Go 失败: %v", err)
	}
	if err := db.Create(&tagBackend).Error; err != nil {
		t.Fatalf("创建标签 后端 失败: %v", err)
	}
	if err := db.Create(&tagRedis).Error; err != nil {
		t.Fatalf("创建标签 Redis 失败: %v", err)
	}

	articles := []models.ArticleModel{
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
	if err := db.Create(&models.ArticleTagModel{ArticleID: articles[0].ID, TagID: tagGo.ID}).Error; err != nil {
		t.Fatalf("创建文章1-Go 标签关系失败: %v", err)
	}
	if err := db.Create(&models.ArticleTagModel{ArticleID: articles[0].ID, TagID: tagBackend.ID}).Error; err != nil {
		t.Fatalf("创建文章1-后端 标签关系失败: %v", err)
	}
	if err := db.Create(&models.ArticleTagModel{ArticleID: articles[1].ID, TagID: tagRedis.ID}).Error; err != nil {
		t.Fatalf("创建文章2-Redis 标签关系失败: %v", err)
	}
	if err := db.Create(&models.UserTopArticleModel{UserID: admin.ID, ArticleID: articles[0].ID}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}
	if err := db.Create(&models.UserTopArticleModel{UserID: author1.ID, ArticleID: articles[0].ID}).Error; err != nil {
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
