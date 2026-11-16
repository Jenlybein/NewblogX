// Package moderation 是「三层机审」的第②层：从**已有评分记录**派生的风险标签
// （低分 / 维度塌陷 / 篇幅异常），不重新调用模型。
//
// 第①层（纯代码高危词匹配）已整体退场（ADR-0032 决策 6 / 票 401）：命中词在落库前
// 就被全站内容过滤替换成 `*`，词表归一到了运行期配置的 content_filter 段、只服务
// 替换与关键词风险页。所以本包不再持有任何词表数据、也不做字符串匹配 ——
// "哪些词"是运营数据，"怎么判"才是本包的规则。质量闸门、第③层模型合规判定、
// 发布分流与 pending_review 可见性都不受这次退场影响。
//
// 本层不落库，所以评分记录一更新，下一次读取立即按新评分判。
//
// 与第三层（模型合规判定）的分工：第三层花钱、有延迟、落风险记录表；
// 本包不 import 任何 AI 包，也不会发起模型调用。
package moderation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 第②层风险标签的规则阈值（规则结构在代码里，没有词表数据）。
//
// 这些数字只在这一处声明：审核列表的标签与提交时的同步风险扫描都从同一份规则派生，
// 不在 handler / 前端再抄一份。
const (
	// LowScoreThreshold 是"低分"的及格线：总分低于它标 low_score。
	//
	// 与评分记录既有的分档一致（"较差文章"0–30、"不合格文章"31–59），
	// 因此 60 分起不再标低分。质量闸门的及格线是另一个可配值，
	// 它不是这个告警阈值的来源：这里的标签只用于排序与标红，不构成放行结论。
	LowScoreThreshold = 60
	// CollapsedDimensionScore 是维度塌陷阈值：任一维度低于它即点名。
	CollapsedDimensionScore = 30
	// MinContentLength 是"篇幅异常"的下界（评分记录里的 rune 数）。
	MinContentLength = 200
	// MaxContentLength 是"篇幅异常"的上界。
	MaxContentLength = 20000
)

// Level 是风险等级。取值会作为契约字段下发给后台审核列表。
//
// 排序与"取最严重者"都依赖 Rank：告警文案的措辞可以改，等级之间的相对
// 顺序不能改。
type Level string

const (
	// LevelNone 没有风险信号。
	LevelNone Level = "none"
	// LevelLow 低风险：只提示，不影响人工判断的主次。
	LevelLow Level = "low"
	// LevelMedium 中风险：值得优先看一眼，例如分数不合格。
	LevelMedium Level = "medium"
	// LevelHigh 高风险：按严重性排在队列最前。
	//
	// 评分派生层不产生 high（低分是 medium，维度塌陷与篇幅异常是 low），
	// 这个取值随契约保留，供人工处置与将来更严的规则使用。
	LevelHigh Level = "high"
)

// Rank 返回等级的可比较序：none < low < medium < high。
//
// 未知值按 none 处理：契约只有四个值，越界值（例如老前端回传的拼写）不应该
// 把一条干净文章排到队列最前面。
func (l Level) Rank() int {
	switch l {
	case LevelHigh:
		return 3
	case LevelMedium:
		return 2
	case LevelLow:
		return 1
	default:
		return 0
	}
}

// SignalCode 是风险标签的稳定标识（前端据此映射中文文案）。
//
// 中文文案放在前端，code 留在契约里：运营改措辞不需要重新生成 OpenAPI，
// 也已经发生的排序与筛选不会因为文案变化而失效。
type SignalCode string

const (
	// CodeLowScore 评分总分低于及格线。
	CodeLowScore SignalCode = "low_score"
	// CodeDimensionCollapse 有一个或多个评分维度塌陷。
	CodeDimensionCollapse SignalCode = "dimension_collapse"
	// CodeLengthAbnormal 篇幅异常（过短或过长）。
	CodeLengthAbnormal SignalCode = "length_abnormal"
)

// Signal 是一条风险信号。
type Signal struct {
	// Code 是稳定标识。
	Code SignalCode
	// Level 是这条信号建议的风险等级。
	Level Level
	// Detail 是给人看的命中细节（维度名 / 分数与篇幅），
	// 不进对外契约；审核列表只下发 code。
	Detail string
}

// ── 第②层：从已有评分记录派生风险标签 ──
//
// 这一层只读评分记录，零新增模型调用（票 311 验收标准 3）。没有评分记录时
// 调用方传 nil，本层不产生任何信号，也不报错（验收标准 5）。

// ScoreFacts 是派生风险标签需要的评分记录片段。
//
// 只搬需要的字段，不把整个 ai/domain.ScoreRecord 搬进来：article 模块不认识
// AI 模块的实体，跨模块只经过使用方定义的窄端口。
type ScoreFacts struct {
	// TotalScore 是评分记录里的最终总分（0–100）。
	TotalScore int
	// DimensionsJSON 是评分记录里的六维分数（JSON 数组，允许为空或坏数据）。
	DimensionsJSON string
	// ContentLength 是评分时的正文长度（rune 数）；0 表示该记录没有可用篇幅。
	ContentLength int
}

// scoreDimension 是 dimensions_json 里的一个维度，只关心名字与分数。
type scoreDimension struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

// ScoreSignals 从评分记录派生风险标签。
//
// 派生规则（全部是纯函数，可反复重算）：
//   - 低分：TotalScore < LowScoreThreshold → low_score（medium）；
//   - 维度塌陷：任一维度 < CollapsedDimensionScore → dimension_collapse（low），
//     Detail 按 JSON 里的顺序点名；
//   - 篇幅异常：ContentLength 非 0 且落在 [MinContentLength, MaxContentLength] 之外
//     → length_abnormal（low）。
//
// dimensions_json 是外部系统的输出，坏数据（不是数组、字段缺失）按"没有维度信息"
// 处理，不让审核列表因为一条脏记录整页失败。
func ScoreSignals(score *ScoreFacts) []Signal {
	if score == nil {
		return nil
	}

	signals := make([]Signal, 0, 3)

	if score.TotalScore < LowScoreThreshold {
		signals = append(signals, Signal{
			Code:  CodeLowScore,
			Level: LevelMedium,
			Detail: fmt.Sprintf("总分 %d 低于及格线 %d",
				score.TotalScore, LowScoreThreshold),
		})
	}

	if collapsed := collapsedDimensions(score.DimensionsJSON); len(collapsed) > 0 {
		signals = append(signals, Signal{
			Code:   CodeDimensionCollapse,
			Level:  LevelLow,
			Detail: strings.Join(collapsed, ","),
		})
	}

	if score.ContentLength > 0 &&
		(score.ContentLength < MinContentLength || score.ContentLength > MaxContentLength) {
		signals = append(signals, Signal{
			Code:  CodeLengthAbnormal,
			Level: LevelLow,
			Detail: fmt.Sprintf("篇幅 %d 字，超出 %d–%d 字",
				score.ContentLength, MinContentLength, MaxContentLength),
		})
	}

	return signals
}

// collapsedDimensions 解析维度 JSON，返回低于塌陷阈值的维度名。
func collapsedDimensions(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}

	var dimensions []scoreDimension
	if err := json.Unmarshal([]byte(raw), &dimensions); err != nil {
		return nil
	}

	var collapsed []string
	for _, d := range dimensions {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		if d.Score < CollapsedDimensionScore {
			collapsed = append(collapsed, name)
		}
	}
	return collapsed
}

// Result 是一次机审判定的结论。
type Result struct {
	// Level 是最严重的信号等级；没有任何信号时为 LevelNone。
	Level Level
	// Signals 按评分层的顺序给出（低分 → 维度塌陷 → 篇幅异常），便于展示。
	Signals []Signal
}

// Labels 返回去重后的风险标签 code（按首次出现顺序）。
//
// 契约下发的是这一份：同一 code 只给一个 label，标签数量不随信号条数膨胀。
func (r Result) Labels() []string {
	if len(r.Signals) == 0 {
		return nil
	}
	seen := make(map[SignalCode]bool, len(r.Signals))
	labels := make([]string, 0, len(r.Signals))
	for _, s := range r.Signals {
		if seen[s.Code] {
			continue
		}
		seen[s.Code] = true
		labels = append(labels, string(s.Code))
	}
	return labels
}

// Evaluate 汇总本层的结论：只有从已有评分派生的标签。
//
// score 为 nil 表示这篇文章还没有评分记录（或评分端口不可用），此时不产生任何
// 信号、不报错（票 311 验收标准 5）。
func Evaluate(score *ScoreFacts) Result {
	signals := ScoreSignals(score)

	level := LevelNone
	for _, s := range signals {
		if s.Level.Rank() > level.Rank() {
			level = s.Level
		}
	}
	return Result{Level: level, Signals: signals}
}
