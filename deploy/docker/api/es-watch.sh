#!/bin/sh
# ES 集群看门狗：定期采样 _cluster/health 与 JVM uptime，只在"发生变化"时写日志。
#
# 为什么需要它：
#   compose 的 depends_on: service_healthy 只是启动瞬间的一次快照。ES 之后被
#   restart: always 悄悄拉回来，`docker compose ps` 永远显示 healthy，
#   "ES 挂过 / 重启过"这件事没有任何地方留下记录 —— 直到有人发现搜不到文章。
#
# 为什么只在变化时写：
#   判定"现在能不能用"是 healthcheck 的职责；这里要的是"什么时候出过事"的证据。
#   每分钟无脑打一行会把日志冲成噪音，反而更看不出异常。
#
# 采样内容与判据：
#   - 连不上          -> ERROR（断开期间每次采样都记，恢复时补一行）
#   - status 变化     -> WARN（green / yellow / red）
#   - JVM uptime 倒退 -> WARN（唯一可靠的"重启过"信号：即使重启正好发生在
#                       两次采样之间，uptime 也会比上次小）
#   - 其余            -> 每小时一条心跳，证明看门狗自己还活着
#
# 输出走容器的标准输出，所以和业务日志混在一起。用前缀过滤：
#   docker compose logs blogx_server | grep '\[es-watch\]'
#
# 用法：由 entrypoint.sh 在 exec 服务进程之前用 & 拉起，不阻塞主进程。
# 依赖：Alpine busybox 自带的 wget / base64 / sed / date，不引入 jq。

set -u

# 采样间隔（秒）。改这里而不是加环境变量：这是固定成本，不需要按部署形态调。
interval=60

es_url="${BLOGX_ES_URL:-http://es:9200}"
es_user="${BLOGX_ES_USERNAME:-elastic}"
auth="Authorization: Basic $(printf '%s:%s' "$es_user" "${BLOGX_ES_PASSWORD:-}" | base64 | tr -d '\n')"

log() {
  printf '[es-watch] %s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

# 从 JSON 里取字段。响应结构固定，只需要取两个值，不值得为此引入 jq。
# pick_num 用贪婪匹配取最后一个匹配项；这些键名在响应里都是唯一的。
pick_num() { sed -n "s/.*\"$1\":\([0-9][0-9]*\).*/\1/p"; }
pick_str() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }

fetch() {
  # -T 10 是必须的：ES 端口被拒绝时 wget 立刻失败，但如果 ES 接受了连接却迟迟
  # 不响应（GC 卡住、磁盘 IO 挂死），没有超时就会一直挂着，看门狗就哑了。
  wget -q -T 10 --header="$auth" -O - "$1" 2>/dev/null
}

last_status=""
last_uptime=""
misses=0
beats=0

log "开始监视 ES：${es_url}（每 ${interval}s 采样，仅在状态变化时记录）"

while :; do
  health="$(fetch "${es_url}/_cluster/health")"
  if [ -z "$health" ]; then
    misses=$((misses + 1))
    if [ "$misses" -eq 1 ]; then
      log "ERROR ES 不可达，连通性中断"
    else
      log "ERROR ES 仍不可达（第 ${misses} 次采样）"
    fi
    # 清空状态基线：恢复后要重新报一次"当前状态"，而不是静默接着用旧值比。
    last_status=""
    sleep "$interval"
    continue
  fi

  if [ "$misses" -gt 0 ]; then
    log "INFO ES 恢复可用（此前连续 ${misses} 次采样失败）"
    misses=0
  fi

  status="$(printf '%s' "$health" | pick_str status)"
  unassigned="$(printf '%s' "$health" | pick_num unassigned_shards)"

  # uptime 单独取一次：只看 status 无法发现"重启后又回到 green"这种情况。
  stats="$(fetch "${es_url}/_nodes/stats?filter_path=nodes.*.jvm.uptime_in_millis")"
  uptime="$(printf '%s' "$stats" | pick_num uptime_in_millis)"

  if [ -n "$uptime" ] && [ -n "$last_uptime" ] && [ "$uptime" -lt "$last_uptime" ]; then
    log "WARN 检测到 ES 重启：JVM uptime ${last_uptime}ms -> ${uptime}ms"
  fi

  if [ "$status" != "$last_status" ]; then
    if [ -z "$last_status" ]; then
      log "INFO 当前状态 status=${status} unassigned_shards=${unassigned} uptime_ms=${uptime:-未知}"
    else
      log "WARN 集群状态变化 ${last_status} -> ${status}（unassigned_shards=${unassigned}）"
    fi
  fi

  last_status="$status"
  if [ -n "$uptime" ]; then
    last_uptime="$uptime"
  fi

  beats=$((beats + 1))
  if [ "$beats" -ge 60 ]; then
    beats=0
    log "INFO 心跳 status=${status} unassigned_shards=${unassigned} uptime_ms=${uptime:-未知}"
  fi

  sleep "$interval"
done
