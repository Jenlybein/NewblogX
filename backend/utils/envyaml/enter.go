package envyaml

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var placeholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// Expand 会把文本中的 ${VAR} / ${VAR:-default} 占位符替换成环境变量值。
// 规则尽量贴近 shell 中的 :- 语义：
// 1. 变量存在且非空：使用环境变量值
// 2. 变量不存在或为空，且提供了 default：使用 default
// 3. 变量不存在，且没有 default：返回错误，避免把必填配置静默展开成空值
func Expand(content string) (string, error) {
	expanded := content
	for i := 0; i < 10; i++ {
		var missingKeys []string
		next := placeholderPattern.ReplaceAllStringFunc(expanded, func(match string) string {
			sub := placeholderPattern.FindStringSubmatch(match)
			if len(sub) == 0 {
				return match
			}
			key := sub[1]
			defaultValue := sub[3]

			value, ok := os.LookupEnv(key)
			if ok && value != "" {
				return value
			}
			if defaultValue != "" || strings.Contains(match, ":-") {
				return defaultValue
			}
			if ok {
				return value
			}
			missingKeys = append(missingKeys, key)
			return match
		})
		if len(missingKeys) > 0 {
			return "", fmt.Errorf("缺少必需的环境变量: %s", strings.Join(missingKeys, ", "))
		}
		if next == expanded {
			return next, nil
		}
		expanded = next
	}
	return expanded, nil
}

// Unmarshal 会先展开环境变量，再做 YAML 反序列化。
func Unmarshal(data []byte, out any) error {
	expanded, err := Expand(string(data))
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(expanded), out)
}
