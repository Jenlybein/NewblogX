package skills

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// 规范硬性约束（校验器逐条检查，不通过即启动失败）。
const (
	maxNameLen          = 64
	maxDescriptionLen   = 1024
	maxCompatibilityLen = 500

	frontmatterDelimiter = "---"
)

// namePattern 是规范对 name 的形状：小写字母 / 数字 / 连字符，
// 不以连字符开头或结尾（连字符也是目录名，必须能被文件系统与 URL 友好地接受）。
var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// frontmatterFields 是 frontmatter 解析后的原始形状，尚未校验。
type frontmatterFields struct {
	name          string
	description   string
	license       string
	compatibility string
	allowedTools  string
	metadata      map[string]string
}

// splitFrontmatter 把 SKILL.md 拆成 frontmatter 与正文。
//
// 只认文件开头的一对 `---`：没有 frontmatter 就不是 skill，不做"尽力猜"。
func splitFrontmatter(text string) (yamlText string, body string, err error) {
	lines := strings.Split(text, "\n")
	// 去掉可能的 BOM：文件被 Windows 编辑器存过是常事。
	lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	if strings.TrimSpace(lines[0]) != frontmatterDelimiter {
		return "", "", fmt.Errorf("缺少 YAML frontmatter：文件必须以 %s 开头", frontmatterDelimiter)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == frontmatterDelimiter {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", "", fmt.Errorf("YAML frontmatter 没有结束标记 %s", frontmatterDelimiter)
}

// parseFrontmatter 只解析字段，不做取值范围校验（校验在 validate 里，
// 那里才拿得到目录名与正文）。
func parseFrontmatter(text string) (*frontmatterFields, error) {
	fields := &frontmatterFields{metadata: map[string]string{}}
	if strings.TrimSpace(text) == "" {
		// 空 frontmatter 不在这里报：下面 validate 会点明缺 name / description。
		return fields, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("frontmatter 不是合法 YAML: %v", err)
	}
	node := &doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return fields, nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter 必须是 YAML 映射（key: value 形式），实际是 %s", describeNode(node))
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]

		var err error
		switch key {
		case "name":
			fields.name, err = scalarString(key, value)
		case "description":
			fields.description, err = scalarString(key, value)
		case "license":
			fields.license, err = scalarString(key, value)
		case "compatibility":
			fields.compatibility, err = scalarString(key, value)
		case "allowed-tools":
			fields.allowedTools, err = scalarString(key, value)
		case "metadata":
			fields.metadata, err = stringMap(value)
		default:
			// 未知顶层字段：容忍。开放规范会加字段，我们只认自己用得到的；
			// "产品私有信息必须写进 metadata"是写作约定，由门禁与评审守，
			// 不在这里硬拒 —— 否则规范一加字段，服务就起不来。
		}
		if err != nil {
			return nil, err
		}
	}
	return fields, nil
}

// validate 按规范逐条校验，并把目录名与正文一起对账。
func (f *frontmatterFields) validate(path, dirName, body string) error {
	violation := func(format string, args ...any) error {
		return &FileError{Path: path, Reason: fmt.Sprintf(format, args...)}
	}

	if f.name == "" {
		return violation("缺少必填字段 name")
	}
	if n := utf8.RuneCountInString(f.name); n > maxNameLen {
		return violation("name 长度 %d 超过 %d 字符上限", n, maxNameLen)
	}
	if !namePattern.MatchString(f.name) {
		return violation("name = %q 不合规：只能用小写字母、数字与连字符，且不能以连字符开头或结尾", f.name)
	}
	if f.name != dirName {
		return violation("name = %q 与目录名 %q 不一致（规范要求两者相同）", f.name, dirName)
	}

	if f.description == "" {
		return violation("缺少必填字段 description（写「什么时候用这份 skill」，触发全靠它）")
	}
	if n := utf8.RuneCountInString(f.description); n > maxDescriptionLen {
		return violation("description 长度 %d 超过 %d 字符上限", n, maxDescriptionLen)
	}

	if n := utf8.RuneCountInString(f.compatibility); n > maxCompatibilityLen {
		return violation("compatibility 长度 %d 超过 %d 字符上限", n, maxCompatibilityLen)
	}

	if strings.TrimSpace(body) == "" {
		return violation("正文为空：frontmatter 之后必须是不为空的 Markdown 正文")
	}
	return nil
}

// scalarString 取一个字符串标量。数字、布尔、null 都不算字符串 ——
// 规范要求这几个字段就是文本。
func scalarString(field string, node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", fmt.Errorf("%s 必须是字符串，实际是 %s", field, describeNode(node))
	}
	return strings.TrimSpace(node.Value), nil
}

// stringMap 取 metadata：**只能是 map[string]string**。
// 嵌套结构（列表 / 映射）与数字、布尔一律拒绝，免得下游按字符串取值时踩空。
func stringMap(node *yaml.Node) (map[string]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("metadata 必须是 map[string]string，实际是 %s", describeNode(node))
	}
	out := make(map[string]string, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return nil, fmt.Errorf("metadata.%s 的值必须是字符串，实际是 %s", key, describeNode(value))
		}
		out[key] = value.Value
	}
	return out, nil
}

// describeNode 把 YAML 节点类型翻成一眼能看懂的报错片段。
func describeNode(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "映射（map）"
	case yaml.SequenceNode:
		return "序列（list）"
	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Value == "" {
			return "空值"
		}
		return fmt.Sprintf("标量 %q", node.Value)
	default:
		return "空"
	}
}
