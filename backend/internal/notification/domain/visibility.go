package domain

import (
	"strings"
	"time"
)

// 公告可见集只有这一份规则。
//
// 线上查询（baseVisible）与有状态假仓储都消费 VisibilityPredicate：
// SQL 子句由 visibilityBranches 逐项拼出，内存判定遍历同一张表。
// 三段 OR 与旧 SQL 逐字对应，任何一处比较符或分支改动都必须同时改这里，
// 不再保留"生产零调用"的等价改写副本。
//
// 用户级软删（id NOT IN 已删除状态）属于个人态而不是可见规则，
// 由各自仓储叠加；这里只回答"这条公告此刻对该注册时间可见吗"。

type visibilityBranch struct {
	rule       VisibleRule
	createdCmp string // "" 表示不比较发布时间；">=" / "<" 对应 SQL 比较符
	expireCmp  string // "" 表示不比较过期时间；">=" 对应 SQL 比较符
}

// visibilityBranches 是线上 SQL 三段 OR 的唯一声明。
var visibilityBranches = []visibilityBranch{
	{rule: VisibleAll},
	{rule: VisibleRegistered, createdCmp: ">=", expireCmp: ">="},
	{rule: VisibleNew, createdCmp: "<", expireCmp: ">="},
}

// VisibilityPredicate 是一次"此刻 / 该用户注册时间"的可见性判定。
type VisibilityPredicate struct {
	Now           time.Time
	UserCreatedAt time.Time
}

// SQL 返回线上查询的可见性子句与参数。
//
// 与 baseVisible 旧的 `Where(db.Where(...).Or(...))` 形状等价，但文本只在
// 这里生成一次；调用方只负责叠加过期总闸、用户级软删与分页。
func (p VisibilityPredicate) SQL() (string, []any) {
	var parts []string
	args := make([]any, 0, 7)
	for _, b := range visibilityBranches {
		conds := []string{"user_visible_rule = ?"}
		args = append(args, int8(b.rule))
		if b.createdCmp != "" {
			conds = append(conds, "created_at "+b.createdCmp+" ?")
			args = append(args, p.UserCreatedAt)
		}
		if b.expireCmp != "" {
			conds = append(conds, "expire_time "+b.expireCmp+" ?")
			args = append(args, p.UserCreatedAt)
		}
		parts = append(parts, "("+strings.Join(conds, " AND ")+")")
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// Matches 用同一张分支表在内存里判定，供有状态假仓储消费。
func (p VisibilityPredicate) Matches(n GlobalNotif) bool {
	if !n.ExpireAt.After(p.Now) {
		return false
	}
	for _, b := range visibilityBranches {
		if n.VisibleRule != b.rule {
			continue
		}
		if !compareTime(n.CreatedAt, p.UserCreatedAt, b.createdCmp) {
			continue
		}
		if !compareTime(n.ExpireAt, p.UserCreatedAt, b.expireCmp) {
			continue
		}
		return true
	}
	return false
}

// compareTime 把 SQL 比较符作用到两个时间点；"" 表示这一支不比较。
func compareTime(left, right time.Time, op string) bool {
	switch op {
	case "":
		return true
	case ">=":
		return !left.Before(right)
	case "<":
		return left.Before(right)
	default:
		return false
	}
}
