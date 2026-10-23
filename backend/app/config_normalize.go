package app

import "myblogx/conf"

func normalizeConfig(cfg *conf.Config) {
	if cfg == nil {
		return
	}

	if cfg.Log.Error == (conf.LogErrorConfig{}) {
		cfg.Log.Error.CaptureStack = true
	}
	if cfg.Log.Error.CaptureMinLevel == "" {
		cfg.Log.Error.CaptureMinLevel = "error"
	}
	if cfg.Log.Error.StackMaxBytes <= 0 {
		cfg.Log.Error.StackMaxBytes = 8192
	}
	if cfg.Log.Error.CauseChainDepth <= 0 {
		cfg.Log.Error.CauseChainDepth = 5
	}

	if cfg.Log.Trace == (conf.LogTraceConfig{}) {
		cfg.Log.Trace.Enabled = true
		cfg.Log.Trace.RequestIDEqualsTraceID = true
		cfg.Log.Trace.InheritFromGateway = true
	}
	if cfg.Log.Trace.GatewayHeaderPriority == "" {
		cfg.Log.Trace.GatewayHeaderPriority = "traceparent>x-request-id"
	}

	if cfg.Log.Cleanup == (conf.LogCleanup{}) {
		cfg.Log.Cleanup.Enabled = true
	}
	if cfg.Log.Cleanup.RetentionDays <= 0 {
		cfg.Log.Cleanup.RetentionDays = 7
	}
	if cfg.Log.Cleanup.RunAt == "" {
		cfg.Log.Cleanup.RunAt = "03:30:00"
	}

	if cfg.River.Retry.MaxAttempts <= 0 {
		cfg.River.Retry.MaxAttempts = 2
	}
	if cfg.River.Retry.DelayMS <= 0 {
		cfg.River.Retry.DelayMS = 200
	}

	if cfg.ImageRefRiver.Retry.MaxAttempts <= 0 {
		cfg.ImageRefRiver.Retry.MaxAttempts = 2
	}
	if cfg.ImageRefRiver.Retry.DelayMS <= 0 {
		cfg.ImageRefRiver.Retry.DelayMS = 200
	}

	if cfg.Replay.BatchSize <= 0 {
		cfg.Replay.BatchSize = 100
	}
}
