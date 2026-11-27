package config

import (
	"os"
	"testing"

	"myblogx/internal/article/domain"

	"gopkg.in/yaml.v3"
)

// 标签上限（一篇文章最多可选几个）的默认值必须两处一致。
//
// 与投稿配额同一口径：site_default_settings.yaml 是"库里没有这段配置时"的
// 唯一来源，代码里的 domain.MaxArticleTags 是归一化回落与"合法上限边界"的
// 唯一来源。两处漂移会让新部署站点的默认上限与代码回落值不同，而两边都
// 不会报错 —— 只有新装站点的作者会看到与老站点不一样的上限。
//
// 这里只钉"默认值"这一条，不钉归一化逻辑（它在 domain 包的
// TestNormalizeTagLimitFallsBackOnIllegalValues 里单独覆盖）。

func TestDefaultSettingsYaml_标签上限默认值(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site struct {
			Article struct {
				MaxTagsPerArticle *int `yaml:"max_tags_per_article"`
			} `yaml:"article"`
		} `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	got := file.Site.Article.MaxTagsPerArticle
	if got == nil {
		t.Fatal("site_default_settings.yaml 的 site.article 必须显式写 max_tags_per_article（缺键会静默变成 0）")
	}
	if *got != domain.MaxArticleTags {
		t.Fatalf("默认值文件写的是 %d，代码常量是 %d，两处必须一致", *got, domain.MaxArticleTags)
	}
	if domain.MaxArticleTags != 5 {
		t.Fatalf("标签默认上限应为 5（产品决定），实际 %d", domain.MaxArticleTags)
	}

	// 默认值必须落在合法区间内：否则 NormalizeTagLimit 会把每个合法配置值
	// 都当成越界、统统回落，后台"可改"这件事就名存实亡且没有任何测试会红。
	if domain.MaxArticleTags < domain.MinArticleTagLimit || domain.MaxArticleTags > domain.MaxArticleTagLimit {
		t.Fatalf("默认上限 %d 不在合法区间 [%d, %d] 内",
			domain.MaxArticleTags, domain.MinArticleTagLimit, domain.MaxArticleTagLimit)
	}
}
