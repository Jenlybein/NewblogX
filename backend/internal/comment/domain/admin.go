package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 本文件是管理端「评论管理」的领域约定（第一期：列表 / 批量删除 / 禁言 / 封禁）。
//
// 分两层职责，刻意不混：
//   - 这里只回答"哪些筛选与处置参数算合法"（状态枚举、分页上下限、理由长度、
//     禁言档位），不解析 HTTP（那是 adapter/http）、不拼 SQL（那是 adapter/mysql）；
//   - 管理员身份判定留在用例层（app.Service 的 viewer.Admin），因为"谁能调"
//     属于用例语义，但它用的常量与 Viewer 已经在本包，不需要额外的领域类型。

// AdminStatus 是管理端列表的状态筛选。
//
// 三种取值都必须显式存在：后台下拉框要能渲染「全部 / 已发布 / 已删除」三项，
// 契约（OpenAPI 的 enums）也要能点出三种值。不允许用"不传这个参数"隐式表示全部
// —— 那样前端一旦漏传，筛选条件就静默失效，运营看到的"全部"其实是个默认值。
type AdminStatus string

const (
	// AdminStatusAll 不按状态筛（含已发布与已删除）。
	AdminStatusAll AdminStatus = "all"
	// AdminStatusPublished 只看已发布。
	AdminStatusPublished AdminStatus = "published"
	// AdminStatusDeleted 只看已删除。
	AdminStatusDeleted AdminStatus = "deleted"
)

// ParseAdminStatus 把 query 里的原始取值转成状态筛选。
//
// 空串按「全部」处理：老书签 / 手写请求不带这个参数时，语义应当是"不过滤"
// 而不是报错。除此之外的未知取值一律拒绝 —— 静默当成 all 会让运营以为筛选
// 生效了，实际看到的是全量数据（与"前端拼错参数名"是同一类问题）。
func ParseAdminStatus(raw string) (AdminStatus, error) {
	switch AdminStatus(strings.TrimSpace(raw)) {
	case "", AdminStatusAll:
		return AdminStatusAll, nil
	case AdminStatusPublished:
		return AdminStatusPublished, nil
	case AdminStatusDeleted:
		return AdminStatusDeleted, nil
	default:
		return "", apperr.New(apperr.CodeCommentAdminStatusInvalid)
	}
}

// IsValid 判断状态在枚举范围内（空串不算，调用方应先走 Normalize）。
func (s AdminStatus) IsValid() bool {
	switch s {
	case AdminStatusAll, AdminStatusPublished, AdminStatusDeleted:
		return true
	default:
		return false
	}
}

// admin 分页与批量上限。
const (
	// AdminDefaultLimit 是管理端列表的默认每页条数。
	//
	// 比读者侧的 10 条大一倍：后台是"扫数据"的场景（找某条违规评论、核对某个
	// 用户最近说了什么），一屏多几条能少翻几次页；再大就会让单页响应变重。
	AdminDefaultLimit = 20
	// AdminMaxLimit 是管理端列表允许的最大每页条数。
	AdminMaxLimit = 100
	// MaxAdminBatchIDs 是一次批量删除允许的最大条数。
	//
	// 上限不是防"删太多"，而是防一次请求把整个帖子家族扫过一遍：批量删除逐条
	// 走完整删除链路（含级联、计数、缓存），条数越多，一次请求占用的连接与
	// 内存越久，中途失败也越难向运营解释。超出就让前端分批提交。
	MaxAdminBatchIDs = 100
	// MaxAdminReasonLen 是处置理由的最大长度（按 rune 计，中文一个字算一个）。
	MaxAdminReasonLen = 200
)

// AdminPage 是管理端列表的分页参数。
//
// 单独一个类型而不是复用 Page：读者侧默认 10 条、上限口径是把越界值打回默认值，
// 管理端是默认 20 条、越界值夹到上限。两个口径混用会让"后台传 limit=200
// 却只拿到 10 条"这种问题永远说不清是谁改的。
type AdminPage struct {
	Page  int
	Limit int
}

// Normalize 收敛分页参数：默认 20 条，超过上限夹到 100，页码至少 1。
func (p AdminPage) Normalize() AdminPage {
	switch {
	case p.Limit <= 0:
		p.Limit = AdminDefaultLimit
	case p.Limit > AdminMaxLimit:
		p.Limit = AdminMaxLimit
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return p
}

// Offset 返回 SQL 偏移量。
func (p AdminPage) Offset() int {
	n := p.Normalize()
	return (n.Page - 1) * n.Limit
}

// AdminFilter 是管理端评论列表的筛选条件。
//
// 所有字段都是"零值 = 不按该项筛"：后台的筛选栏可以任意组合，缺省不能变成
// "筛 ID 为 0 的评论"这种空结果。
type AdminFilter struct {
	// Keyword 是评论内容子串。
	Keyword string
	// UserID 是评论发布人。
	UserID ctype.ID
	// UserKeyword 是发布人昵称子串（读的是评论行上的昵称快照列）。
	UserKeyword string
	// ArticleID 是评论所属文章。
	ArticleID ctype.ID
	// FromAt / ToAt 是 created_at 的时间范围：FromAt 含、ToAt 不含。
	//
	// 右边界用"次日零点 + 不含"而不是"当日 23:59:59 + 含"：后者在毫秒精度下会
	// 漏掉当天最后一毫秒（datetime(3) 存了毫秒），是"选到今天就少几条"的经典来源。
	FromAt *time.Time
	ToAt   *time.Time
	// Status 是状态筛选；零值由 Normalize 补成 all。
	Status AdminStatus
}

// Normalize 收敛筛选条件：去首尾空白、把空状态补成「全部」。
//
// 空白关键词必须归零：LIKE '%%' 会把"没填关键词"变成全表扫描，而它看起来
// 又是一次正常查询。
func (f AdminFilter) Normalize() AdminFilter {
	f.Keyword = strings.TrimSpace(f.Keyword)
	f.UserKeyword = strings.TrimSpace(f.UserKeyword)
	if f.Status == "" {
		f.Status = AdminStatusAll
	}
	return f
}

// NormalizeAdminMuteDays 校验管理端禁言档位。
//
// 只有四档：1 / 7 / 30 天与永久（0）。不接受任意天数是因为这是后台下拉框的
// 固定选项：允许任意天数会让"误输入 3650"变成一个没人预期的永久禁言 ——
// 永久必须是一个显式选择（0），而不是长整数的副作用。
func NormalizeAdminMuteDays(raw int) (int, error) {
	switch raw {
	case 0, 1, 7, 30:
		return raw, nil
	default:
		return 0, apperr.New(apperr.CodeCommentAdminDaysInvalid)
	}
}

// NormalizeAdminReason 校验禁言理由：必填、去首尾空白、≤200 字。
//
// 必填是因为这句话要作为"为什么禁你"的凭据展示在审核记录里（也是管理员
// 事后复盘时唯一能看懂的线索）；空白串不算填。
func NormalizeAdminReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	if reason == "" {
		return "", apperr.New(apperr.CodeCommentAdminReasonRequired)
	}
	if utf8.RuneCountInString(reason) > MaxAdminReasonLen {
		return "", apperr.New(apperr.CodeCommentAdminReasonTooLong)
	}
	return reason, nil
}

// NormalizeAdminOptionalReason 校验可选理由：可以留空，填了就不能超长。
//
// 封禁没有"必填理由"这条要求（封禁本身就是终态、没有到期日需要向用户解释），
// 但超长同样要拒绝：审计日志与后续通知都按 200 字的分辨率在存储与展示上取舍。
func NormalizeAdminOptionalReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	if utf8.RuneCountInString(reason) > MaxAdminReasonLen {
		return "", apperr.New(apperr.CodeCommentAdminReasonTooLong)
	}
	return reason, nil
}
