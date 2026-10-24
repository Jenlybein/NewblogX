// archcheck 是一个基于 Go AST 的架构规则检查器。
//
// 它替代了原来的两套实现（scripts/arch_guard.ps1 与 test/architecture/），
// 那两套检查的 5 个符号在项目中根本不存在，因此永远通过——等于没有守卫。
//
// 设计要点：
//  1. 用 go/ast 分析真实语法树，而不是 grep 字符串，别名/换行/注释都绕不过。
//  2. 支持 --warn（只报告）与 --error（违反即退出非零），便于存量代码渐进收紧。
//  3. 支持 --max-violations 棘轮：存量已知时允许一定数量，但只准减不准增。
//  4. 每条规则都必须能在人造代码上报错——由 rules_test.go 强制验证。
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Violation 表示一处规则违反。
type Violation struct {
	Rule string // 规则编号，如 R1
	File string // 相对项目根的文件路径
	Line int    // 行号
	Msg  string // 人类可读说明
}

// Rule 是一条架构规则。
//
// Check 接收单个文件已经解析好的 AST，返回该文件内的违反列表。
// 约定：Check 不要做跨文件分析（预留扩展点，但当前版本是单文件粒度）。
type Rule struct {
	ID      string
	Title   string
	Level   string // error / warn
	Check   func(fset *token.FileSet, file *ast.File, rel string) []Violation
}

// Config 控制检查行为。
type Config struct {
	Root          string // 项目根（backend/）
	Mode          string // warn 或 error
	MaxViolations int    // 允许的最大违反数，-1 表示不限
	SkipTests     bool   // 是否跳过 _test.go
}

func main() {
	var (
		root          = flag.String("root", ".", "项目根目录")
		mode          = flag.String("mode", "warn", "输出模式：warn 仅报告 / error 违反即失败")
		maxViolations = flag.Int("max-violations", -1, "允许的最大违反数（棘轮），-1 表示不限")
		skipTests     = flag.Bool("skip-tests", true, "跳过 _test.go 文件")
		listRules     = flag.Bool("list-rules", false, "列出所有规则后退出")
	)
	flag.Parse()

	if *listRules {
		for _, r := range rules() {
			fmt.Printf("%s\t[%s]\t%s\n", r.ID, r.Level, r.Title)
		}
		return
	}

	cfg := Config{Root: *root, Mode: *mode, MaxViolations: *maxViolations, SkipTests: *skipTests}

	targets := flag.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	// 兼容 Go 习惯的包模式写法：`./...`、`api/...` 一律视为目录前缀。
	for i, t := range targets {
		targets[i] = normalizeTarget(t)
	}

	violations, err := Run(cfg, targets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "archcheck: %v\n", err)
		os.Exit(2)
	}

	Report(violations)

	// 判断是否失败
	if cfg.Mode == "error" {
		if cfg.MaxViolations >= 0 {
			if len(violations) > cfg.MaxViolations {
				fmt.Fprintf(os.Stderr, "\n✗ 违反数 %d 超过棘轮上限 %d（新增 %d 处）\n",
					len(violations), cfg.MaxViolations, len(violations)-cfg.MaxViolations)
				os.Exit(1)
			}
			fmt.Printf("\n✓ 违反数 %d 未超过棘轮上限 %d\n", len(violations), cfg.MaxViolations)
			return
		}
		if len(violations) > 0 {
			fmt.Fprintf(os.Stderr, "\n✗ 共 %d 处规则违反（--mode=error）\n", len(violations))
			os.Exit(1)
		}
		fmt.Println("\n✓ 全部规则通过")
	}
}

// normalizeTarget 把 Go 的包模式（./...、api/...）转成目录路径。
//
// archcheck 是按文件树遍历的，不需要真正的包解析，
// 因此把 "..."/"..." 尾巴剥掉即可，省掉 go/packages 的加载开销。
func normalizeTarget(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, "/...")
	t = strings.TrimSuffix(t, "...")
	t = strings.TrimSuffix(t, "/")
	if t == "" || t == "." {
		return "."
	}
	return t
}

// Run 遍历目标路径，收集所有违反。
func Run(cfg Config, targets []string) ([]Violation, error) {
	fset := token.NewFileSet()
	var all []Violation

	for _, target := range targets {
		abs := target
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cfg.Root, target)
		}

		err := filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// 跳过无用目录
				base := info.Name()
				if base == ".git" || base == "node_modules" || base == "var" || base == "resources" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if cfg.SkipTests && strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if perr != nil {
				// 解析失败不阻断整体检查，但要让人看见
				fmt.Fprintf(os.Stderr, "warn: 无法解析 %s: %v\n", path, perr)
				return nil
			}

			rel, rerr := filepath.Rel(cfg.Root, path)
			if rerr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)

			for _, rule := range rules() {
				all = append(all, rule.Check(fset, file, rel)...)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all, nil
}

// Report 把违反按规则分组打印。
func Report(violations []Violation) {
	if len(violations) == 0 {
		fmt.Println("archcheck: 未发现规则违反")
		return
	}

	byRule := map[string][]Violation{}
	var order []string
	for _, v := range violations {
		if _, ok := byRule[v.Rule]; !ok {
			order = append(order, v.Rule)
		}
		byRule[v.Rule] = append(byRule[v.Rule], v)
	}

	titles := map[string]string{}
	for _, r := range rules() {
		titles[r.ID] = r.Title
	}

	for _, id := range order {
		vs := byRule[id]
		fmt.Printf("\n── %s · %s（%d 处）\n", id, titles[id], len(vs))
		for _, v := range vs {
			fmt.Printf("  %s:%d  %s\n", v.File, v.Line, v.Msg)
		}
	}

	fmt.Printf("\n合计 %d 处违反\n", len(violations))
}
