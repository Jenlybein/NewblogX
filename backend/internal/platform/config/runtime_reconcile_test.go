package config

import (
	"os"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"

	"gopkg.in/yaml.v3"
)

// 并账周期的配置形状（#279）。
//
// 两件事必须一起守住：
//   - 默认值只在 site_default_settings.yaml 里写一份（深度合并负责把老库缺的键补上），
//     所以老库不需要任何数据迁移；
//   - 老库的 site 段没有 reconcile 子块时，必须从默认值补出 30 分钟，
//     而不是补出 0 —— 0 会让周期回落到默认值，看起来"对"，但那是巧合而不是口径。

func TestDefaultSettingsYaml_并账周期显式写下30分钟(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site conf.Site `yaml:"site"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	if got := file.Site.Reconcile.IntervalMinutes; got != 30 {
		t.Fatalf("site.reconcile.interval_minutes 默认值应为 30，实际 %d", got)
	}
	// 只解析出 30 还不够：缺键同样能靠结构体默认值凑出来。原文里必须真有这一行。
	if !strings.Contains(string(raw), "interval_minutes: 30") {
		t.Fatal("site_default_settings.yaml 必须显式写 interval_minutes: 30，实际内容里没有这一行")
	}
}

func TestDecodeSection_老载荷没有并账周期时按默认值补齐(t *testing.T) {
	defaults := conf.Site{Reconcile: siteconf.Reconcile{IntervalMinutes: 30}}
	got, err := decodeSection(SectionSite, defaults, map[string]string{
		SectionSite: `{"site_info":{"beian":"老备案"},"chat":{"stranger_msg_enabled":true}}`,
	})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.Reconcile.IntervalMinutes != 30 {
		t.Fatalf("老载荷缺 reconcile 子块时应补默认 30 分钟，实际 %d", got.Reconcile.IntervalMinutes)
	}
	if !got.Chat.StrangerMsgEnabled {
		t.Fatal("同段内已有的 chat 子块应保留")
	}
}

func TestDecodeSection_后台写过的并账周期优先于默认值(t *testing.T) {
	defaults := conf.Site{Reconcile: siteconf.Reconcile{IntervalMinutes: 30}}
	got, err := decodeSection(SectionSite, defaults, map[string]string{
		SectionSite: `{"reconcile":{"interval_minutes":5}}`,
	})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.Reconcile.IntervalMinutes != 5 {
		t.Fatalf("库里写过的周期应生效，实际 %d", got.Reconcile.IntervalMinutes)
	}
}
