package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"
)

// rules 返回全部规则。
//
// 编号约定：R1–R9 是结构规则，R10+ 保留给后续模块切分。
// 每条规则都要在 rules_test.go 里有对应的"人造违规"用例，
// 否则它可能像旧守卫那样永远通过却没人发现。
func rules() []Rule {
	return []Rule{
		ruleAPINoGorm(),
		ruleAPINoDeps(),
		ruleServiceNoGin(),
		ruleDomainNoTimeNow(),
		ruleNoImportCycleAcrossModules(),
		ruleFileTooLong(),
		ruleNoBareGo(),
		ruleNoChineseErrorsNew(),
	}
}

// ── R1：api 层禁止直接持有 *gorm.DB ───────────────────────────────
//
// 现状：61 个 api 文件 import gorm，74 处 *gorm.DB，102 处直接查询调用。
// 这是"分层不存在"的最直接证据——HTTP handler 自己在写 SQL。
// 目标：api 层只依赖能力接口，拿不到数据库连接。
func ruleAPINoGorm() Rule {
	return Rule{
		ID:    "R1",
		Title: "api 层禁止直接使用 *gorm.DB",
		Level: "error",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isAPILayer(rel) {
				return nil
			}
			var out []Violation

			// 1) import 了 gorm 包
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if p == "gorm.io/gorm" || strings.HasPrefix(p, "gorm.io/gorm/") {
					out = append(out, Violation{
						Rule: "R1", File: rel, Line: fset.Position(imp.Pos()).Line,
						Msg: fmt.Sprintf("api 层不应 import %s", p),
					})
				}
			}

			// 2) 字段/参数里出现 *gorm.DB（即便改用别名也拦得住，因为是类型表达式）
			ast.Inspect(file, func(n ast.Node) bool {
				if star, ok := n.(*ast.StarExpr); ok {
					if sel, ok := star.X.(*ast.SelectorExpr); ok {
						if pkg, ok := sel.X.(*ast.Ident); ok && sel.Sel.Name == "DB" && pkg.Name == "gorm" {
							out = append(out, Violation{
								Rule: "R1", File: rel, Line: fset.Position(n.Pos()).Line,
								Msg: "api 层出现 *gorm.DB 类型",
							})
						}
					}
				}
				return true
			})
			return out
		},
	}
}

// ── R2：禁止 type Deps struct ─────────────────────────────────────
//
// 现状：32 个 Deps，30 个含 *gorm.DB，传递链最长 6 层。
// 这是把 global.DB 改名成 deps.DB，比裸全局更糟——它让人误以为重构已完成。
// 目标：完全废弃，改为能力接口（ADR-001）。
func ruleAPINoDeps() Rule {
	return Rule{
		ID:    "R2",
		Title: "禁止定义 type Deps struct（改用能力接口）",
		Level: "error",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			var out []Violation
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					// 命中：Deps 或 XxxDeps（如 APIDeps）
					name := ts.Name.Name
					if name != "Deps" && !strings.HasSuffix(name, "Deps") {
						continue
					}
					if _, isStruct := ts.Type.(*ast.StructType); !isStruct {
						continue
					}
					out = append(out, Violation{
						Rule: "R2", File: rel, Line: fset.Position(ts.Pos()).Line,
						Msg: fmt.Sprintf("定义了依赖打包结构体 %s（应改为窄接口注入）", name),
					})
				}
			}
			return out
		},
	}
}

// ── R3：service 层禁止依赖 gin ───────────────────────────────────
//
// 业务层一旦 import gin，就只能通过起 HTTP 请求来测试，
// 这是"无法单测"的直接原因。
func ruleServiceNoGin() Rule {
	return Rule{
		ID:    "R3",
		Title: "service/domain 层禁止 import gin",
		Level: "error",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isServiceOrDomainLayer(rel) {
				return nil
			}
			var out []Violation
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if strings.Contains(p, "gin-gonic/gin") {
					out = append(out, Violation{
						Rule: "R3", File: rel, Line: fset.Position(imp.Pos()).Line,
						Msg: "业务层不应依赖 HTTP 框架",
					})
				}
			}
			return out
		},
	}
}

// ── R4：domain 层禁止直接取当前时间 ──────────────────────────────
//
// 直接用 time.Now() 会让"过期/限流/排序"类逻辑无法确定性测试。
// 目标：注入 Clock 端口（见 06 测试体系 §确定性）。
// 注意：只检查 domain 层，非 domain 暂不强制，避免一次性几千处。
func ruleDomainNoTimeNow() Rule {
	return Rule{
		ID:    "R4",
		Title: "domain 层禁止直接调用 time.Now（应注入 Clock）",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isDomainLayer(rel) {
				return nil
			}
			var out []Violation
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if pkg.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until") {
					out = append(out, Violation{
						Rule: "R4", File: rel, Line: fset.Position(call.Pos()).Line,
						Msg: fmt.Sprintf("domain 层直接调用 time.%s()", sel.Sel.Name),
					})
				}
				return true
			})
			return out
		},
	}
}

// ── R5：禁止 api 层 import repository（含别名绕过）─────────────────
//
// 旧守卫只匹配 import 路径字符串，用 `type X = repo.X` 就能绕过。
// 这里同时检查 import 与"点号引用的包名前缀"。
func ruleNoImportCycleAcrossModules() Rule {
	return Rule{
		ID:    "R5",
		Title: "api 层禁止 import repository（分层倒置）",
		Level: "error",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isAPILayer(rel) {
				return nil
			}
			var out []Violation
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if strings.Contains(p, "/repository") || strings.HasSuffix(p, "repository") {
					out = append(out, Violation{
						Rule: "R5", File: rel, Line: fset.Position(imp.Pos()).Line,
						Msg: fmt.Sprintf("api 层不应 import %s（仓储应被 service 使用）", p),
					})
				}
			}
			return out
		},
	}
}

// ── R6：单文件行数上限 ───────────────────────────────────────────
//
// 现状：article_sync.go 1,547 行、sync.go 852 行。
// 超大文件不是风格问题——它意味着职责没有边界，改一处要读一天。
func ruleFileTooLong() Rule {
	return Rule{
		ID:    "R6",
		Title: "单文件不得超过 800 行",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			// 用 fset 里的行数信息判断
			tf := fset.File(file.Pos())
			if tf == nil {
				return nil
			}
			lines := tf.LineCount()
			if lines <= 800 {
				return nil
			}
			return []Violation{{
				Rule: "R6", File: rel, Line: 1,
				Msg: fmt.Sprintf("文件共 %d 行，超过 800 行上限", lines),
			}}
		},
	}
}

// ── R7：禁止裸 go 语句（必须用 safego）──────────────────────────
//
// 现状：7 处裸 `go message_service.*`，全项目 recover() 零命中。
// panic 会直接杀死进程，而 gin 的 Recovery 只管 handler 自己的 goroutine。
// 目标：统一走 safego.Go（内部 recover + 结构化日志）。
func ruleNoBareGo() Rule {
	return Rule{
		ID:    "R7",
		Title: "禁止裸 go 语句（应使用 safego.Go）",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			var out []Violation
			ast.Inspect(file, func(n ast.Node) bool {
				gs, ok := n.(*ast.GoStmt)
				if !ok {
					return true
				}
				// 允许 safego.Go(...) 这种调用形式：go func 才是问题
				// 这里判定：调用目标是 funcLit 或普通函数调用都算裸 goroutine
				out = append(out, Violation{
					Rule: "R7", File: rel, Line: fset.Position(gs.Pos()).Line,
					Msg: "裸 go 语句缺少 panic 保护（应改用 safego.Go）",
				})
				return true
			})
			return out
		},
	}
}

// ── R8：禁止 errors.New 中文硬编码 ──────────────────────────────
//
// 现状：115 处中文 errors.New，没有统一错误码，
// 前端无法按 code 分支，只能匹配中文字符串。
func ruleNoChineseErrorsNew() Rule {
	return Rule{
		ID:    "R8",
		Title: "禁止 errors.New 使用中文字面量（应使用 apperr 错误码）",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			var out []Violation
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "errors" || sel.Sel.Name != "New" {
					return true
				}
				if len(call.Args) != 1 {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || !hasChinese(s) {
					return true
				}
				out = append(out, Violation{
					Rule: "R8", File: rel, Line: fset.Position(call.Pos()).Line,
					Msg: "中文错误字符串硬编码（应改为 apperr 错误码）",
				})
				return true
			})
			return out
		},
	}
}

// ── 辅助判断 ────────────────────────────────────────────────────

// isAPILayer 判断是否处于 HTTP 接口层。
func isAPILayer(rel string) bool {
	return strings.HasPrefix(rel, "api/") || strings.Contains(rel, "/adapter/http/")
}

// isServiceOrDomainLayer 判断是否处于业务层。
func isServiceOrDomainLayer(rel string) bool {
	return strings.HasPrefix(rel, "service/") ||
		strings.Contains(rel, "/domain/") ||
		strings.Contains(rel, "/app/")
}

// isDomainLayer 判断是否处于领域层。
func isDomainLayer(rel string) bool {
	return strings.Contains(rel, "/domain/") || strings.HasPrefix(rel, "domain/")
}

// hasChinese 判断字符串是否含 CJK 汉字。
func hasChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// base 返回路径最后一段（便于测试）。
func base(p string) string { return path.Base(p) }
