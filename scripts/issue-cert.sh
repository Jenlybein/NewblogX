#!/usr/bin/env bash
# 首次签发主站 Let's Encrypt 证书，并把续期接进 acme.sh 的 cron。
#
# 为什么需要它：nginx 只认 deploy/state/nginx/cert/{domain.pem,domain.key}，
# 而 acme.sh 的 standalone 方式要临时占用 80 端口 —— 仓库的 nginx 容器正占着它。
# 停/起 nginx 的时序、证书落盘路径、续期后的 reload 命令必须一次配全，
# 漏掉任何一步的症状都是"签发成功、续期后还是旧证书"。
#
# 用法（在仓库根目录、用能直接跑 docker 的用户执行；仓库里的脚本是 644，用 bash 调起）：
#   bash scripts/issue-cert.sh --email you@example.com --domain 你的域名
#   bash scripts/issue-cert.sh --email you@example.com --domain 你的域名 --domain www.你的域名
#
# 前置条件（脚本不会替你改 DNS，也不检查它们）：
#   - 域名的 A/AAAA 解析指向这台服务器的公网 IP（dig +short 你的域名 可验证）
#   - 公网 80 放行（安全组/防火墙）；大陆服务器还要求域名已经备案
#   - 域名没有被 CDN/代理（如 Cloudflare 橙云）挡住 HTTP-01 校验
#
# 环境变量：
#   ACME_SH                acme.sh 路径，默认 ~/.acme.sh/acme.sh
#   BLOGX_NGINX_CONTAINER  nginx 容器名，默认 nginx
#   BLOGX_ACME_CA          CA 简名（acme.sh --server 支持的值），默认 letsencrypt
#   BLOGX_NO_INSTALL=1     跳过安装 acme.sh（已装好时用）
#
# 退出码：0 成功；2 参数/环境不满足；其它为 acme.sh / docker 的原始失败码。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CERT_DIR="$ROOT/deploy/state/nginx/cert"
NGINX_CONTAINER="${BLOGX_NGINX_CONTAINER:-nginx}"
CA="${BLOGX_ACME_CA:-letsencrypt}"
ACME_SH="${ACME_SH:-$HOME/.acme.sh/acme.sh}"

EMAIL=""
DOMAINS=()

usage() {
  cat <<'EOF'
用法：bash scripts/issue-cert.sh --email <邮箱> --domain <域名> [--domain <域名>...]

  在服务器上签发主站证书（Let's Encrypt / acme.sh standalone），
  把证书装到 deploy/state/nginx/cert/，后续续期交给 acme.sh 的 cron。

参数：
  -e, --email <邮箱>      ACME 账号邮箱（必填，用于到期提醒）
  -d, --domain <域名>     证书要覆盖的域名，可重复；第一个是主域名（必填）
  -h, --help              显示本帮助

环境变量：
  ACME_SH                acme.sh 路径，默认 ~/.acme.sh/acme.sh
  BLOGX_NGINX_CONTAINER  nginx 容器名，默认 nginx
  BLOGX_ACME_CA          CA 简名（acme.sh --server 支持的值），默认 letsencrypt
  BLOGX_NO_INSTALL=1     跳过安装 acme.sh（已装好时用）
EOF
}

log() { printf '==> %s\n' "$*"; }
die() { printf '错误：%s\n' "$*" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    -e|--email)
      [ $# -ge 2 ] || die "$1 缺少参数"
      EMAIL="$2"
      shift 2
      ;;
    -d|--domain)
      [ $# -ge 2 ] || die "$1 缺少参数"
      DOMAINS+=("$2")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "未知参数：$1（可用 --email / --domain / --help）"
      ;;
  esac
done

[ -n "$EMAIL" ] || die "缺少 --email"
case "$EMAIL" in
  *@*) ;;
  *) die "--email 看起来不是邮箱：$EMAIL" ;;
esac
[ "${#DOMAINS[@]}" -gt 0 ] || die "缺少 --domain"

command -v docker >/dev/null 2>&1 || die "找不到 docker：请用能执行 docker 的用户运行"
docker ps >/dev/null 2>&1 || die "docker 不可用（当前用户没有 docker 权限？）"

if [ ! -x "$ACME_SH" ] && [ "${BLOGX_NO_INSTALL:-0}" != "1" ]; then
  log "安装 acme.sh（会写当前用户的每日 cron）"
  curl -sSfL https://get.acme.sh | sh -s "email=$EMAIL" || die "acme.sh 安装失败"
fi
[ -x "$ACME_SH" ] || die "找不到 acme.sh：$ACME_SH（可用 ACME_SH 环境变量指定）"

# 续期时 acme.sh 会重新调用这两条钩子：先腾出 80，签完再把 nginx 拉起来。
# 首次执行时容器可能还不存在，所以失败必须被吞掉，不能中止签发。
HOOK_STOP="docker stop $NGINX_CONTAINER 2>/dev/null || true"
HOOK_START="docker start $NGINX_CONTAINER 2>/dev/null || true"

log "签发证书：${DOMAINS[*]}（CA：$CA；standalone 会临时停掉容器 $NGINX_CONTAINER）"
issue_args=(--issue --standalone --server "$CA")
for domain in "${DOMAINS[@]}"; do
  issue_args+=(-d "$domain")
done
issue_args+=(--pre-hook "$HOOK_STOP" --post-hook "$HOOK_START")
"$ACME_SH" "${issue_args[@]}"

log "证书装到 $CERT_DIR（续期成功后自动 reload nginx）"
mkdir -p "$CERT_DIR"
"$ACME_SH" --install-cert -d "${DOMAINS[0]}" \
  --key-file "$CERT_DIR/domain.key" \
  --fullchain-file "$CERT_DIR/domain.pem" \
  --reloadcmd "docker exec $NGINX_CONTAINER nginx -s reload 2>/dev/null || true"
chmod 600 "$CERT_DIR/domain.key"

if command -v openssl >/dev/null 2>&1; then
  log "证书信息："
  openssl x509 -in "$CERT_DIR/domain.pem" -noout -subject -issuer -dates || true
fi

log "完成。后续续期由 acme.sh 的 cron 负责（~/.acme.sh/acme.sh --list 看下次续期时间）。"
log "若这次是首次部署、nginx 还没起：docker compose up -d --build 之后 nginx 会直接加载证书。"
