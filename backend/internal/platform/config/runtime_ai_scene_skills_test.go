package config

import (
	"os"
	"testing"

	"myblogx/conf"

	"gopkg.in/yaml.v3"
)

// 场景 ↔ skill 的绑定（#310）。
//
// 验收口径是「对应关系在配置里，不在 skill 文件里」：默认值文件里必须真的
// 写出一条 search_answer → jelly-voice 的绑定，而不是靠代码里的隐式约定。
// 这条守卫盯的是**唯一那份默认配置**：删掉它，捏捏就退回没有口径的回答。
func TestDefaultSettingsYaml_搜索回答绑定捏捏的口径skill(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		AI conf.AI `yaml:"ai"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	names := file.AI.SceneSkills["search_answer"]
	if len(names) != 1 || names[0] != "jelly-voice" {
		t.Fatalf("ai.scene_skills.search_answer = %v，期望 [jelly-voice]", names)
	}
}
