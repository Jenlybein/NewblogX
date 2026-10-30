-- 日志库
CREATE DATABASE IF NOT EXISTS blogx_logs;

-- 统一说明：
-- 1) 六张日志表均包含核心 12 列 + extra_json
-- 2) 保留 message/request_id/log_kind 作为查询与兼容辅助列
-- 3) 部分日志类型保留场景扩展列（HTTP/Audit/CDC/Replay/AI 用量）
-- 4) 全部使用 ReplacingMergeTree：日志写入是「至少一次」语义——
--    WAL 重放可能重复投递同一批（写完 CH、还没删 WAL 文件时崩溃）。
--    去重键是 ORDER BY 里的 event_id。需要立即去重的查询用 FINAL，
--    或 count(DISTINCT event_id)；不做 FINAL 时后台合并最终也会收敛。
--    前提：重放投递的是磁盘上的同一份字节，ORDER BY 元组（含 ts）完全一致；
--    若日志被重新生成（ts 变化），会被当成新事件，不会去重。
-- 5) L.10（2026-10-24）审计字段瘦身：action_audit_logs 删除了 4 个 *_raw 列，
--    request_body/response_body 仅失败请求时写并加 CODEC(ZSTD(9))，新增 payload_ref 预留列。
--    存量部署（表已存在时 CREATE IF NOT EXISTS 不改结构）需手工迁移：
--      ALTER TABLE blogx_logs.action_audit_logs DROP COLUMN IF EXISTS request_body_raw;
--      ALTER TABLE blogx_logs.action_audit_logs DROP COLUMN IF EXISTS response_body_raw;
--      ALTER TABLE blogx_logs.action_audit_logs DROP COLUMN IF EXISTS request_header_raw;
--      ALTER TABLE blogx_logs.action_audit_logs DROP COLUMN IF EXISTS response_header_raw;
--      ALTER TABLE blogx_logs.action_audit_logs ADD COLUMN IF NOT EXISTS payload_ref String;
--      ALTER TABLE blogx_logs.action_audit_logs MODIFY COLUMN request_body String CODEC(ZSTD(9));
--      ALTER TABLE blogx_logs.action_audit_logs MODIFY COLUMN response_body String CODEC(ZSTD(9));

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
) ENGINE = ReplacingMergeTree
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
) ENGINE = ReplacingMergeTree
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
    -- 仅在失败请求时填充（success=0）；成功请求留空。见 docs/architecture/governance.md §4.2
    request_body String CODEC(ZSTD(9)),
    response_body String CODEC(ZSTD(9)),
    -- 预留：将来启用"原始报文冷存储"时，指向对象存储的取件码；默认不写
    payload_ref String,
    extra_json String
) ENGINE = ReplacingMergeTree
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
    user_id UInt64,
    ip String,
    cdc_job_id String,
    stream LowCardinality(String),
    source_table LowCardinality(String),
    action LowCardinality(String),
    target_key String,
    retry_count UInt8,
    result LowCardinality(String),
    extra_json String
) ENGINE = ReplacingMergeTree
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
    user_id UInt64,
    ip String,
    cdc_job_id String,
    stream LowCardinality(String),
    source_table LowCardinality(String),
    action LowCardinality(String),
    target_key String,
    retry_count UInt8,
    result LowCardinality(String),
    extra_json String
) ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (stream, ts, source_table, event_id)
TTL toDateTime(ts) + INTERVAL 120 DAY
SETTINGS index_granularity = 8192;

-- AI 调用用量（ai_call）
--
-- 与其余五张表的两处刻意不同：
--  1) TTL 400 天而非 120 天：后台首页要出「本年」聚合，120 天会把前八个月的
--     柱子抹平。本表行数与 AI 调用次数同阶（每天几十~几千行，单行约 400B），
--     多留半年可以忽略。
--  2) ORDER BY 以 ts 打头：这张表的查询**全部**带时间窗（日志页翻页、首页
--     聚合），没有"按维度前缀裁剪"的场景；把 scene 放前面反而让主键区间
--     裁剪失效。
--
-- 不记 prompt / 回复正文：日志管道有 32KB 单条上限，超限是整条丢弃；且正文
-- 属于用户内容，不该出现在访问权限更宽的日志目录里。
CREATE TABLE IF NOT EXISTS blogx_logs.ai_call_logs (
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
    scene LowCardinality(String),
    vendor LowCardinality(String),
    model LowCardinality(String),
    stream UInt8,
    success UInt8,
    attempts UInt8,
    prompt_tokens UInt32,
    completion_tokens UInt32,
    total_tokens UInt32,
    cached_tokens UInt32,
    reasoning_tokens UInt32,
    usage_available UInt8,
    latency_ms UInt32,
    extra_json String
) ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (ts, scene, event_id)
TTL toDateTime(ts) + INTERVAL 400 DAY
SETTINGS index_granularity = 8192;
