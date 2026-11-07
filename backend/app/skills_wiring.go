package app

import (
	"fmt"

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
