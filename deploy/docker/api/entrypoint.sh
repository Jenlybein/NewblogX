#!/bin/sh
set -eu

default_config_file="config/settings.yaml"

resolve_config_file() {
  current="$default_config_file"
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -f)
        shift
        if [ "$#" -gt 0 ] && [ -n "$1" ]; then
          current="$1"
        fi
        ;;
      -f=*)
        current="${1#-f=}"
        ;;
    esac
    shift
  done
  printf '%s' "$current"
}

should_wrap_server_start() {
  if [ "$#" -eq 0 ]; then
    return 0
  fi

  case "$1" in
    /app/server|./server|server)
      ;;
    *)
      return 1
      ;;
  esac

  shift
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -db|--db|-es|--es|-t|--t|-s|--s|-version|--version)
        return 1
        ;;
      -t=*|--t=*|-s=*|--s=*|-es=*|--es=*|-db=*|--db=*|-version=*|--version=*)
        return 1
        ;;
    esac
    shift
  done

  return 0
}

wait_for_es() {
  es_url="${BLOGX_ES_URL:-http://es:9200}"
  es_user="${BLOGX_ES_USERNAME:-elastic}"
  auth_header="Authorization: Basic $(printf '%s:%s' "$es_user" "${BLOGX_ES_PASSWORD:-}" | base64 | tr -d '\n')"

  # -T 15：端口拒绝会立刻失败，但 ES 接受了连接却不响应（GC 卡死、磁盘挂起）时
  # 没有超时就会永远挂在这里，容器永远起不来。
  until wget -q -T 15 --header="$auth_header" -O /dev/null "${es_url}/_cluster/health?wait_for_status=yellow&timeout=5s"; do
    echo "等待 ES 完整启动..."
    sleep 3
  done
}

mysql_client_bin() {
  if command -v mariadb >/dev/null 2>&1; then
    printf '%s' "mariadb"
    return 0
  fi

  printf '%s' "mysql"
}

runtime_table_exists() {
  "$(mysql_client_bin)" \
    -h"${BLOGX_DB_MASTER_HOST}" \
    -P"${BLOGX_DB_MASTER_PORT}" \
    -u"${BLOGX_DB_USER}" \
    -p"${BLOGX_DB_PASSWORD}" \
    --skip-ssl \
    -D"${BLOGX_DB_NAME}" \
    -Nse "SHOW TABLES LIKE 'runtime_config_models';"
}

if [ "$#" -eq 0 ]; then
  set -- /app/server -f "$default_config_file" -role=all
fi

if ! should_wrap_server_start "$@"; then
  exec "$@"
fi

config_file="$(resolve_config_file "$@")"
table_name="$(runtime_table_exists)"

if [ -z "$table_name" ]; then
  echo "检测到数据库未初始化，开始执行数据库迁移与 ES 初始化..."
  /app/server -f "$config_file" -db
  wait_for_es
  /app/server -f "$config_file" -es -s ensure
fi

wait_for_es

# 后台看门狗：depends_on 只保证"启动那一刻"依赖是好的，ES 之后重启我们无从得知。
# 放后台是为了不阻塞下面的 exec；脚本自己做了所有兜底，任何一次采样失败都不会
# 影响主进程。挂载缺失时静默跳过，保证镜像单独跑也能正常启动。
if [ -f /app/es-watch.sh ]; then
  /bin/sh /app/es-watch.sh &
fi

exec "$@"
