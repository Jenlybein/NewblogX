package app

import (
	"fmt"
	"strings"

	"myblogx/conf"
	"myblogx/internal/platform/skills"
)

// LoadSkills 是运行时策略文本的启动入口：读进程级配置里的目录，加载并校验。
//
// 它**不做**代码内兜底文本：目录读不到、某份 SKILL.md 不合规，都原样返回错误，
// 由 main 把它变成"进程起不来"。用户看到的第一条错误必须是运维看到的那一条，
// 而不是线上回答变得莫名其妙（ADR-0030）。
//
// 启动阶段只读目录层（名称 + 描述），正文留到消费点按需读 —— 这里返回的
// Catalog 不持有正文。
func LoadSkills(cfg *conf.Config) (*skills.Catalog, error) {
	var dirs []string
	if cfg != nil {
		dirs = cfg.Skills.DirsList()
	} else {
		dirs = conf.Skills{}.DirsList()
	}

	catalog, err := skills.Load(dirs)
	if err != nil {
		return nil, fmt.Errorf("初始化运行时策略 skill 失败: %w", err)
	}
	return catalog, nil
}

// ValidateSceneSkillBindings 校验运行期配置里「场景 → skill」的每个引用都真实存在。
//
// 为什么必须在启动期查：`ai.scene_skills` 是**运行期数据**（后台可改），
// 引用一个不存在的 skill 不会立刻弄坏进程，只会在那个场景第一次被调用时
// 报 ai.policy_skill_missing —— 那时用户已经在等回答了。把校验挪到启动期，
// 配置错误在部署时就暴露，而不是变成线上的一次随机失败（ADR-0030 决策：
// "引用了未知场景就启动失败"）。
//
// 空名单是合法的（场景不绑定任何策略文本），不在这里报错。
func ValidateSceneSkillBindings(aiConf conf.AI, catalog *skills.Catalog) error {
	for scene, names := range aiConf.SceneSkills {
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if catalog == nil {
				return fmt.Errorf("运行时策略 skill 目录未加载，但场景 %q 引用了 %q", scene, name)
			}
			if _, ok := catalog.Get(name); !ok {
				return fmt.Errorf("场景 %q 引用了不存在的运行时策略 skill %q（启动失败，不做兜底）", scene, name)
			}
		}
	}
	return nil
}