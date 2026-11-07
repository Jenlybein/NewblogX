// Package skills 读取「运行时策略 skill」文本：产品进程启动时按
// Agent Skills 开放规范（https://agentskills.io/specification）扫目录，
// 只把名称与描述装进目录层，正文用到时才从磁盘读。
//
// 为什么它是平台包：格式遵循开放规范（谁拿走都能直接用），实现不 import 任何
// 业务包；放在 internal/<业务模块>/ 会被 archcheck 当业务模块限制（R10/R11），
// 而这是共享设施。将来要对外发布成独立库，整包移出去零改动。
//
// 渐进披露的两层在这里：
//
//	Load   —— Step 1 Discover + Step 2 Parse：只解析 frontmatter
//	List   —— Step 3 Disclose：调用方只拿到 name + description
//	Body   —— Step 4 Activate：真正要用正文时才读盘
//
// 校验是严格的：名称与目录名不一致、描述超长、正文为空、metadata 值不是字符串，
// 一律返回点名具体文件的错误 —— 由调用方在启动阶段把它变成"进程起不来"，
// 而不是让用户先发现（ADR-0033）。
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skillFileName 是规范里每个 skill 目录下唯一的入口文件名。
const skillFileName = "SKILL.md"

// hashPrefixLen 是版本号的长度：sha256 十六进制前 12 位，够短到能落库，
// 也够长到不同修订不会撞车。
const hashPrefixLen = 12

// Info 是目录层的一条：只有名称与描述。正文不在这里 —— 目录不该背着正文。
type Info struct {
	Name        string
	Description string
}

// Catalog 是一次加载的结果，按名称排序，可安全地只读共享。
type Catalog struct {
	infos  []Info
	byName map[string]*Skill
}

// List 返回目录层（名称 + 描述），按名称升序。返回副本，调用方改不动目录。
func (c *Catalog) List() []Info {
	if c == nil {
		return nil
	}
	out := make([]Info, len(c.infos))
	copy(out, c.infos)
	return out
}

// Len 报告加载到多少份 skill。
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.infos)
}

// Get 按名称取一份 skill，供后续按场景取正文的消费点使用。
func (c *Catalog) Get(name string) (*Skill, bool) {
	if c == nil {
		return nil, false
	}
	s, ok := c.byName[name]
	return s, ok
}

// Skill 是一份已校验通过的 skill 的元数据。正文不驻留内存：Body 按需读盘。
type Skill struct {
	name          string
	description   string
	path          string
	hash          string
	license       string
	compatibility string
	allowedTools  string
	metadata      map[string]string
}

// Name 是 frontmatter 里的 name，等于父目录名。
func (s *Skill) Name() string { return s.name }

// Description 是"什么时候用这份 skill"的一句话，触发全靠它。
func (s *Skill) Description() string { return s.description }

// Path 是 SKILL.md 的路径，用于报错与排查。
func (s *Skill) Path() string { return s.path }

// Hash 是 SKILL.md 字节的 sha256 前 12 位：线上跑的是哪一版由它决定，
// 可以随调用记录落库。
func (s *Skill) Hash() string { return s.hash }

// License 是可选字段，空串表示没写。
func (s *Skill) License() string { return s.license }

// Compatibility 是可选字段，空串表示没写。
func (s *Skill) Compatibility() string { return s.compatibility }

// AllowedTools 是可选字段：空格分隔的工具名，空串表示没写。
func (s *Skill) AllowedTools() string { return s.allowedTools }

// Metadata 是产品私有信息的唯一去处（map[string]string）。返回副本。
func (s *Skill) Metadata() map[string]string {
	out := make(map[string]string, len(s.metadata))
	for k, v := range s.metadata {
		out[k] = v
	}
	return out
}

// Body 按需读取正文（frontmatter 之后的 Markdown），两端空白去掉。
//
// 启动时校验正文非空，但不驻留：正文可能几百行，20 份 skill 不该在启动时
// 占 20 份内存与日志体积。
func (s *Skill) Body() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", fmt.Errorf("读取运行时策略 skill 正文失败: %s: %w", s.path, err)
	}
	_, body, err := splitFrontmatter(string(data))
	if err != nil {
		return "", &FileError{Path: s.path, Reason: err.Error()}
	}
	return strings.TrimSpace(body), nil
}

// FileError 是某一份 SKILL.md 不合规：路径与原因都在错误里，
// 运维看到的第一行就能定位到文件。
type FileError struct {
	Path   string
	Reason string
}

func (e *FileError) Error() string {
	return fmt.Sprintf("运行时策略 skill 文件 %s 不合规: %s", e.Path, e.Reason)
}

// Load 扫 roots 下每个子目录的 SKILL.md，校验 frontmatter 后返回目录层。
//
// roots 来自进程级配置（BLOGX_SKILLS_DIRS，逗号分隔）。任何一份不合规、
// 任何一个目录读不到，都返回错误、不返回半个目录：调用方据此让进程起不来，
// 不存在代码内兜底文本。
func Load(roots []string) (*Catalog, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("没有配置任何运行时策略 skill 目录（BLOGX_SKILLS_DIRS）")
	}

	cat := &Catalog{byName: make(map[string]*Skill)}

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("运行时策略 skill 目录读取失败: %s: %w", root, err)
		}
		for _, entry := range entries {
			// 根下可以放 README.md 这类普通文件：它们不是 skill。
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dirName := entry.Name()
			path := filepath.Join(root, dirName, skillFileName)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("运行时策略 skill 子目录 %s 缺少可读的 %s: %w", filepath.Join(root, dirName), skillFileName, err)
			}

			skill, err := parseSkillFile(path, dirName, data)
			if err != nil {
				return nil, err
			}
			if prev, dup := cat.byName[skill.name]; dup {
				// 同名两份：说明两份 root 里有同一个 skill 的两个版本，
				// 谁生效取决于扫描顺序 —— 这种不确定性必须在启动时炸掉。
				return nil, fmt.Errorf(
					"运行时策略 skill 名称重复: %q（%s 与 %s）",
					skill.name, prev.path, skill.path,
				)
			}
			cat.byName[skill.name] = skill
			cat.infos = append(cat.infos, Info{Name: skill.name, Description: skill.description})
		}
	}

	sort.Slice(cat.infos, func(i, j int) bool { return cat.infos[i].Name < cat.infos[j].Name })
	return cat, nil
}

// parseSkillFile 校验一份 SKILL.md 并构造 Skill。
//
// data 是文件字节：版本号按字节算，因此在这里一并算出。
func parseSkillFile(path, dirName string, data []byte) (*Skill, error) {
	fm, body, err := splitFrontmatter(string(data))
	if err != nil {
		return nil, &FileError{Path: path, Reason: err.Error()}
	}

	fields, err := parseFrontmatter(fm)
	if err != nil {
		return nil, &FileError{Path: path, Reason: err.Error()}
	}

	if err := fields.validate(path, dirName, body); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(data)
	return &Skill{
		name:          fields.name,
		description:   fields.description,
		path:          path,
		hash:          hex.EncodeToString(sum[:])[:hashPrefixLen],
		license:       fields.license,
		compatibility: fields.compatibility,
		allowedTools:  fields.allowedTools,
		metadata:      fields.metadata,
	}, nil
}
