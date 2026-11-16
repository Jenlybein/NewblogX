package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
)

// 本文件是风险页的**处置动作**（ADR-0032 决策 7 / 票 398、399）。
//
// 处置有四个动作，分两类：
//   - 内容动作（删除命中内容、忽略误报）：删除语义按文本面分派 ——
//     文章 / 评论 / 合集走各自模块的软删用例，昵称 / 简介走用户模块的资料重置
//     （昵称改写为「问题昵称 + uid 末 4 位」、简介清空）；私信面没有可删除的实体，
//     delete 明确报错而不是静默 no-op。
//   - 账号动作（封禁、禁言）：见 AccountDisposer。内容保留 —— 处罚对象是账号，
//     不是这两段文本（文本早已被替换成 `*` 且已发布）。
//
// 关键口径：
//   - 处置痕迹（处理人 / 时间 / 备注）写在命中记录行上，供事后追溯；
//   - 同一记录重复处置是幂等的：已处置的直接返回当前状态，不再动内容/账号。

// Action 是处置动作。
//
// 用字符串而不是 int8：动作枚举既是 HTTP 请求里的外部形状，也是审计日志与
// 前端按钮的键，套一层"数字 ↔ 名字"的对照表只会让契约和实现各自漂移。
type Action string

// 四个处置动作。
const (
	// ActionDelete 删除命中的内容（按文本面分派，见 Disposer）。
	ActionDelete Action = "delete"
	// ActionIgnore 忽略误报：状态转"已忽略"，内容保持原样。
	ActionIgnore Action = "ignore"
	// ActionBan 封禁账号（账号禁用）：登录与发布都被拦，内容保留（票 399）。
	ActionBan Action = "ban"
	// ActionMute 禁言账号（带时长，默认 7 天）：只拦发布类动作（票 399）。
	ActionMute Action = "mute"
)

// ParseAction 把外部字符串解析成处置动作；不认识返回 false。
func ParseAction(raw string) (Action, bool) {
	switch Action(raw) {
	case ActionDelete:
		return ActionDelete, true
	case ActionIgnore:
		return ActionIgnore, true
	case ActionBan:
		return ActionBan, true
	case ActionMute:
		return ActionMute, true
	}
	return "", false
}

// Valid 判断动作是否是登记过的四个之一。
func (a Action) Valid() bool {
	_, ok := ParseAction(string(a))
	return ok
}

// IsAccountAction 判断动作是否作用于账号（封禁 / 禁言）而不是内容。
//
// 两类动作的落点不同：内容动作走 Disposer（软删内容 / 重置资料），
// 账号动作走 AccountDisposer（改账号状态 / 写 mute_until）。
func (a Action) IsAccountAction() bool {
	return a == ActionBan || a == ActionMute
}

// StatusFor 返回该动作把记录带到哪个状态。
//
// 封禁与禁言把记录标成「账号已处置」：内容没被删，但这条风险已经有结论，
// 不该继续留在待处理队列里。
func (a Action) StatusFor() Status {
	switch a {
	case ActionDelete:
		return StatusDeleted
	case ActionBan, ActionMute:
		return StatusAccount
	default:
		return StatusIgnored
	}
}

// MaxNoteLen 是处置备注的最大长度（按 rune 计，与昵称等文本字段同一口径）。
const MaxNoteLen = 200

// NormalizeNote 校验并收敛备注：去首尾空白后按 rune 计数不超过 MaxNoteLen。
//
// 备注是可选的审计补充，不是自由文本输入框：超过上限的备注只会让审计表里
// 出现一条无法阅读的长文，所以在这里直接拒绝，而不是截断后静默保存
// （截断会让管理员以为写进去了）。
func NormalizeNote(raw string) (string, error) {
	note := strings.TrimSpace(raw)
	if utf8.RuneCountInString(note) > MaxNoteLen {
		return "", apperr.New(apperr.CodeFilterNoteTooLong)
	}
	return note, nil
}

// ProblemNicknamePrefix 是资料重置后的昵称前缀（spec 决策 7 的口径）。
const ProblemNicknamePrefix = "问题昵称"

// ProblemNickname 返回资料重置后的昵称：「问题昵称」+ uid 末 4 位。
//
// 末 4 位补零到四位（uid 42 → 问题昵称0042）：不补零会让 "0042" 与 "42"
// 这类边界在人工核对 uid 时对不上，而固定四位也保证同一条规则对所有人一致。
func ProblemNickname(userID ctype.ID) string {
	last4 := userID.Uint64() % 10000
	return fmt.Sprintf("%s%04d", ProblemNicknamePrefix, last4)
}

// Handle 是一次处置的审计痕迹。
//
// HandledAt 与 Note 都是"处置那一刻"的快照：之后改备注不会重写历史（也还没有
// 改备注的入口 —— 重复处置直接返回当前状态）。
type Handle struct {
	Status    Status
	HandlerID ctype.ID
	HandledAt time.Time
	Note      string
}

// Disposer 是处置动作对**内容模块**的写端口，实现留在装配层
// （app/filter_wiring.go 的 bridge）。
//
// 分成四个方法而不是一个带 surface 参数的 Delete：面 → 实体 的映射属于内容过滤
// 模块，内容过滤不该把"我有六个面"泄露给别的模块；分成四个方法也让"把昵称面
// 接到文章删除"变成编译错误，而不是一条删错东西的处置。
//
// actor 是发起处置的管理员 ID：它会被内容模块的软删用例当成操作者
// （文章删除事件、评论删除权限判定都要它）。它不影响 hit 行上的处理人 ——
// 那是 Handle 自己的字段。
type Disposer interface {
	// DeleteArticle 软删一篇文章（连同评论 / 点赞 / 收藏等关联数据）。
	DeleteArticle(ctx context.Context, actor, articleID ctype.ID) error
	// DeleteComment 软删一条评论（根评论连同其回复）。
	DeleteComment(ctx context.Context, actor, commentID ctype.ID) error
	// DeleteCollection 软删一个合集（不删文章本体）。
	DeleteCollection(ctx context.Context, actor, collectionID ctype.ID) error
	// ResetProfile 重置被处置用户的资料：昵称改写为 nickname、简介清空。
	ResetProfile(ctx context.Context, actor, userID ctype.ID, nickname string) error
}

// AccountDisposer 是处置动作对**账号模块**的写端口（票 399），实现留在装配层
// （app/filter_wiring.go 的 accountDisposerBridge）。
//
// 与内容 Disposer 分开：内容删除与账号处罚是两件事，混在一个接口里会让
// "只想测删除"的用例被迫实现封禁与禁言。分成两个接口也让"把封禁接到删除桥"
// 变成编译错误。
//
// 时长校验（1–365、缺省 7 天）不在这里重复实现：MuteUser 的实现方（user 模块）
// 负责校验并返回 user.mute_days_invalid，本模块只负责把天数原样传过去。
type AccountDisposer interface {
	// BanUser 封禁账号（账号禁用：登录与发布都被拦）。
	BanUser(ctx context.Context, actor, userID ctype.ID) error
	// MuteUser 禁言账号 days 天；days 为 0 时由实现方按默认值（7 天）处理。
	MuteUser(ctx context.Context, actor, userID ctype.ID, days int) error
}
