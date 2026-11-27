package config

import (
	"os"
	"testing"

	"myblogx/conf"

	"gopkg.in/yaml.v3"
)

// 第 11 段 content_filter（ADR-0032）的两件事：
//   - 段登记与解码（默认值只放非涉政种子，字段级合并照常）；
//   - 默认值文件里的种子词条形状（词表只服务替换与关键词风险页）。
//
// 首启时把旧 site.article.moderation / site.comment.moderation 合并进新段的迁移逻辑
// 已随「文章词表机审退场」删除（票 401）：旧配置结构不再存在，合并的来源与目标
// 都没有了。

func cfWord(word, tier, category string) conf.ContentFilterWord {
	return conf.ContentFilterWord{Word: word, Tier: tier, Category: category}
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

// 老库里那段 JSON 还是「加 auto_dispose 之前」的形状时，解码必须回落 release/release。
//
// 为什么这条不写成"空串也按 mask 解释"就够：库里存的空串会让后台下拉框显示空白，
// 运营看到的是"没配置"而不是"配了只打星"；而默认行为必须与改造前逐字节一致。
// 回落的来源是默认值文件（深度合并补缺字段），所以这里连默认值文件一起验 ——
// 少了 yaml 里那一段，这条用例就是红的，不会安静地存下一份空口径。
func TestContentFilterAutoDisposeFallsBackToReleaseOnOldSection(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var defaults conf.RuntimeDefaults
	if err := yaml.Unmarshal(raw, &defaults); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}
	if defaults.ContentFilter.AutoDispose.BlockAction != "release" ||
		defaults.ContentFilter.AutoDispose.WatchAction != "release" {
		t.Fatalf("默认值文件里的 auto_dispose 应为 release/release，实际 %+v",
			defaults.ContentFilter.AutoDispose)
	}

	got, err := decodeSection(SectionContentFilter, defaults.ContentFilter, map[string]string{
		// 老库：只有词表 / 白名单 / 保留天数这几个老字段，没有 auto_dispose。
		SectionContentFilter: `{"words":[{"word":"冰毒","tier":"block","category":"drugs"}],` +
			`"whitelist":[],"whitelist_phrases":[],"retain_days":90}`,
	})
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if got.AutoDispose.BlockAction != "release" || got.AutoDispose.WatchAction != "release" {
		t.Fatalf("老库缺 auto_dispose 时应回落 release/release，实际 %+v", got.AutoDispose)
	}
	if len(got.Words) != 1 || got.Words[0].Word != "冰毒" {
		t.Fatalf("老字段不该被这次回落影响，实际 %+v", got.Words)
	}
}
