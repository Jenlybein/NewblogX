-- 日志库
CREATE DATABASE IF NOT EXISTS blogx_logs;

-- 统一说明：
-- 1) 五张日志表均包含核心 12 列 + extra_json
-- 2) 保留 message/request_id/log_kind 作为查询与兼容辅助列
-- 3) 部分日志类型保留场景扩展列（HTTP/Audit/CDC/Replay）

-- 运行日志（runtime）
CREATE TABLE IF NOT EXISTS blogx_logs.runtime_logs (
    event_id UInt64,
    ts DateTime64(3, 'Asia/Shanghai'),
    level LowCardinality(String),
    trace_id String,
    span_id String,
    parent_span_id String,
    service LowCardinality(String),
    env LowCardinality(String),
    instance_id LowCardinality(String),
    event_name LowCardinality(String),
    error_code String,
    error_message String,

    message String,
    request_id String,
    log_kind LowCardinality(String),
    host LowCardinality(String),
    file String,
    func String,
    user_id UInt64,
    ip String,
    method LowCardinality(String),
    path String,
    status_code UInt16,
    latency_ms UInt32,
    error_type String,
    error_stack String,
    extra_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (service, ts, level, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;

-- 登录事件日志（login_event）
CREATE TABLE IF NOT EXISTS blogx_logs.login_event_logs (
    event_id UInt64,
    ts DateTime64(3, 'Asia/Shanghai'),
    level LowCardinality(String),
    trace_id String,
    span_id String,
    parent_span_id String,
    service LowCardinality(String),
    env LowCardinality(String),
    instance_id LowCardinality(String),
    event_name LowCardinality(String),
    error_code String,
    error_message String,

    message String,
    request_id String,
    log_kind LowCardinality(String),
    host LowCardinality(String),
    user_id UInt64,
    ip String,
    username String,
    login_type LowCardinality(String),
    success UInt8,
    reason String,
    addr String,
    ua String,
    extra_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (user_id, ts, event_name, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;

-- 操作审计日志（action_audit）
CREATE TABLE IF NOT EXISTS blogx_logs.action_audit_logs (
    event_id UInt64,
    ts DateTime64(3, 'Asia/Shanghai'),
    level LowCardinality(String),
    trace_id String,
    span_id String,
    parent_span_id String,
    service LowCardinality(String),
    env LowCardinality(String),
    instance_id LowCardinality(String),
    event_name LowCardinality(String),
    error_code String,
    error_message String,

    message String,
    request_id String,
    log_kind LowCardinality(String),
    host LowCardinality(String),
    user_id UInt64,
    ip String,
    method LowCardinality(String),
    path String,
    status_code UInt16,
    action_name LowCardinality(String),
    target_type LowCardinality(String),
    target_id String,
    success UInt8,
    request_body String,
    response_body String,
    request_body_raw String,
    response_body_raw String,
    request_header_raw String,
    response_header_raw String,
    extra_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (user_id, ts, action_name, target_type, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;

-- CDC 执行日志（river / image_ref_river）
CREATE TABLE IF NOT EXISTS blogx_logs.cdc_event_logs (
    event_id UInt64,
    ts DateTime64(3, 'Asia/Shanghai'),
    level LowCardinality(String),
    trace_id String,
    span_id String,
    parent_span_id String,
    service LowCardinality(String),
    env LowCardinality(String),
    instance_id LowCardinality(String),
    event_name LowCardinality(String),
    error_code String,
    error_message String,

    message String,
    request_id String,
    log_kind LowCardinality(String),
    host LowCardinality(String),
    cdc_job_id String,
    stream LowCardinality(String),
    source_table LowCardinality(String),
    action LowCardinality(String),
    target_key String,
    retry_count UInt8,
    result LowCardinality(String),
    extra_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (stream, ts, source_table, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;

-- 回放日志（DLQ replay）
CREATE TABLE IF NOT EXISTS blogx_logs.replay_event_logs (
    event_id UInt64,
    ts DateTime64(3, 'Asia/Shanghai'),
    level LowCardinality(String),
    trace_id String,
    span_id String,
    parent_span_id String,
    service LowCardinality(String),
    env LowCardinality(String),
    instance_id LowCardinality(String),
    event_name LowCardinality(String),
    error_code String,
    error_message String,

    message String,
    request_id String,
    log_kind LowCardinality(String),
    host LowCardinality(String),
    cdc_job_id String,
    stream LowCardinality(String),
    source_table LowCardinality(String),
    action LowCardinality(String),
    target_key String,
    retry_count UInt8,
    result LowCardinality(String),
    extra_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (stream, ts, source_table, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;
