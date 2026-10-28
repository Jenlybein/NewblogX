#!/usr/bin/env bash
# 部署后验收：确认后台组件真的在跑，而不只是"进程还活着"。
#
# 为什么需要它：river 死掉**不会**让进程退出 —— 容器照常 healthy、站点照常能开，
# 只是新文章永远进不了 ES，且没有任何一处报错。2026-10-29 的事故就是这样：
# 镜像在 02:03 部署，全量转储当场失败、river 静默退出，直到有人发现首页是空的。
#
# 这类故障只能在"有人在看着"的时刻暴露，而部署正是那一刻。所以把它做成部署的
# 最后一步：不通过就不要宣布部署成功。
#
# 为什么不直接把容器 healthcheck 改成 /health/ready：nginx 是
# `depends_on: blogx_server: condition: service_healthy`，一旦 CDC 挂掉让容器
# 变 unhealthy，下次 compose up 会连 nginx 一起起不来 —— CDC 坏了不该导致全站
# 不可用。所以 healthcheck 继续留在 /health（存活），就绪判定放在部署流程里。
#
# 用法（在服务器上，仓库根目录）：
#   ./scripts/verify-deploy.sh                     # 默认 compose 文件 + blogx_server
#   ./scripts/verify-deploy.sh blogx_server        # 指定服务名
#   BLOGX_COMPOSE_FILE=/path/to/compose.yml ./scripts/verify-deploy.sh
#
# 环境变量：
#   BLOGX_READY_TIMEOUT=30   等待就绪的秒数（容器刚重建时 river 还在做位点解析）
#   BLOGX_REQUIRE_CDC=0      允许 cdc_river 缺席（只跑 -role=api 的部署用）
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

COMPOSE_FILE="${BLOGX_COMPOSE_FILE:-$ROOT/deploy/compose/production/docker-compose.yml}"
SERVICE="${1:-blogx_server}"
TIMEOUT_SECONDS="${BLOGX_READY_TIMEOUT:-30}"
REQUIRE_CDC="${BLOGX_REQUIRE_CDC:-1}"

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "找不到 compose 文件：$COMPOSE_FILE" >&2
  exit 2
fi

compose() { docker compose -f "$COMPOSE_FILE" "$@"; }

# 端口从容器内的环境变量读，不把 8080 写死。
read_ready() {
  compose exec -T "$SERVICE" sh -c 'wget -qO- "http://127.0.0.1:${BLOGX_APP_PORT}/health/ready"' 2>/dev/null
}

# 逐项判定，避免"healthy:true 但 CDC 其实是 disabled"这种假通过。
verdict_ok() {
  printf '%s' "$1" | grep -q '"healthy":true' || return 1

  if [ "$REQUIRE_CDC" = "1" ]; then
    printf '%s' "$1" | grep -q '"name":"cdc_river","state":"running"' || return 1
  fi

  return 0
}

deadline=$(( $(date +%s) + TIMEOUT_SECONDS ))
body=""
while :; do
  body="$(read_ready)"
  if verdict_ok "$body"; then
    echo "就绪检查通过：$body"
    exit 0
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    break
  fi
  sleep 2
done

echo "就绪检查失败（等了 ${TIMEOUT_SECONDS}s）。后台组件状态：" >&2
echo "${body:-<读不到 /health/ready：容器可能在重启中，或 BLOGX_APP_PORT 不对>}" >&2
echo >&2
echo "排查顺序：" >&2
echo "  1) docker compose -f $COMPOSE_FILE logs $SERVICE --tail 200 | grep -iE 'river|canal|失败'" >&2
echo "  2) 全量转储失败（exit 2 / MySQL 1231）：确认镜像里的 dump 客户端没被换回 MariaDB 版本" >&2
echo "  3) 确认启动参数是 -role=all：-role=api 时进程里根本没有 CDC 消费者" >&2
exit 1
