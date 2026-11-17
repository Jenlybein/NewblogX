// Package domain 是"违规计分与阶梯处罚"的规则层（ADR-0035）。
//
// 这一层只有纯函数：事件怎么变成有效分、有效分怎么变成处罚。没有数据库、
// 没有时间源（now 一律从外面传进来），所以每条阈值都能被表驱动单测钉死。
//
// 为什么分数不存成一列再天天加减：分数是**派生值**。存一列就得有定时任务
// 每天减分、封禁期还得决定减不减、回滚一次就得补偿。而"近 90 天、按 60 天
// 半衰期衰减"只是一个函数，随时可重算，撤销一条事件后重新算一遍就是正确结果。
package domain

import (
	"math"
	"time"
)

// Level 是违规等级。等级决定分值，也决定"要不要人工确认"。
type Level string

const (
	// LevelKeyword 关键词命中（一次请求命中多个词只算一次）。
	LevelKeyword Level = "keyword"
	// LevelImage 图片违规（机审判定即计全分）。
	LevelImage Level = "image"
	// LevelGeneral 人工认定的一般违规。
	LevelGeneral Level = "general"
	// LevelSevere 恶劣违规（涉未成年人、组织性违法等），由人工认定。
	LevelSevere Level = "severe"
)

// Score 返回该等级的分值。
//
// 关键词 5 / 图片 20 / 一般 10 / 恶劣 50 是产品决策，不是可调参数：
// 分值散进配置会让"为什么我被封了"变得无法解释。
func (l Level) Score() int {
	switch l {
	case LevelKeyword:
		return 5
	case LevelImage:
		return 20
	case LevelGeneral:
		return 10
	case LevelSevere:
		return 50
	default:
		return 0
	}
}

// Source 是事件来源。人工撤销与申诉都靠它区分"机器判定"与"人已经看过"。
type Source string

const (
	// SourceMachine 机器判定（关键词引擎、图片审核）。
	SourceMachine Source = "machine"
	// SourceManual 人工判定。
	SourceManual Source = "manual"
)

// Status 是事件状态。账本只追加：撤销是"把状态改成 revoked"，不是删行。
type Status string

const (
	// StatusActive 生效中。
	StatusActive Status = "active"
	// StatusRevoked 已撤销（申诉通过或管理员纠错）。
	StatusRevoked Status = "revoked"
)

// Event 是一条违规事件。
type Event struct {
	ID        uint64
	UserID    uint64
	Level     Level
	Score     int
	Source    Source
	RefType   string
	RefID     uint64
	Evidence  string
	Status    Status
	IP        string
	Device    string
	CreatedAt time.Time
}

// Window 是有效分的统计窗口。
//
// 90 天窗口 + 60 天半衰期：一次误伤在两个月后只算半分、三个月后彻底出局，
// 常用户不会因为"陈年旧账"被封；持续违规的人分数一直往上走。
const (
	Window     = 90 * 24 * time.Hour
	HalfLife   = 60 * 24 * time.Hour
	halfDivisor = float64(1)
)

// EffectiveScore 计算一个用户当前的有效分。
//
// 只统计：
//   - status 为 active 的事件（撤销的不算）；
//   - 落在 Window 内的事件（更早的不算）。
//
// 每条事件按 HalfLife 指数衰减：60 天前的一条 20 分事件现在只值 10 分。
func EffectiveScore(events []Event, now time.Time) float64 {
	total := 0.0
	for _, event := range events {
		if event.Status != StatusActive {
			continue
		}
		age := now.Sub(event.CreatedAt)
		if age < 0 {
			// 时钟回拨：当作刚发生，不放大也不丢弃。
			age = 0
		}
		if age > Window {
			continue
		}
		score := float64(event.Score)
		if score == 0 {
			score = float64(event.Level.Score())
		}
		total += score * math.Pow(0.5, age.Hours()/HalfLife.Hours()/halfDivisor)
	}
	return total
}

// Punishment 是一档处罚。
type Punishment string

const (
	// PunishmentNone 不处罚（只记录与通知）。
	PunishmentNone Punishment = "none"
	// PunishmentMute1Day 禁言一天。
	PunishmentMute1Day Punishment = "mute_1d"
	// PunishmentMute7Days 禁言七天。
	PunishmentMute7Days Punishment = "mute_7d"
	// PunishmentMute30Days 禁言三十天。
	PunishmentMute30Days Punishment = "mute_30d"
	// PunishmentManualReview 人工评估（可永久封禁）。
	PunishmentManualReview Punishment = "manual_review"
)

// 阈值是产品决策：100 一天、120 七天、150 三十天、200 人工。
const (
	ThresholdMute1Day  = 100.0
	ThresholdMute7Days = 120.0
	ThresholdMute30Days = 150.0
	ThresholdManualReview = 200.0
)

// Decide 按有效分决定该给的处罚档位。
//
// 注意最后两档的差别：150 仍是自动禁言，200 只**触发人工评估**。
// 永久封禁不可逆，不能由一行阈值代码决定。
func Decide(score float64) Punishment {
	switch {
	case score >= ThresholdManualReview:
		return PunishmentManualReview
	case score >= ThresholdMute30Days:
		return PunishmentMute30Days
	case score >= ThresholdMute7Days:
		return PunishmentMute7Days
	case score >= ThresholdMute1Day:
		return PunishmentMute1Day
	default:
		return PunishmentNone
	}
}

// MuteDays 返回该档位的禁言天数（非禁言档返回 0）。
func (p Punishment) MuteDays() int {
	switch p {
	case PunishmentMute1Day:
		return 1
	case PunishmentMute7Days:
		return 7
	case PunishmentMute30Days:
		return 30
	default:
		return 0
	}
}

// severityRank 给档位排序，用于"封禁期内再违规升一档"。
func (p Punishment) severityRank() int {
	switch p {
	case PunishmentNone:
		return 0
	case PunishmentMute1Day:
		return 1
	case PunishmentMute7Days:
		return 2
	case PunishmentMute30Days:
		return 3
	case PunishmentManualReview:
		return 4
	default:
		return 0
	}
}

// Next 返回升一档后的处罚。
//
// 用途：封禁期内再次违规。不重新按分数算 —— 否则一个刚被封一天的账号
// 在封禁期里再犯一次，分数可能还是 100 出头，等于原地不动。
func (p Punishment) Next() Punishment {
	switch p {
	case PunishmentNone:
		return PunishmentMute1Day
	case PunishmentMute1Day:
		return PunishmentMute7Days
	case PunishmentMute7Days:
		return PunishmentMute30Days
	default:
		// 30 天与"人工评估"之间不再自动升档：再往上就是永久封禁，
		// 必须由人看。
		return PunishmentManualReview
	}
}

// Escalate 计算"考虑封禁期加成"之后的最终处罚。
//
// currentlyMuted 为真表示这个账号现在正处在禁言期里。
func Escalate(current Punishment, currentlyMuted bool) Punishment {
	if !currentlyMuted {
		return current
	}
	if current == PunishmentNone {
		// 封禁期内的轻微违规也至少推一档：这是"再犯"的信号。
		return PunishmentNone.Next()
	}
	return current.Next()
}

// LadderLabel 返回给用户看的中文档位说明（通知文案用）。
func (p Punishment) LadderLabel() string {
	switch p {
	case PunishmentMute1Day:
		return "禁言 1 天"
	case PunishmentMute7Days:
		return "禁言 7 天"
	case PunishmentMute30Days:
		return "禁言 30 天"
	case PunishmentManualReview:
		return "转人工评估（可能永久封禁）"
	default:
		return "仅记录，不处罚"
	}
}
