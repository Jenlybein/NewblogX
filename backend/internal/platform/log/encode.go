package log

import (
	"strconv"
	"time"
)

// encodeEvent 把事件序列化为一行 JSON（不含换行）。
//
// 只用已知字段拼装，不做反射、不做 map 遍历，因此开销与字段数线性相关。
// timeFormat 决定 ts 字段格式，默认与现有 logrus 输出一致。
// 只在写协程调用。
func encodeEvent(dst []byte, ev *Event, timeFormat string) []byte {
	dst = append(dst, '{')
	first := true

	addRaw := func(k string, write func([]byte) []byte) {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = quote(dst, k)
		dst = append(dst, ':')
		dst = write(dst)
	}
	addStr := func(k, v string) {
		if v == "" {
			return
		}
		addRaw(k, func(b []byte) []byte { return quote(b, v) })
	}

	// 必有字段
	addRaw("ts", func(b []byte) []byte { return quote(b, ev.TS.Format(timeFormat)) })
	addRaw("level", func(b []byte) []byte { return quote(b, ev.Level.String()) })
	addRaw("kind", func(b []byte) []byte { return quote(b, ev.Kind.String()) })
	addRaw("event_id", func(b []byte) []byte { return appendUint(b, ev.EventID) })
	addStr("service", ev.Service)
	addStr("env", ev.Env)
	addStr("instance_id", ev.InstanceID)

	// 可选标量
	addStr("request_id", ev.RequestID)
	addStr("trace_id", ev.TraceID)
	addStr("span_id", ev.SpanID)
	addStr("parent_span_id", ev.ParentSpanID)
	addStr("event", ev.EventName)
	addStr("error_code", ev.ErrorCode)
	addStr("error_msg", ev.ErrorMsg)
	addStr("ip", ev.IP)
	if ev.UserID != 0 {
		addRaw("user_id", func(b []byte) []byte { return appendUint(b, ev.UserID) })
	}
	addStr("message", ev.Message)

	// 结构化字段
	for i := range ev.Fields {
		f := &ev.Fields[i]
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = quote(dst, f.Key)
		dst = append(dst, ':')

		switch f.val.kind {
		case kindString:
			dst = quote(dst, f.val.s)
		case kindInt:
			dst = strconv.AppendInt(dst, f.val.i, 10)
		case kindUint:
			dst = strconv.AppendUint(dst, f.val.u, 10)
		case kindFloat:
			dst = strconv.AppendFloat(dst, f.val.f, 'f', -1, 64)
		case kindBool:
			if f.val.b {
				dst = append(dst, "true"...)
			} else {
				dst = append(dst, "false"...)
			}
		case kindDuration:
			// 编码为毫秒数，便于聚合；字段名自带语义
			dst = strconv.AppendInt(dst, f.val.d.Milliseconds(), 10)
		case kindTime:
			dst = quote(dst, f.val.t.Format(time.RFC3339Nano))
		case kindStrings:
			dst = append(dst, '[')
			for j, s := range f.val.ss {
				if j > 0 {
					dst = append(dst, ',')
				}
				dst = quote(dst, s)
			}
			dst = append(dst, ']')
		default:
			dst = append(dst, "null"...)
		}
	}

	return append(dst, '}')
}
