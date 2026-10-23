#!/bin/sh
set -eu

: "${BLOGX_REDIS_PASSWORD:?BLOGX_REDIS_PASSWORD 未设置}"

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\/&\\]/\\&/g'
}

redis_password_escaped="$(escape_sed_replacement "${BLOGX_REDIS_PASSWORD}")"

sed \
  -e "s/__BLOGX_REDIS_PASSWORD__/${redis_password_escaped}/g" \
  /etc/redis/redis.conf.tmpl > /tmp/redis.conf

exec redis-server /tmp/redis.conf
