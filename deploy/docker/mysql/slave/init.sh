#!/bin/bash

main() (
  set -euo pipefail

  : "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD 未设置}"
  : "${BLOGX_DB_NAME:?BLOGX_DB_NAME 未设置}"

  BLOGX_DB_REPL_USER="${BLOGX_DB_REPL_USER:-repl}"
  BLOGX_DB_REPL_PASSWORD="${BLOGX_DB_REPL_PASSWORD:-${MYSQL_ROOT_PASSWORD}}"

  sql_escape_string() {
    local value="$1"
    value=${value//\\/\\\\}
    value=${value//\'/\'\'}
    printf '%s' "$value"
  }

  sql_escape_identifier() {
    local value="$1"
    value=${value//\`/\`\`}
    printf '%s' "$value"
  }

  run_local_mysql() {
    if mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" > /dev/null 2>&1; then
      mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" "$@"
      return
    fi

    mysql -uroot "$@"
  }

  run_master_mysql() {
    mysql -h mysql-master -uroot -p"${MYSQL_ROOT_PASSWORD}" "$@"
  }

  echo "等待主库 mysql-master:3306 启动..."
  until run_master_mysql -e "SELECT 1" > /dev/null 2>&1; do
    sleep 2
  done

  local repl_user_escaped
  local repl_password_escaped
  local db_name_escaped

  repl_user_escaped="$(sql_escape_string "${BLOGX_DB_REPL_USER}")"
  repl_password_escaped="$(sql_escape_string "${BLOGX_DB_REPL_PASSWORD}")"
  db_name_escaped="$(sql_escape_identifier "${BLOGX_DB_NAME}")"

  echo "开始配置从库..."
  # 8.0.23 起复制术语改名：CHANGE MASTER TO → CHANGE REPLICATION SOURCE TO、
  # START/STOP/SHOW SLAVE → START/STOP/SHOW REPLICA。旧语法在 8.0 仍可用但会打
  # deprecation 警告，8.4 起移除；这里直接用新语法，与 mysql:8.0 镜像保持一致。
  run_local_mysql <<SQL
SET GLOBAL read_only = 0;
STOP REPLICA;
RESET REPLICA ALL;
CHANGE REPLICATION SOURCE TO
SOURCE_HOST='mysql-master',
SOURCE_USER='${repl_user_escaped}',
SOURCE_PASSWORD='${repl_password_escaped}',
SOURCE_PORT=3306,
SOURCE_AUTO_POSITION=1,
SOURCE_CONNECT_RETRY=10;
START REPLICA;
SET GLOBAL read_only = 1;
SHOW REPLICA STATUS\G;
SQL

  run_master_mysql -e "CREATE DATABASE IF NOT EXISTS \`${db_name_escaped}\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;"
)

main "$@"
