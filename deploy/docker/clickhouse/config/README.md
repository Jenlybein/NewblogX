# ClickHouse 配置覆盖层

本目录 + 同级 `users.d/`,挂载进容器覆盖官方镜像的默认配置。
目的是让这套编排在**任何机器上拉起都不会重演** `trace_log` 失控。

## 为什么需要它

官方镜像默认打开 query profiler 与 memory profiler,并把所有
`system.*_log` 自监控表设成**无 TTL**。在一台小机器上,结果是:

- `system.trace_log` 无限膨胀(本项目实测 **1.95 亿行 / 2.81 GiB**);
- 后台 merge 线程持续 churn,把仅有的两颗 CPU 占满;
- 机器内存吃紧 → mysqld / ES 热数据被换到 swap → 站点时快时慢。

实测写入速率:**~1418 行/秒**,其中 >99% 是 `Memory`/`MemoryPeak`
采样(来自 `memory_profiler_step`,每次内存分配写一行),
真正的 CPU/Real 采样只占千分之一。

## 目录结构

| 路径 | 挂载到 | 放什么 |
| --- | --- | --- |
| `config/` | `/etc/clickhouse-server/config.d` | **服务器级**设置(见于 `system.server_settings`) |
| `users.d/` | `/etc/clickhouse-server/users.d` | **会话级**设置 / `<profiles>`(见于 `system.settings`) |

**放错目录会被静默忽略**——不报错、不告警,但配置不生效。
判断标准:该名字出现在 `system.settings` 还是 `system.server_settings`。

| 文件 | 作用 |
| --- | --- |
| `config/system-log-ttl.xml` | 给 `system.*_log` 加 TTL + 降采集频率 + 限制后台线程池 |
| `config/logging.xml` | **日志文件轮转**:level 收紧 + 单文件 50 MB + 只留 3 份 |
| `config/low-memory.xml` | 服务器级:内存上限、并发、缓存、连接数 |
| `users.d/profiler-off.xml` | **关键**:关掉 profiler,从源头掐掉 `trace_log` 写入 |
| `users.d/low-memory.xml` | 会话级:`max_memory_usage` |

## 三处独立的日志,别混

小机器上日志能吃出三份开销,各自的开关完全不同:

| 是什么 | 落在哪 | 谁控制 |
| --- | --- | --- |
| `system.trace_log` 等**系统表** | CH 数据目录里 | 表 TTL + profiler 开关(`users.d/profiler-off.xml`) |
| **CH 自身日志文件** | `state/clickhouse/logs/*.log` | CH 的 `<logger>`(`config/logging.xml`) |
| **容器 stdout** | `/var/lib/docker/containers/*-json.log` | compose 的 `logging:` 选项 |

实测过的数字:只修了第一处,另两处仍在涨——日志文件 12 小时攒了 **731 MB**。

## 生效方式

```bash
cd deploy/compose/production
docker compose up -d --force-recreate clickhouse
```

## 四条硬约束(都踩过,改配置前必读)

### 1. `<ttl>` 与 `<engine>` 互斥

一旦给某张 system 表写了 `<engine>`,就**不能再写独立的 `<ttl>`**,
否则启动报 `Code 36: "If 'engine' is specified for system table, TTL
parameters should be specified directly inside 'engine'"`。
反过来写了 `<engine>` 又继承着 `<partition_by>` 也会冲突。

> 本项目只用 `<ttl>`,不写 `<engine>`。
> 例外:`opentelemetry_span_log` 在官方 config 里**本来就有** `<engine>`,
> 所以不能给它加独立 `<ttl>` —— 已在文件里注释掉。

### 2. `background_pool_size` 有下限(>=25)

必须同时满足:

```
>= number_of_free_entries_in_pool_to_execute_mutation            (默认 20)
>= number_of_free_entries_in_pool_to_execute_optimize_entire_partition (默认 25)
```

不满足则 `Code 36` 拒绝启动。注意 24 < 25,写 24 仍然失败。
本项目取 **26**。这类错误**不打 stdout**,只落在挂载出来的 `err.log` 里,
表现为容器无限 `Restarting` —— 排查时容易误判成 OOM。

### 3. `max_memory_usage` 等会话设置必须在 `<profiles>` 里

写在顶层会报 `Code 137: "A setting 'max_memory_usage' appeared at top
level in config ... should be located in <profiles> section"`。

### 4. `total_memory_profiler_step` 不是 25.3 的内建设置

写了报 `Code 115: "Setting total_memory_profiler_step is neither a
builtin setting nor started with the prefix 'SQL_'"`。较新版本才引入。

### 5. `asynchronous_metric_log` 没有 `collect_interval_milliseconds`

`metric_log` 有这个键(默认 1000ms),但 **`asynchronous_metric_log` 没有**。
写了会被**静默忽略**——不报错,行数一点不降。

实测:设了 `<collect_interval_milliseconds>10000</collect_interval_milliseconds>`
之后仍是 179 行/秒,与没写完全一样。

真正的开关是**服务器级** `asynchronous_metrics_update_period_s`(默认 1)。
改到 10 后实测 **179 → 17.9 行/秒**,3 天稳态从 4640 万行降到 464 万行。

> 另一个陷阱:只加 `<ttl>` 或 `flush_interval_milliseconds` 都压不住它。
> `flush_interval` 只控制"攒多久落盘一次",不影响写入行数。

### 6. `<logger>` 默认值很危险,必须显式覆盖

官方镜像默认:

```xml
<level>trace</level>   <!-- trace 级,极其啰嗦 -->
<size>1000M</size>     <!-- 单文件上限 1 GB -->
<count>10</count>      <!-- 保留 10 份 -->
```

即单类日志理论上能堆到 **10 GB**。实测 12 小时攒了 **731 MB**
(`clickhouse-server.log` 75.8 MB + 8 个 gz 备份;**err.log** 10.7 MB + 6 个 gz 备份)。

`config/logging.xml` 收紧为 `warning` / `50M` / `3` 后,新日志文件
**30 秒零增长**,只剩 Warning 行,trace 刷屏消失。

> 覆盖 `<logger>` 时**必须把 `<log>` 和 `<errorlog>` 路径一起写全**。
> config.d 的合并语义在不同版本上不一致,只写 level/size/count 有可能
> 整体替换 `<logger>` 节点,导致路径丢失、日志写到别处或干脆不写。
>
> 注意这里的文件与 compose 的 `logging:` 选项、`system.text_log` 表
> 是三件不同的事,见上文「三处独立的日志」。

## 存量数据:TTL 是惰性的

配置只对新写入+后续 merge 生效。**历史数据不会立刻消失**。
本次实测的处理方式:

1. TTL 规则生效后,CH 把旧结构表重命名为 `<原名>_0` 并保留
   (如 `trace_log_0` 仍占 2.84 GiB);
2. 这些 `_0` 表是孤儿,需要手动确认后 DROP;
3. 清理后磁盘 **3.6 GB → 40 MB**。

一次性清理命令:

```bash
for t in trace_log_0 asynchronous_metric_log_0 text_log_0 latency_log_0 \
         metric_log_0 error_log_0 processors_profile_log_0 query_log_0 part_log_0; do
  clickhouse-client --query "DROP TABLE IF EXISTS system.\`$t\` SYNC"
done
```

> DROP 前请先自行确认这些确实是遗留旧表(名字带 `_0` 后缀)。
> **不要对业务表 `blogx_logs.*` 执行任何 DROP。**

## 排查技巧

ClickHouse 的配置错误很少出现在容器 stdout。查法:

```bash
# 报错都在挂载出来的 err.log 里
sudo tail -50 deploy/state/clickhouse/logs/clickhouse-server.err.log

# 错误 Row indicate `<Error> Application:` + `Code: NN`
```

本次所有 `Code 36` / `137` / `115` / `347` 都是靠它定位的。
