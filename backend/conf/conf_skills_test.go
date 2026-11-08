package conf_test

import (
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/utils/envyaml"
)

// 目录列表是"进程级配置 → 加载器"的唯一形状：逗号分隔，空白与空项去掉，
// 全空回落到默认 skills（进程工作目录下的 skills/）。
func TestSkillsDirsList(t *testing.T) {
	cases := []struct {
		name string
		dirs string
		want []string
	}{
		{name: "没配置用默认值", dirs: "", want: []string{conf.DefaultSkillsDirs}},
		{name: "只有空白也用默认值", dirs: "   ", want: []string{conf.DefaultSkillsDirs}},
		{name: "单个目录", dirs: "skills", want: []string{"skills"}},
		{name: "多个目录去空白与空项", dirs: " skills , ../other-skills ,, ", want: []string{"skills", "../other-skills"}},
		{name: "开发时指向仓库根的 skills", dirs: "../skills", want: []string{"../skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := conf.Skills{Dirs: tc.dirs}.DirsList()
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("DirsList() = %v，期望 %v", got, tc.want)
			}
		})
	}
}

// settings.yaml 里的占位符是运维的实际入口：留空要能落到默认值，
// 设了 BLOGX_SKILLS_DIRS 要原样生效（逗号分隔多目录）。
func TestSkillsSectionReadsEnvPlaceholder(t *testing.T) {
	t.Run("留空回落到默认值", func(t *testing.T) {
		t.Setenv("BLOGX_SKILLS_DIRS", "")
		var cfg struct {
			Skills conf.Skills `yaml:"skills"`
		}
		if err := envyaml.Unmarshal([]byte("skills:\n  dirs: \"${BLOGX_SKILLS_DIRS:-}\"\n"), &cfg); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if got := cfg.Skills.DirsList(); len(got) != 1 || got[0] != conf.DefaultSkillsDirs {
			t.Fatalf("DirsList() = %v，期望默认 %q", got, conf.DefaultSkillsDirs)
		}
	})

	t.Run("环境变量覆盖", func(t *testing.T) {
		t.Setenv("BLOGX_SKILLS_DIRS", "../skills, .agents/skills")
		var cfg struct {
			Skills conf.Skills `yaml:"skills"`
		}
		if err := envyaml.Unmarshal([]byte("skills:\n  dirs: \"${BLOGX_SKILLS_DIRS:-}\"\n"), &cfg); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if got := strings.Join(cfg.Skills.DirsList(), "|"); got != "../skills|.agents/skills" {
			t.Fatalf("DirsList() = %q，期望 %q", got, "../skills|.agents/skills")
		}
	})
}
