package log_service

import "strings"

type ReplayEvent struct {
	baseEvent
	CdcJobID    string `json:"cdc_job_id,omitempty"`
	Stream      string `json:"stream,omitempty"`
	SourceTable string `json:"source_table,omitempty"`
	Action      string `json:"action,omitempty"`
	TargetKey   string `json:"target_key,omitempty"`
	RetryCount  int    `json:"retry_count,omitempty"`
	Result      string `json:"result,omitempty"`
}

type ReplayEventInput struct {
	Level        string
	Message      string
	RequestID    string
	TraceID      string
	SpanID       string
	ParentSpanID string
	EventName    string
	ErrorCode    string
	ErrorMessage string
	ErrorType    string
	ErrorStack   string
	CauseChain   []string
	CdcJobID     string
	Stream       string
	SourceTable  string
	Action       string
	TargetKey    string
	RetryCount   int
	Result       string
	Extra        map[string]any
}

func EmitReplayEvent(deps Deps, input ReplayEventInput) {
	level := strings.TrimSpace(strings.ToLower(input.Level))
	if level == "" {
		level = "info"
	}
	base := newBaseEvent(deps, "replay_event", level, input.Message)
	base.RequestID = input.RequestID
	base.TraceID = defaultIfEmptyString(input.TraceID, input.RequestID)
	if base.RequestID == "" && base.TraceID != "" {
		base.RequestID = base.TraceID
	}
	base.SpanID = input.SpanID
	base.ParentSpanID = input.ParentSpanID
	base.EventName = input.EventName
	if base.EventName == "" {
		base.EventName = defaultReplayEventName(input)
	}
	base.ErrorCode = input.ErrorCode
	base.ErrorMessage = input.ErrorMessage

	extra := make(map[string]any, len(input.Extra)+4)
	for key, value := range input.Extra {
		extra[key] = value
	}
	if input.ErrorType != "" {
		extra["error.type"] = input.ErrorType
	}
	if stack := strings.TrimSpace(input.ErrorStack); stack != "" && shouldCaptureStack(deps.LogConfig, level) {
		if clipped, truncated := truncateByBytes(stack, stackMaxBytes(deps.LogConfig)); truncated {
			extra["error.stack"] = clipped
			extra["error_stack_truncated"] = true
		} else {
			extra["error.stack"] = clipped
		}
	}
	if len(input.CauseChain) > 0 {
		depth := causeChainDepth(deps.LogConfig)
		chain := input.CauseChain
		if len(chain) > depth {
			chain = chain[:depth]
		}
		extra["error.cause_chain"] = strings.Join(chain, "->")
	}
	base.ExtraJSON = marshalExtra(extra)

	event := ReplayEvent{
		baseEvent:   base,
		CdcJobID:    input.CdcJobID,
		Stream:      input.Stream,
		SourceTable: input.SourceTable,
		Action:      input.Action,
		TargetKey:   input.TargetKey,
		RetryCount:  input.RetryCount,
		Result:      input.Result,
	}
	if err := replayEventSink().write(deps, event); err != nil && deps.Logger != nil {
		deps.Logger.Errorf("写入回放日志失败: %v", err)
	}
}

func defaultReplayEventName(input ReplayEventInput) string {
	switch strings.ToLower(strings.TrimSpace(input.Result)) {
	case "failed":
		return "replay_failed"
	case "started":
		return "replay_started"
	default:
		return "replay_success"
	}
}
