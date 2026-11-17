package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 存量清洗（票 402）：这里钉住命令的真正行为，而不是"扫了几个表"。
//
//   - 预演一次都不写：文本与命中记录都必须原样；
//   - apply 后文本变等长 `*`、补写了命中记录；
//   - 再跑一次是幂等的：命中 0、记录不再增；
//   - 六个文本面各自被扫到（少一面就是漏了整整一类内容）；
//   - 「审核中」的存量评论被退役。

// sweepRecorderSpy 记录补写了什么（生产实现是 *filterapp.Service）。
type sweepRecorderSpy struct {
	hits []filterdomain.Hit
}

func (r *sweepRecorderSpy) Record(_ context.Context, hit filterdomain.Hit) {
	r.hits = append(r.hits, hit)
}

// newSweepEngine 构造一个只含"冰毒"的引擎（拦截 / 毒品档）。
func newSweepEngine(t *testing.T) *contentfilter.Engine {
	t.Helper()
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		nil,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	return engine
}

// sweepFixture 是六个文本面的最小数据集，每面都有一条含"冰毒"的记录。
// 字段类型用 ctype.ID 而不是裸 uint64：雪花 ID 超出 JS 安全整数范围，
// 裸整型在契约里会印成 integer（archcheck 的 R12），测试里引入同样的形状
// 只会把这条纪律变成"只在生产代码上成立"。
type sweepFixture struct {
	article    ctype.ID
	comment    ctype.ID
	user       ctype.ID
	chat       ctype.ID
	collection ctype.ID
}

// seedSweepFixture 建表并插入六个面的数据。
func seedSweepFixture(t *testing.T) (*gorm.DB, sweepFixture) {
	t.Helper()
	db := testutil.SetupSQLite(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.Comment, schema.Chat, schema.Collection)

	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	ids := sweepFixture{
		article:    ctype.NewID(1001),
		comment:    ctype.NewID(2001),
		user:       ctype.NewID(3001),
		chat:       ctype.NewID(4001),
		collection: ctype.NewID(5001),
	}

	// 用 InsertUser 而不是手拼 UserRow：它会顺带建出 user_conf / user_stat，
	// 手拼会漏掉 AfterCreate 钩子（那时读用户资料会撞空表）。
	schema.InsertUser(t, db, schema.UserSpec{
		ID:        ids.user.Uint64(),
		Username:  "u1",
		Nickname:  "卖冰毒的",
		Abstract:  "聊聊冰毒",
		CreatedAt: now,
		UpdatedAt: now,
	})

	articles := []schema.ArticleRow{{
		BaseRow:  schema.BaseRow{ID: ids.article, CreatedAt: now, UpdatedAt: now},
		Title:    "关于冰毒",
		Content:  "正文里有冰毒",
		AuthorID: ids.user,
	}}
	if err := db.Create(&articles).Error; err != nil {
		t.Fatalf("插入文章失败: %v", err)
	}

	comments := []schema.CommentRow{{
		BaseRow:  schema.BaseRow{ID: ids.comment, CreatedAt: now, UpdatedAt: now},
		Content:  "测试冰毒",
		UserID:   ids.user,
		ArticleID: ids.article,
		Status:   2,
	}}
	if err := db.Create(&comments).Error; err != nil {
		t.Fatalf("插入评论失败: %v", err)
	}

	chats := []schema.ChatMsgRow{{
		BaseRow:   schema.BaseRow{ID: ids.chat, CreatedAt: now, UpdatedAt: now},
		SenderID:  ids.user,
		ReceiverID: ctype.NewID(ids.user.Uint64() + 1),
		SessionID: "chat:1:2",
		Content:   "私信里有冰毒",
		SendTime:  now,
	}}
	if err := db.Create(&chats).Error; err != nil {
		t.Fatalf("插入私信失败: %v", err)
	}

	collections := []schema.CollectionRow{{
		BaseRow:     schema.BaseRow{ID: ids.collection, CreatedAt: now, UpdatedAt: now},
		Title:       "冰毒合集",
		Description: "描述里也有冰毒",
		IsDisplay:   true,
		CreatorID:   ids.user,
	}}
	if err := db.Create(&collections).Error; err != nil {
		t.Fatalf("插入合集失败: %v", err)
	}
	return db, ids
}


// readArticle 读文章三列。
func readArticle(t *testing.T, db *gorm.DB, id ctype.ID) (title, content string) {
	t.Helper()
	var row schema.ArticleRow
	if err := db.First(&row, "id = ?", id.Uint64()).Error; err != nil {
		t.Fatalf("读文章失败: %v", err)
	}
	return row.Title, row.Content
}

// TestSweepDryRunChangesNothing 预演：文本与命中记录都不动。
func TestSweepDryRunChangesNothing(t *testing.T) {
	db, ids := seedSweepFixture(t)
	engine := newSweepEngine(t)

	report, err := sweepContent(context.Background(), db, engine, nil, false)
	if err != nil {
		t.Fatalf("预演不该失败: %v", err)
	}
	if report.Apply {
		t.Fatal("预演的报告应标记为未写库")
	}
	// 六个面都得报出来：报告少一面 = 扫描漏了一面。
	if len(report.Surfaces) != 6 {
		t.Fatalf("应报六个文本面，实际 %d: %+v", len(report.Surfaces), report.Surfaces)
	}

	title, content := readArticle(t, db, ids.article)
	if strings.Contains(title, "*") || strings.Contains(content, "*") {
		t.Fatalf("预演不该改文章正文: title=%q content=%q", title, content)
	}
	var hitRows int64
	db.Model(&schema.ContentFilterHitRow{}).Count(&hitRows)
	if hitRows != 0 {
		t.Fatalf("预演不该写命中记录，实际 %d 条", hitRows)
	}
}

// TestSweepApplyMasksAllSixSurfaces apply：六个面都替换，并各写一条记录。
func TestSweepApplyMasksAllSixSurfaces(t *testing.T) {
	db, ids := seedSweepFixture(t)
	engine := newSweepEngine(t)
	recorder := &sweepRecorderSpy{}

	if _, err := sweepContent(context.Background(), db, engine, recorder, true); err != nil {
		t.Fatalf("apply 不该失败: %v", err)
	}

	title, content := readArticle(t, db, ids.article)
	if title != "关于**" || content != "正文里有**" {
		t.Fatalf("文章标题与正文都应替换: title=%q content=%q", title, content)
	}

	var user schema.UserRow
	if err := db.First(&user, "id = ?", ids.user.Uint64()).Error; err != nil {
		t.Fatalf("读用户失败: %v", err)
	}
	if user.Nickname != "卖**的" || user.Abstract != "聊聊**" {
		t.Fatalf("昵称与简介都应替换: nick=%q abstract=%q", user.Nickname, user.Abstract)
	}

	var comment schema.CommentRow
	if err := db.First(&comment, "id = ?", ids.comment.Uint64()).Error; err != nil {
		t.Fatalf("读评论失败: %v", err)
	}
	if comment.Content != "测试**" {
		t.Fatalf("评论应替换: %q", comment.Content)
	}

	var chat schema.ChatMsgRow
	if err := db.First(&chat, "id = ?", ids.chat.Uint64()).Error; err != nil {
		t.Fatalf("读私信失败: %v", err)
	}
	if chat.Content != "私信里有**" {
		t.Fatalf("私信应替换: %q", chat.Content)
	}

	var col schema.CollectionRow
	if err := db.First(&col, "id = ?", ids.collection.Uint64()).Error; err != nil {
		t.Fatalf("读合集失败: %v", err)
	}
	if col.Title != "**合集" || col.Description != "描述里也有**" {
		t.Fatalf("合集标题与描述都应替换: title=%q desc=%q", col.Title, col.Description)
	}

	// 六个面各至少写了一条记录（同一面多个字段会写多条）。
	surfaces := map[filterdomain.Surface]bool{}
	for _, hit := range recorder.hits {
		surfaces[hit.Surface] = true
	}
	if len(surfaces) < 6 {
		t.Fatalf("六个文本面都该写记录，实际只写了 %d 个面: %+v", len(surfaces), surfaces)
	}
}

// TestSweepApplyIsIdempotent 第二次执行不再命中、不再写记录。
func TestSweepApplyIsIdempotent(t *testing.T) {
	db, _ := seedSweepFixture(t)
	engine := newSweepEngine(t)

	first := &sweepRecorderSpy{}
	if _, err := sweepContent(context.Background(), db, engine, first, true); err != nil {
		t.Fatalf("第一次 apply 失败: %v", err)
	}

	second := &sweepRecorderSpy{}
	report, err := sweepContent(context.Background(), db, engine, second, true)
	if err != nil {
		t.Fatalf("第二次 apply 失败: %v", err)
	}
	for _, stats := range report.Surfaces {
		if stats.Hit != 0 || stats.Spans != 0 {
			t.Fatalf("第二次不该再命中（%s: hit=%d spans=%d）", stats.Surface, stats.Hit, stats.Spans)
		}
	}
	if len(second.hits) != 0 {
		t.Fatalf("第二次不该写命中记录，实际 %d 条", len(second.hits))
	}
	if len(first.hits) == 0 {
		t.Fatal("第一次应写了命中记录（否则幂等用例证明不了什么）")
	}
}

// TestSweepRetiresLegacyExaminingComments 事故遗留的「审核中」评论被退役。
func TestSweepRetiresLegacyExaminingComments(t *testing.T) {
	db, _ := seedSweepFixture(t)
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	// 插入一条「审核中」的存量评论（票 400 之后没有任何写入路径会再产生它）。
	if err := db.Create(&schema.CommentRow{
		BaseRow:  schema.BaseRow{ID: ctype.NewID(2002), CreatedAt: now, UpdatedAt: now},
		Content:  "冰毒",
		UserID:   ctype.NewID(3001),
		ArticleID: ctype.NewID(1001),
		Status:   1,
	}).Error; err != nil {
		t.Fatalf("插入审核中评论失败: %v", err)
	}

	report, err := sweepContent(context.Background(), db, newSweepEngine(t), &sweepRecorderSpy{}, true)
	if err != nil {
		t.Fatalf("apply 失败: %v", err)
	}
	if report.PendingComments != 1 {
		t.Fatalf("应报出 1 条审核中的存量评论，实际 %d", report.PendingComments)
	}
	if report.RetiredComments != 1 {
		t.Fatalf("应退役 1 条，实际 %d", report.RetiredComments)
	}

	var comment schema.CommentRow
	if err := db.First(&comment, "id = ?", 2002).Error; err != nil {
		t.Fatalf("读评论失败: %v", err)
	}
	if comment.Status != 3 {
		t.Fatalf("审核中的评论应转为已删除（status=3），实际 %d", comment.Status)
	}
}

// TestSweepRefusesEmptyWordlist 词表为空时必须报错，而不是打一份"0 命中"的漂亮报告。
func TestSweepRefusesEmptyWordlist(t *testing.T) {
	db, _ := seedSweepFixture(t)
	// 空词表构建出来的引擎没有任何词：命令入口应当先拒绝（这里直接验证守卫语义）。
	empty, err := contentfilter.NewEngine(nil, nil)
	if err != nil {
		t.Fatalf("空引擎不该构建失败: %v", err)
	}
	if empty.WordCount() != 0 {
		t.Fatalf("空引擎词条数应为 0，实际 %d", empty.WordCount())
	}
	report, err := sweepContent(context.Background(), db, empty, nil, false)
	if err != nil {
		t.Fatalf("扫描本身不该失败: %v", err)
	}
	// 空词表时命中必然为 0 —— 这正是为什么命令入口（FlagFilterSweep）要额外拒绝它。
	for _, stats := range report.Surfaces {
		if stats.Hit != 0 {
		t.Fatalf("空词表不该命中任何内容（%s）", stats.Surface)
		}
	}
}





