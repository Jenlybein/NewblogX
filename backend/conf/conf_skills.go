// 运行时策略 skill 配置

package conf

import "strings"

// DefaultSkillsDirs 是进程级配置没写 skills.dirs 时的默认值。
//
// 相对**进程工作目录**解析：仓库根 skills/ 就是默认布局（ADR-0030）。
// 容器里由镜像把 skills/ 放到 /app/skills；本地从 backend/ 启动时
// 在 .envrc 里给 BLOGX_SKILLS_DIRS='../skills'（.envrc.example 有例子）。
const DefaultSkillsDirs = "skills"

// Skills 是进程级的「运行时策略 skill」目录配置。
//
// 为什么进 settings.yaml 而不是数据库运行时配置：读不到就不启动影响的是
// 进程怎么跑，不是某次业务请求的行为；把它放进能热改的那一层，
// 就会出现"进程起来了，但策略文本没了"的中间态。
type Skills struct {
	// Dirs 是逗号分隔的目录列表，逐个扫描 <root>/*/SKILL.md。
	Dirs string `yaml:"dirs"`
}

// DirsList 把逗号分隔的配置拆成目录列表：去空白、丢空项；全空回落到默认值。
//
// 默认值只在这里定义一份：settings.yaml 里写的是空占位符
// （${BLOGX_SKILLS_DIRS:-}），不在 yaml 里再抄一个 "skills"，
// 否则两处默认值迟早对不上。
func (s Skills) DirsList() []string {
	var out []string
	for _, part := range strings.Split(s.Dirs, ",") {
		if dir := strings.TrimSpace(part); dir != "" {
			out = append(out, dir)
		}
	}
	if len(out) == 0 {
		return []string{DefaultSkillsDirs}
	}
	return out
}
