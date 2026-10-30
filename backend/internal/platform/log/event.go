// Package log 是有界、类型化、异步的日志管道。
//
// 设计目标（见 docs/architecture/governance.md §四）：
//  1. Emit 只做投递，O(1)，不序列化、不写盘、不阻塞请求路径；
//  2. 进程内缓冲有硬上限（条目数 + 字节数双限），超限丢弃并计数，绝不无限增长；
//  3. 字段是封闭类型集合，禁止 any、禁止传结构体（从类型上杜绝反射大对象）。
//
// 本包不依赖 logrus：日志管道是基础设施，不能反过来被业务日志库绑死。
package log

import (
	"strconv"
	"time"
)

// Level 是日志级别，值越小越详细。
type Level uint8

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String 返回可读级别名。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// enabled 判断给定级别是否达到当前阈值。
func (l Level) enabled(min Level) bool { return l >= min }

// Kind 是日志分类，决定“可丢/不可丢”与目标表。
type Kind uint8

const (
	KindRuntime Kind = iota
	KindAccess
	KindAudit
	KindLogin
	KindCDC
	KindReplay
	// KindAICall 是 AI 模型调用记录（场景、模型、token 用量、耗时）。
	// 只能追加在末尾：已有值被写进了历史日志的 log_kind 字段，改值会让旧数据错位。
	KindAICall
)

// String 返回可读分类名，直接用作 JSON 字段值。
func (k Kind) String() string {
	switch k {
	case KindRuntime:
		return "runtime"
	case KindAccess:
		return "access"
	case KindAudit:
		return "audit"
	case KindLogin:
		return "login"
	case KindCDC:
		return "cdc"
	case KindReplay:
		return "replay"
	case KindAICall:
		return "ai_call"
	default:
		return "unknown"
	}
}

// valueKind 是 Field 值的封闭类型标签。
type valueKind uint8

const (
	kindString valueKind = iota
	kindInt
	kindUint
	kindFloat
	kindBool
	kindDuration
	kindTime
	kindStrings
)

// maxStringsLen 限制 []string 字段长度，避免一个超长列表拖垮编码。
const maxStringsLen = 32

// fieldValue 是封闭的值联合。没有 any，也没有反射入口。
type fieldValue struct {
	kind valueKind
	s    string
	i    int64
	u    uint64
	f    float64
	b    bool
	d    time.Duration
	t    time.Time
	ss   []string
}

// Field 是一个结构化字段。值只能由本文件的构造函数产生。
type Field struct {
	Key string
	val fieldValue
}

// Str 构造字符串字段。
func Str(key, v string) Field { return Field{Key: key, val: fieldValue{kind: kindString, s: v}} }

// Int 构造有符号整数字段。
func Int(key string, v int64) Field { return Field{Key: key, val: fieldValue{kind: kindInt, i: v}} }

// Uint 构造无符号整数字段。
func Uint(key string, v uint64) Field { return Field{Key: key, val: fieldValue{kind: kindUint, u: v}} }

// Float 构造浮点字段。
func Float(key string, v float64) Field {
	return Field{Key: key, val: fieldValue{kind: kindFloat, f: v}}
}

// Bool 构造布尔字段。
func Bool(key string, v bool) Field { return Field{Key: key, val: fieldValue{kind: kindBool, b: v}} }

// Dur 构造时长字段（编码为毫秒数，便于聚合）。
func Dur(key string, v time.Duration) Field {
	return Field{Key: key, val: fieldValue{kind: kindDuration, d: v}}
}

// Time 构造时间字段（编码为 RFC3339Nano）。
func Time(key string, v time.Time) Field {
	return Field{Key: key, val: fieldValue{kind: kindTime, t: v}}
}

// Strs 构造字符串列表字段，超过 maxStringsLen 的部分会被截断。
func Strs(key string, v []string) Field {
	if len(v) > maxStringsLen {
		v = v[:maxStringsLen]
	}
	return Field{Key: key, val: fieldValue{kind: kindStrings, ss: v}}
}

// Base 是所有事件共有的标量字段。出现这里之外的结构体字段都算设计事故。
type Base struct {
	EventID      uint64
	TS           time.Time
	Level        Level
	Kind         Kind
	Service      string
	Env          string
	InstanceID   string
	RequestID    string
	TraceID      string
	SpanID       string
	ParentSpanID string
	EventName    string
	ErrorCode    string
	ErrorMsg     string
	UserID       uint64
	IP           string
}

// Event 是一条日志。Message 与 Fields 之外没有别的入口，
// 因此“把一个大结构体塞进日志”在类型层面就不可能发生。
type Event struct {
	Base
	Message string
	Fields  []Field
}

// size 估算该事件占用的字节数（只做加法，不序列化）。
// 估算偏保守即可——它的用途是给有界缓冲做预算，不要求精确。
func (e *Event) size() int64 {
	const baseOverhead = 160
	n := int64(baseOverhead) + int64(len(e.Message))
	n += int64(len(e.Service) + len(e.Env) + len(e.InstanceID))
	n += int64(len(e.RequestID) + len(e.TraceID) + len(e.SpanID) + len(e.ParentSpanID))
	n += int64(len(e.EventName) + len(e.ErrorCode) + len(e.ErrorMsg) + len(e.IP))
	for i := range e.Fields {
		f := &e.Fields[i]
		n += int64(len(f.Key)) + 8
		switch f.val.kind {
		case kindString, kindStrings:
			n += int64(len(f.val.s))
			for _, s := range f.val.ss {
				n += int64(len(s))
			}
		case kindTime:
			n += 32
		default:
			n += 8
		}
	}
	return n
}

// quote 把字符串编码为 JSON 字符串字面量。仅用于写协程，不在 Emit 路径上。
func quote(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigit(c>>4), hexDigit(c&0xF))
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

// hexDigit 返回 0-15 的十六进制字符。
func hexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + v - 10
}

// appendUint 追加无符号整数的十进制表示。
func appendUint(dst []byte, v uint64) []byte { return strconv.AppendUint(dst, v, 10) }
