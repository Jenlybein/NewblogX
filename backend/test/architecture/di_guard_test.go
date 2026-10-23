package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNoAppctxImportInApiServiceRepository(t *testing.T) {
	root := projectRoot(t)
	targets := []string{
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
		filepath.Join(root, "repository"),
	}
	pattern := regexp.MustCompile(`"myblogx/appctx"`)

	violations := collectViolations(t, targets, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到禁止的 appctx 依赖: %v", violations)
	}
}

func TestNoServiceLocatorPatternInApiServiceRepository(t *testing.T) {
	root := projectRoot(t)
	targets := []string{
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
		filepath.Join(root, "repository"),
	}
	pattern := regexp.MustCompile(`MustFromGin|DepsFromGin|mustApp\(`)

	violations := collectViolations(t, targets, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到禁止的 Service Locator 模式: %v", violations)
	}
}

func TestServiceLayerNoGinImport(t *testing.T) {
	root := projectRoot(t)
	target := filepath.Join(root, "service")
	pattern := regexp.MustCompile(`"github.com/gin-gonic/gin"`)

	violations := collectViolations(t, []string{target}, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到 service 层直接依赖 gin: %v", violations)
	}
}

func TestRepositoryNoServiceImport(t *testing.T) {
	root := projectRoot(t)
	target := filepath.Join(root, "repository")
	pattern := regexp.MustCompile(`"myblogx/service/`)

	violations := collectViolations(t, []string{target}, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到 repository 反向依赖 service: %v", violations)
	}
}

func TestApiNoRepositoryImport(t *testing.T) {
	root := projectRoot(t)
	target := filepath.Join(root, "api")
	pattern := regexp.MustCompile(`"myblogx/repository/`)

	violations := collectViolations(t, []string{target}, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到 api 越层依赖 repository: %v", violations)
	}
}

func TestNoRuntimeSingletonCall(t *testing.T) {
	root := projectRoot(t)
	targets := []string{
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
		filepath.Join(root, "repository"),
	}
	pattern := regexp.MustCompile(`site_service\.GetRuntime`)

	violations := collectViolations(t, targets, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到 runtime 配置包级单例调用: %v", violations)
	}
}

func TestNoRuntimeSingletonOrConfigureCallInNonTestCode(t *testing.T) {
	root := projectRoot(t)
	pattern := regexp.MustCompile(`site_service\.GetRuntime|Configure\(`)

	violations := collectViolations(t, []string{root}, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到禁止的隐式配置入口（site_service.GetRuntime/Configure）: %v", violations)
	}
}

func TestApiServiceNoWholeConfigPointerSignature(t *testing.T) {
	root := projectRoot(t)
	targets := []string{
		filepath.Join(root, "api"),
		filepath.Join(root, "service"),
	}
	pattern := regexp.MustCompile(`\*conf\.Config\)`)

	violations := collectViolations(t, targets, func(path string, content string) bool {
		return pattern.MatchString(content)
	})
	if len(violations) > 0 {
		t.Fatalf("检测到 api/service 使用整份配置指针签名: %v", violations)
	}
}

func TestHighFreqQueryServiceNoSQLImplementation(t *testing.T) {
	root := projectRoot(t)
	files := []string{
		filepath.Join(root, "service", "comment_service", "query.go"),
		filepath.Join(root, "service", "follow_service", "query.go"),
		filepath.Join(root, "service", "favorite_service", "query.go"),
		filepath.Join(root, "service", "top_service", "query.go"),
		filepath.Join(root, "service", "chat_service", "query.go"),
		filepath.Join(root, "service", "search_service", "article_search_utils_build.go"),
	}
	pattern := regexp.MustCompile(`\.Where\(|\.Model\(|\.Select\(|\.Find\(|\.Take\(|\.Pluck\(|\.Scan\(`)

	var violations []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("读取文件失败: %v", err)
		}
		if pattern.Match(content) {
			rel, _ := filepath.Rel(root, file)
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("高频 query service 仍包含 SQL 实现: %v", violations)
	}
}

func collectViolations(t *testing.T, roots []string, matcher func(path string, content string) bool) []string {
	t.Helper()

	project := projectRoot(t)
	var violations []string

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(content)
			if matcher(path, text) {
				rel, relErr := filepath.Rel(project, path)
				if relErr != nil {
					rel = path
				}
				violations = append(violations, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描目录失败: %v", err)
		}
	}
	return violations
}
