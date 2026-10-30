package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// ── R12：ID 语义的字段必须用 ctype.ID，禁止裸整型 ──────────────────
//
// 背景见 docs/adr/0007-id-string-type-as-truth.md。一句话版本：
//
//	雪花 ID 是 18 位十进制，超过 JS 的安全整数上限（16 位）。
//	以前 ctype.ID 底层是 uint64，靠 MarshalJSON 偷偷输出字符串，
//	于是"Go 类型"和"JSON 里的样子"是两个真相——
//	swag 读 Go 类型，契约里印成 integer，与实际传输不符，
//	只能靠生成后的脚本再把 integer 改成 string。
//
// E 系列改动把底层换成 string，让"类型即真相"：swag 印出来就是 string，
// 流水线从 4 步缩到 2 步。这条规则兜住回退——只要有人再写一个
// `HomeStyleID uint64`，新字段就会重新在契约里印成 integer，
// 前端拿到的是精度已经丢掉的数字，而且没有任何编译错误提示他。
//
// 豁免必须写明理由，不能是"懒得改"：
//
//  1. internal/platform/log/**、internal/testkit/schema/**
//     这些结构对应 ClickHouse 的 UInt64 列，外部真相在 DDL 里。
//     Go 侧改成 string 反而与列类型不一致，还可能让 JSONEachRow 摄入失败
//     ——那是"日志丢数据"，比契约印错严重得多。
//  2. ServerID（canalbase / conf_image_ref_river）
//     MySQL 复制协议的 server_id，uint32，不是雪花 ID。
//  3. initializedID（platform/idgen）
//     雪花 worker 编号，不是业务 ID。
//  4. TemplateID（conf/conf_mail.go）
//     腾讯云 SES 的邮件模板编号，控制台分配的小整数，类型由 SDK 决定。
func ruleIDMustBeCtype() Rule {
	return Rule{
		ID:    "R12",
		Slug:  "id-must-be-ctype",
		Title: "ID 语义的字段必须用 ctype.ID，禁止裸整型（会丢 JS 精度）",
		Level: "warn",
		Check: func(fset *token.FileSet, file *ast.File, rel string) []Violation {
			if isIDExemptPath(rel) {
				return nil
			}
			var out []Violation
			ast.Inspect(file, func(n ast.Node) bool {
				st, ok := n.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, field := range st.Fields.List {
					if !isIDFieldName(field) {
						continue
					}
					typeName, bare := bareIntTypeName(field.Type)
					if !bare {
						continue
					}
					if isExemptIDField(rel, field) {
						continue
					}
					out = append(out, Violation{
						Rule: "R12", File: rel, Line: fset.Position(field.Pos()).Line,
						Msg: fmt.Sprintf("ID 字段类型是 %s，应为 ctype.ID（雪花 ID 超出 JS 安全整数，裸整型会让契约印成 integer）", typeName),
					})
				}
				return true
			})
			return out
		},
	}
}

// isIDExemptPath 判断整个包是否属于 R12 豁免范围。
//
// 这两个包的结构与 ClickHouse 的 UInt64 列一一对应，
// 真相在 01-logs.sql 里，不在 HTTP 契约里。
func isIDExemptPath(rel string) bool {
	return strings.HasPrefix(rel, "internal/platform/log/") ||
		strings.HasPrefix(rel, "internal/testkit/schema/")
}

// isExemptIDField 判断是否是逐个点名豁免的非业务 ID 字段。
func isExemptIDField(rel string, field *ast.Field) bool {
	if len(field.Names) == 0 {
		return false
	}
	name := field.Names[0].Name
	switch name {
	case "ServerID":
		// MySQL 复制协议的 server_id，或雪花生成器的机器号：
		// 都是 uint32 的"节点编号"，不是业务实体 ID，不经过 HTTP 契约。
		switch rel {
		case "conf/conf_river.go",
			"conf/conf_system.go",
			"conf/conf_image_ref_river.go",
			"internal/platform/cdc/canalbase/canalbase.go":
			return true
		}
	case "initializedID":
		// 雪花生成器的 worker 编号（未导出字段）。
		return rel == "internal/platform/idgen/idgen.go"
	case "TemplateID":
		// 腾讯云 SES 的邮件模板编号：由控制台分配的小整数，不是雪花 ID，
		// 也不会参与 ctype.ID 那套"底层为字符串"的精度保护。
		// 类型由 SDK 决定（SendEmail 的入参是 *uint64），改成 ctype.ID
		// 反而要在每处调用点来回转换。
		return rel == "conf/conf_mail.go"
	}
	return false
}

// isIDFieldName 判断字段名是否带 ID 语义。
//
// 用 Go 字段名而不是 json tag：改 tag 是随手就做的事，
// 改字段名会牵动所有引用，因此拿字段名当"这是不是 ID"的判据更稳。
func isIDFieldName(field *ast.Field) bool {
	if len(field.Names) == 0 {
		return false // 嵌入字段（如 BaseRow）不判
	}
	for _, name := range field.Names {
		n := name.Name
		if n == "ID" || n == "Id" || n == "IDs" || n == "Ids" {
			return true
		}
		for _, suffix := range []string{"ID", "IDs", "Id", "Ids"} {
			if strings.HasSuffix(n, suffix) {
				return true
			}
		}
	}
	return false
}

// bareIntTypeName 判断类型表达式是否是原生整型（含指针与切片），
// 返回可读的类型名与判定结果。
func bareIntTypeName(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if isBareIntIdent(t.Name) {
			return t.Name, true
		}
	case *ast.StarExpr:
		if inner, ok := bareIntTypeName(t.X); ok {
			return "*" + inner, true
		}
	case *ast.ArrayType:
		if t.Len != nil {
			return "", false // 定长数组不是 ID 的常见形态，不判
		}
		if inner, ok := bareIntTypeName(t.Elt); ok {
			return "[]" + inner, true
		}
	}
	return "", false
}

// isBareIntIdent 判断标识符是否是原生整型名。
func isBareIntIdent(name string) bool {
	switch name {
	case "uint64", "uint", "uint32", "int64", "int", "int32":
		return true
	default:
		return false
	}
}
