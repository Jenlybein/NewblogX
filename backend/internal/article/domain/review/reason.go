package review

import (
	"strings"

	"myblogx/internal/kernel/enum/article_review_enum"
)

// 驳回原因 = 固定枚举（值）+ 可选的一句自由文本。
//
// 本文件只回答两件事：**两者怎么装进审核任务已有的 reason 列**（零 DDL：不能加列），
// 以及**作者看到的到底是哪句话**。"必须选一条"是用例层的事（app.reviewReason）。

// ReasonCode 是驳回原因的固定枚举值。
//
// 别名到 kernel/enum：词表只在那一处声明（GLOSSARY「枚举别名」），
// 这里重新声明一份会让契约与代码各自演进。
type ReasonCode = article_review_enum.ReasonCode

// 驳回原因取值（别名，方便本包与调用方少 import 一层）。
const (
	// ReasonQuality 内容质量不达标。
	ReasonQuality = article_review_enum.ReasonQuality
	// ReasonCompliance 内容不合规。
	ReasonCompliance = article_review_enum.ReasonCompliance
	// ReasonIncomplete 内容不完整。
	ReasonIncomplete = article_review_enum.ReasonIncomplete
	// ReasonFormat 排版或格式问题。
	ReasonFormat = article_review_enum.ReasonFormat
	// ReasonOther 其他原因（说明必须补一句，否则作者只看到「其他原因」）。
	ReasonOther = article_review_enum.ReasonOther
)

// ReasonNoteMaxLen 是补充说明的字数上限。
//
// reason 列是 varchar(500)，而且枚举值也编在同一列里；上限取 300 字符，
// 既留足了枚举前缀，也让"一句人话"不至于变成一篇申诉。用例层负责拦截，
// 超长文本进到写库只会换来一个 500。
const ReasonNoteMaxLen = 300

// ReasonMissingFallback 是审核任务没填原因时的兜底文案。
//
// #303 之前驳回原因不是必填，历史任务行的 reason 可能是空串或纯空白。
// 把空串原样拼进通知，作者看到的是"失败原因："后面什么都没有——比不写这一行
// 更糟（会以为界面坏了）。兜底文案只陈述事实（没写原因）并指出下一步，
// 不替审核员编一个理由。
const ReasonMissingFallback = "审核员未填写具体原因，如有疑问请联系管理员"

// FormatReason 把"枚举 + 补充说明"编码进审核任务已有的 reason 列。
//
// 零 DDL：枚举值没有自己的列（既有表不允许加列），所以编进 reason 里：
//
//	第一行 = 枚举值；换行之后的全部内容 = 自由文本；没有自由文本时只有一行。
//
// 为什么用换行而不是 JSON：换行是唯一既不需要转义、又不会跟自由文本里的标点
// 撞车的分隔符，而且自由文本自己可以含换行——从第二行起原样保留，不多一层转义。
// 历史行（#303 之前的自由文本、撤销原因）第一行不是已知枚举值，
// ParseReason 会把整串当文本，作者看到的内容一字不变。
func FormatReason(code ReasonCode, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return string(code)
	}
	return string(code) + "\n" + note
}

// ParseReason 把 reason 列拆回"枚举 + 自由文本"。
//
// code 为空串表示这一行没有枚举：历史行、撤销原因，或将来被下线掉的旧取值——
// 这些情况一律按纯文本处理，不能让作者看到 "quality_v2" 这种机器值。
func ParseReason(raw string) (ReasonCode, string) {
	// 先去尾部的换行：只选了枚举的行不该因为尾部空白而被当成"有补充说明"。
	raw = strings.TrimRight(raw, "\r\n")
	first, rest, _ := strings.Cut(raw, "\n")
	code := ReasonCode(strings.TrimSpace(first))
	if code.Label() == "" {
		return "", strings.TrimSpace(raw)
	}
	return code, strings.TrimSpace(rest)
}

// ReasonText 返回**给作者看**的驳回原因：枚举文案 + 自由文本，空白一律换成兜底。
//
// 这是作者可见内容的唯一渲染入口：#303 建立的送达链路（驳回通知正文）与
// 作者打开被打回的草稿看到的那句话都走它，替换文案只改这里。
// 历史行没有枚举时原样呈现整串；空串才落兜底文案。
func ReasonText(raw string) string {
	code, note := ParseReason(raw)
	if code == "" {
		if note == "" {
			return ReasonMissingFallback
		}
		return note
	}
	if note == "" {
		return code.Label()
	}
	// 一级标签 + 补充说明：作者一眼能看出这是"哪一类问题"，再看具体那句话。
	return code.Label() + "：" + note
}

// ReasonCodes 返回固定枚举的全部取值，顺序即下拉框顺序（词表见 kernel/enum）。
func ReasonCodes() []ReasonCode { return article_review_enum.ReasonCodes() }

// ReasonLabel 返回取值的中文文案；未知取值返回空串（据此判断"不是固定枚举"）。
func ReasonLabel(code ReasonCode) string { return code.Label() }
