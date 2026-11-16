package config

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/platform/secretbox"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 第 11 段 content_filter（ADR-0032）的三件事：
//   - 段登记与解码（默认值只放非涉政种子，字段级合并照常）；
//   - 首启迁移（旧的两份词表只在"新段还没出现"时合并一次）；
//   - 合并后不会再复活（哪怕运营把词删空）。

func cfWord(word, tier, category string) conf.ContentFilterWord {
	return conf.ContentFilterWord{Word: word, Tier: tier, Category: category}
}

// newContentFilterService 与 newTestService 相同，但日志写进给定缓冲区 ——
// 合并必须留下日志，而 newTestService 的 logger 指向 io.Discard。
func newContentFilterService(
	t *testing.T, defaults conf.RuntimeDefaults, logs *bytes.Buffer,
) (*RuntimeConfigService, *gorm.DB) {
	t.Helper()
	dsn := "file:cf_" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	if err := db.AutoMigrate(&runtimeConfigRow{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	logger := logrus.New()
	logger.SetOutput(logs)
	logger.SetLevel(logrus.InfoLevel)
	svc := NewRuntimeConfigService(defaults, logger, db)
	box, err := secretbox.New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatalf("构造测试主密钥失败: %v", err)
	}
	svc.SetSealer(box)
	return svc, db
}

func TestContentFilterSection_登记与字段级合并(t *testing.T) {
	if _, ok := LookupSection(SectionContentFilter); !ok {
		t.Fatal("第 11 段 content_filter 未在注册表登记")
	}

	// 库里那段只有 words、没有 whitelist：words 整段替换、whitelist 回退默认值。
	defaults := conf.ContentFilter{
		Words:     []conf.ContentFilterWord{cfWord("默认词", "watch", "fraud")},
		Whitelist: []string{"默认白名单"},
	}
	got, err := decodeSection(SectionContentFilter, defaults, map[string]string{
		SectionContentFilter: `{"words":[{"word":"冰毒","tier":"block","category":"drugs"}]}`,
	})
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if len(got.Words) != 1 || got.Words[0].Word != "冰毒" || got.Words[0].Tier != "block" {
		t.Fatalf("词表应整段替换为库里的值，实际 %+v", got.Words)
	}
	if len(got.Whitelist) != 1 || got.Whitelist[0] != "默认白名单" {
		t.Fatalf("白名单缺失字段应回退默认值，实际 %+v", got.Whitelist)
	}
}

// 默认值文件里只放非涉政种子词：涉政/暴恐词只进数据库（ADR-0032 决策 8）。
func TestDefaultSettingsYaml_contentFilter只含非涉政种子(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var doc struct {
		ContentFilter conf.ContentFilter `yaml:"content_filter"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}
	if len(doc.ContentFilter.Words) == 0 {
		t.Fatal("默认值里应有非涉政种子词（否则新站点上线时词表是空的）")
	}
	for _, entry := range doc.ContentFilter.Words {
		if entry.Category == "politics" || entry.Category == "violence" {
			t.Errorf("默认值里不应出现涉政/暴恐词条: %+v", entry)
		}
		if entry.Tier != "block" && entry.Tier != "watch" {
			t.Errorf("档位只有 block / watch，实际 %+v", entry)
		}
		if len([]rune(entry.Word)) < 2 {
			t.Errorf("单字词不入表，实际 %+v", entry)
		}
	}
}

// 首次上线：库里还没有 content_filter 段 → 合并旧的两份词表，并留下日志。
func TestInitRuntimeConfig_新段首次出现时合并旧词表(t *testing.T) {
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{
			Article: siteconf.Article{Moderation: siteconf.Moderation{
				BlockWords: []string{"代开发票"},
				WatchWords: []string{"加微信"},
			}},
			Comment: siteconf.Comment{Moderation: siteconf.Moderation{
				// 与文章重复的词必须只留一条；评论特有的词也要并进来。
				BlockWords: []string{"代开发票", "刷单返利"},
				WatchWords: []string{"加微信", "私聊"},
			}},
		},
		ContentFilter: conf.ContentFilter{
			Words: []conf.ContentFilterWord{cfWord("办假证", "block", "fake_docs")},
		},
	}
	var logs bytes.Buffer
	svc, db := newContentFilterService(t, defaults, &logs)

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	got := svc.GetRuntimeContentFilter()
	tiers := make(map[string]string, len(got.Words))
	for _, entry := range got.Words {
		tiers[entry.Word] = entry.Tier
	}
	want := map[string]string{
		"办假证":  "block", // 默认种子词保留
		"代开发票": "block", // 旧词（文章 + 评论各一份，只留一条）
		"刷单返利": "block",
		"加微信":  "watch",
		"私聊":   "watch",
	}
	if len(got.Words) != len(want) {
		t.Fatalf("合并后应有 %d 条词，实际 %d: %+v", len(want), len(got.Words), got.Words)
	}
	for word, tier := range want {
		if tiers[word] != tier {
			t.Errorf("词 %q 的档位应为 %q，实际 %q", word, tier, tiers[word])
		}
	}

	// 合并结果必须真的落库（否则下一次启动会再合并一遍，运营删掉的词会复活）。
	if payload := readPayload(t, db, SectionContentFilter); !strings.Contains(payload, "代开发票") {
		t.Fatalf("合并结果没有写进库里，实际 %s", payload)
	}
	if !strings.Contains(logs.String(), "已合并旧词表") {
		t.Fatalf("合并应留下一条日志，实际日志: %s", logs.String())
	}
}

// 新段已存在（哪怕是运营自己配的）→ 不合并旧词。
func TestInitRuntimeConfig_新段非空时不合并旧词(t *testing.T) {
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{Article: siteconf.Article{Moderation: siteconf.Moderation{
			BlockWords: []string{"旧词"},
		}}},
	}
	var logs bytes.Buffer
	svc, _ := newContentFilterService(t, defaults, &logs)
	// 先把段写进库：模拟"这一版之前已经上线过、运营也维护过词表"。
	preset := conf.ContentFilter{
		Words:     []conf.ContentFilterWord{cfWord("运营词", "watch", "fraud")},
		Whitelist: []string{"白名单词"},
	}
	if err := svc.UpdateContentFilter(preset); err != nil {
		t.Fatalf("预置词表失败: %v", err)
	}

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	got := svc.GetRuntimeContentFilter()
	if len(got.Words) != 1 || got.Words[0].Word != "运营词" {
		t.Fatalf("新段非空时不应合并旧词，实际 %+v", got.Words)
	}
	if len(got.Whitelist) != 1 || got.Whitelist[0] != "白名单词" {
		t.Fatalf("新段非空时白名单也不该被动过，实际 %+v", got.Whitelist)
	}
}

// 合并只发生一次：运营把词删空之后重启，不会把旧词捞回来。
func TestInitRuntimeConfig_合并只发生一次(t *testing.T) {
	defaults := conf.RuntimeDefaults{
		Site: conf.Site{Article: siteconf.Article{Moderation: siteconf.Moderation{
			BlockWords: []string{"旧词"},
		}}},
	}
	var logs bytes.Buffer
	svc, db := newContentFilterService(t, defaults, &logs)
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("首次初始化失败: %v", err)
	}
	if len(svc.GetRuntimeContentFilter().Words) == 0 {
		t.Fatal("首次启动应合并出词条")
	}

	// 运营清空词表（先把种子词也删掉，模拟"从零开始维护"）。
	if err := svc.UpdateContentFilter(conf.ContentFilter{}); err != nil {
		t.Fatalf("清空词表失败: %v", err)
	}

	// 重启：新服务读同一个库，段已经存在 → 不再合并。
	restarted := NewRuntimeConfigService(defaults, quietLogger(), db)
	restarted.SetSealer(testSealer(t))
	if err := restarted.InitRuntimeConfig(); err != nil {
		t.Fatalf("重启初始化失败: %v", err)
	}
	if words := restarted.GetRuntimeContentFilter().Words; len(words) != 0 {
		t.Fatalf("重启不应复活旧词，实际 %+v", words)
	}
}

// 合并的纯函数部分：跨两份词表去重、档位映射、类目统一落到 fake_docs。
func TestMergeModerationLists_去重与档位映射(t *testing.T) {
	current := conf.ContentFilter{
		Words:     []conf.ContentFilterWord{cfWord("既有词", "block", "drugs"), cfWord("PayPal", "watch", "fraud")},
		Whitelist: []string{"白名单"},
	}
	article := siteconf.Moderation{BlockWords: []string{"代开发票"}, WatchWords: []string{"paypal"}}
	comment := siteconf.Moderation{BlockWords: []string{"代开发票"}, WatchWords: []string{"私聊"}}

	merged, added := mergeModerationLists(current, article, comment)
	if added != 2 {
		t.Fatalf("应新增 2 条（代开发票 / 私聊），实际 %d", added)
	}
	if len(merged.Words) != 4 {
		t.Fatalf("合并后应有 4 条，实际 %+v", merged.Words)
	}
	byWord := make(map[string]conf.ContentFilterWord, len(merged.Words))
	for _, entry := range merged.Words {
		byWord[entry.Word] = entry
	}
	if entry := byWord["代开发票"]; entry.Tier != "block" || entry.Category != "fake_docs" {
		t.Fatalf("旧阻断词应映射成 block + fake_docs，实际 %+v", entry)
	}
	if entry := byWord["私聊"]; entry.Tier != "watch" || entry.Category != "fake_docs" {
		t.Fatalf("旧观察词应映射成 watch + fake_docs，实际 %+v", entry)
	}
	if _, dup := byWord["paypal"]; dup {
		t.Fatal("大小写不同但同形的词不应重复入表")
	}
	if len(merged.Whitelist) != 1 || merged.Whitelist[0] != "白名单" {
		t.Fatalf("合并不应动白名单，实际 %+v", merged.Whitelist)
	}
	// 纯函数：不得改动入参里的切片。
	if len(current.Words) != 2 {
		t.Fatalf("合并不能原地改写当前段，实际 %+v", current.Words)
	}
}
