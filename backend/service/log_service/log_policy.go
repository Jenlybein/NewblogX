package log_service

import (
	"strings"

	"myblogx/conf"
)

const (
	defaultStackMaxBytes   = 8192
	defaultCauseChainDepth = 5
)

func traceEnabled(cfg conf.Logrus) bool {
	if cfg.Trace == (conf.LogTraceConfig{}) {
		return true
	}
	return cfg.Trace.Enabled
}

func TraceEnabled(cfg conf.Logrus) bool {
	return traceEnabled(cfg)
}

func requestIDEqualsTraceID(cfg conf.Logrus) bool {
	if cfg.Trace == (conf.LogTraceConfig{}) {
		return true
	}
	return cfg.Trace.RequestIDEqualsTraceID
}

func RequestIDEqualsTraceID(cfg conf.Logrus) bool {
	return requestIDEqualsTraceID(cfg)
}

func inheritTraceFromGateway(cfg conf.Logrus) bool {
	if cfg.Trace == (conf.LogTraceConfig{}) {
		return true
	}
	return cfg.Trace.InheritFromGateway
}

func InheritTraceFromGateway(cfg conf.Logrus) bool {
	return inheritTraceFromGateway(cfg)
}

func gatewayHeaderPriority(cfg conf.Logrus) []string {
	raw := strings.TrimSpace(cfg.Trace.GatewayHeaderPriority)
	if raw == "" {
		return []string{"traceparent", "x-request-id"}
	}
	raw = strings.ReplaceAll(raw, ",", ">")
	parts := strings.Split(raw, ">")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		header := strings.ToLower(strings.TrimSpace(p))
		if header == "" {
			continue
		}
		out = append(out, header)
	}
	if len(out) == 0 {
		return []string{"traceparent", "x-request-id"}
	}
	return out
}

func GatewayHeaderPriority(cfg conf.Logrus) []string {
	return gatewayHeaderPriority(cfg)
}

func shouldCaptureStack(cfg conf.Logrus, level string) bool {
	if cfg.Error == (conf.LogErrorConfig{}) {
		return stringsToRank(level) >= stringsToRank("error")
	}
	if !cfg.Error.CaptureStack {
		return false
	}
	minLevel := cfg.Error.CaptureMinLevel
	if minLevel == "" {
		minLevel = "error"
	}
	return stringsToRank(level) >= stringsToRank(minLevel)
}

func ShouldCaptureStack(cfg conf.Logrus, level string) bool {
	return shouldCaptureStack(cfg, level)
}

func stackMaxBytes(cfg conf.Logrus) int {
	if cfg.Error.StackMaxBytes <= 0 {
		return defaultStackMaxBytes
	}
	return cfg.Error.StackMaxBytes
}

func StackMaxBytes(cfg conf.Logrus) int {
	return stackMaxBytes(cfg)
}

func causeChainDepth(cfg conf.Logrus) int {
	if cfg.Error.CauseChainDepth <= 0 {
		return defaultCauseChainDepth
	}
	return cfg.Error.CauseChainDepth
}

func CauseChainDepth(cfg conf.Logrus) int {
	return causeChainDepth(cfg)
}

func stringsToRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return 1
	case "info":
		return 2
	case "warn":
		return 3
	case "error":
		return 4
	default:
		return 0
	}
}

func truncateByBytes(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	return value[:maxBytes], true
}

func TruncateByBytes(value string, maxBytes int) (string, bool) {
	return truncateByBytes(value, maxBytes)
}
