#!/bin/sh
# blogx nginx 配置渲染脚本
#
# 只用一个开关 BLOGX_NGINX_ENV 选择部署形态（local / test / production / api-only），
# 由此推导出模板需要的变量，再用 envsubst 渲染成 /etc/nginx/nginx.conf。
#
# 显式传入的同名环境变量优先于这里的默认值，便于临时覆盖（例如指向别的 upstream）。

set -eu
set -a

PROFILE="${BLOGX_NGINX_ENV:-local}"

case "$PROFILE" in
  local)
    : "${BLOGX_NGINX_SERVER_NAME:=localhost}"
    : "${BLOGX_NGINX_LISTEN_443:=}"
    : "${BLOGX_NGINX_SSL_CERT:=}"
    : "${BLOGX_NGINX_SSL_KEY:=}"
    : "${BLOGX_NGINX_WEB_UPSTREAM:=blogx_web:3000}"
    : "${BLOGX_NGINX_ADMIN_UPSTREAM:=blogx_admin:3001}"
    : "${BLOGX_NGINX_SERVE_WEB:=1}"
    : "${BLOGX_NGINX_SERVE_ADMIN:=1}"
    ;;
  test)
    : "${BLOGX_NGINX_SERVER_NAME:=blog.gentlybeing.cn}"
    : "${BLOGX_NGINX_LISTEN_443:=listen 443 ssl;}"
    : "${BLOGX_NGINX_SSL_CERT:=ssl_certificate /usr/share/nginx/cert/domain.pem;}"
    : "${BLOGX_NGINX_SSL_KEY:=ssl_certificate_key /usr/share/nginx/cert/domain.key;}"
    : "${BLOGX_NGINX_WEB_UPSTREAM:=blogx_web:3000}"
    : "${BLOGX_NGINX_ADMIN_UPSTREAM:=blogx_admin:3001}"
    : "${BLOGX_NGINX_SERVE_WEB:=1}"
    : "${BLOGX_NGINX_SERVE_ADMIN:=1}"
    ;;
  production)
    : "${BLOGX_NGINX_SERVER_NAME:=blogx.gentlybeing.cn}"
    : "${BLOGX_NGINX_LISTEN_443:=listen 443 ssl;}"
    : "${BLOGX_NGINX_SSL_CERT:=ssl_certificate /usr/share/nginx/cert/domain.pem;}"
    : "${BLOGX_NGINX_SSL_KEY:=ssl_certificate_key /usr/share/nginx/cert/domain.key;}"
    : "${BLOGX_NGINX_WEB_UPSTREAM:=blogx_web:3000}"
    : "${BLOGX_NGINX_ADMIN_UPSTREAM:=blogx_admin:3001}"
    : "${BLOGX_NGINX_SERVE_WEB:=1}"
    : "${BLOGX_NGINX_SERVE_ADMIN:=1}"
    ;;
  api-only)
    : "${BLOGX_NGINX_SERVER_NAME:=blog.gentlybeing.cn}"
    : "${BLOGX_NGINX_LISTEN_443:=listen 443 ssl;}"
    : "${BLOGX_NGINX_SSL_CERT:=ssl_certificate /usr/share/nginx/cert/domain.pem;}"
    : "${BLOGX_NGINX_SSL_KEY:=ssl_certificate_key /usr/share/nginx/cert/domain.key;}"
    # 未使用的 web/admin upstream 指向 api 容器，保证 nginx 启动时能解析主机名；
    # 对应 location 由 SERVE_WEB / SERVE_ADMIN = 0 直接返回 404。
    : "${BLOGX_NGINX_WEB_UPSTREAM:=blogx_server:8080}"
    : "${BLOGX_NGINX_ADMIN_UPSTREAM:=blogx_server:8080}"
    : "${BLOGX_NGINX_SERVE_WEB:=0}"
    : "${BLOGX_NGINX_SERVE_ADMIN:=0}"
    ;;
  *)
    echo "未知的 BLOGX_NGINX_ENV: $PROFILE（可选 local|test|production|api-only）" >&2
    exit 1
    ;;
esac

: "${BLOGX_NGINX_API_UPSTREAM:=blogx_server:8080}"

# 只替换 BLOGX_NGINX_*，nginx 自身的 $host / $remote_addr / $http_upgrade 等不受影响。
envsubst '${BLOGX_NGINX_SERVER_NAME} ${BLOGX_NGINX_LISTEN_443} ${BLOGX_NGINX_SSL_CERT} ${BLOGX_NGINX_SSL_KEY} ${BLOGX_NGINX_WEB_UPSTREAM} ${BLOGX_NGINX_ADMIN_UPSTREAM} ${BLOGX_NGINX_API_UPSTREAM} ${BLOGX_NGINX_SERVE_WEB} ${BLOGX_NGINX_SERVE_ADMIN}' \
  < /etc/nginx/nginx.conf.tmpl > /etc/nginx/nginx.conf

exec nginx -g 'daemon off;'
