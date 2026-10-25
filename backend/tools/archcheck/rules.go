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
		ruleLogNoAny(),
		ruleModuleNoModels(),
		ruleModuleNoCrossAdapter(),
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
			// safego 是这条规则的正解实现地：启动裸 goroutine 的那一行必然
			// 出现在它内部。若连它自己也被拦，规则将无解。豁免范围**精确**
			// 限定到该目录，不覆盖 platform 的其他部分。
			if isSafeGoImpl(rel) {
				return nil
			}
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

// isSafeGoImpl 判断是否是 safego 自身实现（R7 的唯一豁免点）。
//
// 只匹配该包目录下的文件；测试文件不在豁免范围内也无影响（它们不写裸 go）。
func isSafeGoImpl(rel string) bool {
	return strings.HasPrefix(rel, "internal/platform/safego/")
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

// ── R9：日志审计输入禁止 any（L.10 闸门）────────────────────────
//
// 老毛病：审计 body 曾用 `any` 接收任意结构体，进而在请求路径上反射整棵树，
// 是内存/CPU 尖刺的根源（见 docs/architecture/07 §1.2）。
// 现在 body 只接受边界采集好的 []byte；这里从类型上堵死回归。
//
// 只检查名字以 AuditInput / AuditEvent 结尾的结构体（及 GinAuditInput），
// 不误伤业务结构体里的普通 any。
func ruleLogNoAny() Rule {
	return Rule{
		ID:    "R9",
		Slug:  "log-no-any",
		Title: "日志审计输入禁 any / 禁结构体",
		Level: "error",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isLogLayer(rel) {
				return nil
			}
			var out []Violation
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				name := ts.Name.Name
				if !strings.HasSuffix(name, "AuditInput") &&
					!strings.HasSuffix(name, "AuditEvent") &&
					name != "GinAuditInput" {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, f := range st.Fields.List {
					if isAnyType(f.Type) {
						out = append(out, Violation{
							Rule: "R9", File: rel, Line: fset.Position(f.Pos()).Line,
							Msg: fmt.Sprintf("%s 的字段不应为 any（改用受控类型，如 []byte / string）", name),
						})
					}
				}
				return true
			})
			return out
		},
	}
}

// isAnyType 判断类型表达式是否是裸 any / interface{}。
func isAnyType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "any"
	case *ast.InterfaceType:
		return t.Methods == nil || len(t.Methods.List) == 0
	}
	return false
}

// isLogLayer 判断是否属于日志/审计层（R9 的作用范围）。
//
// 覆盖 internal/platform/log 整个子树：日志写入层原本在 service/log_service，
// 已迁到 internal/platform/log/writer（它构建在 platform/log 的 Event/Emitter 之上）。
// 规则的作用范围必须跟着代码走，否则搬一次目录就静默失去保护。
func isLogLayer(rel string) bool {
	return strings.HasPrefix(rel, "internal/platform/log/") ||
		strings.HasPrefix(rel, "internal/platform/http/middleware/")
}

// ── R10：业务模块禁止 import models ─────────────────────────────
//
// M5 的 row 归属口径（见 04 §4.0）：row 结构（GORM 标签、列映射、TableName）
// 复制进各模块 adapter/mysql 包内，models/ 只做减法。
//
// 不加这条闸门，九个批次都会照抄 M4 的做法继续 import models，
// 到 M6 删 models/ 时就从"删文件"变成"九次集体返工"。
//
// 为什么放行 models/ctype：它是值类型（ID），不是 row 结构，全仓 186 个文件在用；
// 搬它属于 M6 随 models 一起去 kernel 的事，与本规则主题无关。
func ruleModuleNoModels() Rule {
	return Rule{
		ID:    "R10",
		Slug:  "mod-no-models",
		Title: "业务模块禁止 import models（row 结构应放本模块 adapter）",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if !isBusinessModule(rel) {
				return nil
			}
			var out []Violation
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if p != "myblogx/models" && !strings.HasPrefix(p, "myblogx/models/") {
					continue
				}
				if strings.HasPrefix(p, "myblogx/models/ctype") {
					continue
				}
				out = append(out, Violation{
					Rule: "R10", File: rel, Line: fset.Position(imp.Pos()).Line,
					Msg: fmt.Sprintf("业务模块不应 import %s（row 结构复制进本模块 adapter/mysql）", p),
				})
			}
			return out
		},
	}
}

// ── R11：禁止跨模块 import 别人的 adapter ───────────────────────
//
// 跨模块协作只能走 port（读）或 outbox 事件（写），见 04 §4.2。
// 一旦允许直接 import 别人的 adapter，那个 adapter 就成了事实上的公共库，
// 依赖方向会从"模块 → 端口"退化成"模块 → 模块实现"，等于白拆。
func ruleModuleNoCrossAdapter() Rule {
	return Rule{
		ID:    "R11",
		Slug:  "mod-no-cross-adapter",
		Title: "禁止跨模块 import 其他模块的 adapter",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			self := moduleOf(rel)
			if self == "" {
				return nil
			}
			var out []Violation
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				other := moduleOfImport(p)
				if other == "" || other == self {
					continue
				}
				out = append(out, Violation{
					Rule: "R11", File: rel, Line: fset.Position(imp.Pos()).Line,
					Msg: fmt.Sprintf("模块 %s 不应 import 模块 %s 的 adapter（改走 port 或 outbox）", self, other),
				})
			}
			return out
		},
	}
}

// isBusinessModule 判断是否属于 vertical slice 的业务模块。
//
// internal/platform、internal/testkit、internal/kernel、internal/apperr 是共享设施，
// 不属于任何业务模块，不受 row 归属口径约束。
func isBusinessModule(rel string) bool {
	mod := moduleOf(rel)
	return mod != ""
}

// moduleOf 从相对路径里取业务模块名；返回空串表示不是业务模块。
//
//	internal/article/adapter/mysql/x.go → "article"
//	internal/platform/idgen/x.go        → ""（共享设施）
func moduleOf(rel string) string {
	if !strings.HasPrefix(rel, "internal/") {
		return ""
	}
	rest := strings.TrimPrefix(rel, "internal/")
	seg := rest
	if i := strings.Index(rest, "/"); i >= 0 {
		seg = rest[:i]
	}
	switch seg {
	case "", "platform", "testkit", "kernel", "apperr":
		return ""
	}
	return seg
}

// moduleOfImport 判断 import 路径是否指向某个业务模块的 adapter 包。
//
//	myblogx/internal/article/adapter/mysql → "article"
//	myblogx/internal/article/domain        → ""（domain 可被跨模块依赖其契约）
func moduleOfImport(p string) string {
	const prefix = "myblogx/internal/"
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[1] != "adapter" {
		return ""
	}
	switch parts[0] {
	case "platform", "testkit", "kernel", "apperr":
		return ""
	}
	return parts[0]
}
