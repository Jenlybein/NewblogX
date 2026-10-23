#!/bin/bash

main() (
  set -euo pipefail

  : "${BLOGX_DB_USER:?BLOGX_DB_USER 未设置}"
  : "${BLOGX_DB_PASSWORD:?BLOGX_DB_PASSWORD 未设置}"

  BLOGX_DB_REPL_USER="${BLOGX_DB_REPL_USER:-repl}"
  BLOGX_DB_REPL_PASSWORD="${BLOGX_DB_REPL_PASSWORD:-${BLOGX_DB_PASSWORD}}"

  sql_escape_string() {
    local value="$1"
    value=${value//\\/\\\\}
    value=${value//\'/\'\'}
    printf '%s' "$value"
  }

  run_mysql() {
    if [[ -n "${MYSQL_ROOT_PASSWORD:-}" ]] && mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" > /dev/null 2>&1; then
      mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" "$@"
      return
    fi

    mysql -uroot "$@"
  }

  local db_user_escaped
  local db_password_escaped
  local repl_user_escaped
  local repl_password_escaped

  db_user_escaped="$(sql_escape_string "${BLOGX_DB_USER}")"
  db_password_escaped="$(sql_escape_string "${BLOGX_DB_PASSWORD}")"
  repl_user_escaped="$(sql_escape_string "${BLOGX_DB_REPL_USER}")"
  repl_password_escaped="$(sql_escape_string "${BLOGX_DB_REPL_PASSWORD}")"

  run_mysql <<SQL
CREATE USER IF NOT EXISTS '${db_user_escaped}'@'%' IDENTIFIED BY '${db_password_escaped}';
ALTER USER '${db_user_escaped}'@'%' IDENTIFIED BY '${db_password_escaped}';
GRANT ALL PRIVILEGES ON *.* TO '${db_user_escaped}'@'%' WITH GRANT OPTION;

CREATE USER IF NOT EXISTS '${repl_user_escaped}'@'%' IDENTIFIED BY '${repl_password_escaped}';
ALTER USER '${repl_user_escaped}'@'%' IDENTIFIED BY '${repl_password_escaped}';
GRANT REPLICATION SLAVE ON *.* TO '${repl_user_escaped}'@'%';

FLUSH PRIVILEGES;
SQL
)

main "$@"
