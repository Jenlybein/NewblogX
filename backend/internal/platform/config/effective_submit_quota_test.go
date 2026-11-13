package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// 每日投稿上限（票 304 / ADR-0021）的默认值与合法区间。
//
// 与私信限流同一口径：默认值与区间只在生效值模块声明一次，越界值必须收敛 ——
// 填 0 或负数若按字面执行，全站用户当天一篇文章都提交不了。

func TestNormalizeArticleSubmitDailyMax(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"缺省回落", 0, DefaultArticleSubmitDailyMax},
		{"负数回落", -3, DefaultArticleSubmitDailyMax},
		{"超上限回落", 100000, DefaultArticleSubmitDailyMax},
		{"合法值原样", 5, 5},
		{"下界内", 1, 1},
		{"上界内", 9999, 9999},
		{"默认值本身合法", DefaultArticleSubmitDailyMax, DefaultArticleSubmitDailyMax},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeArticleSubmitDailyMax(c.in); got != c.want {
				t.Fatalf("NormalizeArticleSubmitDailyMax(%d) = %d，期望 %d", c.in, got, c.want)
			}
		})
	}
}

// TestDefaultSettingsYaml_投稿配额默认值15 钉住"字面默认值"这一条。
//
// site_default_settings.yaml 是"库里没有这段配置时"的唯一来源；代码里的
// DefaultArticleSubmitDailyMax 是归一化与 Redis 兜底的唯一来源。两处漂移会让
// 新部署的站点上限与老站点不同，而两边都不会报错。
func TestDefaultSettingsYaml_投稿配额默认值15(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site struct {
			Article struct {
				SubmitDailyMax *int `yaml:"submit_daily_max"`
			} `yaml:"article"`
		} `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}
	got := file.Site.Article.SubmitDailyMax
	if got == nil {
		t.Fatal("site_default_settings.yaml 的 site.article 必须显式写 submit_daily_max（缺键会静默变成 0）")
	}
	if *got != DefaultArticleSubmitDailyMax {
		t.Fatalf("默认值文件写的是 %d，代码常量是 %d，两处必须一致", *got, DefaultArticleSubmitDailyMax)
	}
	if DefaultArticleSubmitDailyMax != 15 {
		t.Fatalf("投稿配额默认值应为每自然日 15 次（spec 决策 19），实际 %d", DefaultArticleSubmitDailyMax)
	}
}
