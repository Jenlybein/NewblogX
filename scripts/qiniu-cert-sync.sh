#!/usr/bin/env bash
# 把 acme.sh 刚签好 / 续好的证书推给七牛，并绑定到 CDN 域名。
#
# 谁调用它：acme.sh 的 --reloadcmd —— 首次 --install-cert 时执行一次，
# 之后每次续期成功都会再执行。也可以手动跑。
#
# 用法（凭据从环境变量读，不进仓库）：
#   set -a; . /etc/blogx/qiniu/env; bash /opt/blogx/scripts/qiniu-cert-sync.sh
#
# 必需的环境变量：
#   QINIU_AK     七牛 AccessKey
#   QINIU_SK     七牛 SecretKey
#   CDN_DOMAIN   要绑证书的 CDN 域名（如 image.example.com）
#   KEY_FILE     私钥路径（acme.sh --key-file 落盘的位置）
#   CHAIN_FILE   证书链路径（acme.sh --fullchain-file 落盘的位置）
# 可选：
#   CERT_NAME    七牛控制台里的证书名，默认用 CDN_DOMAIN
#
# 前置：curl、openssl、python3（七牛接口要求 JSON，PEM 换行需要 python3 转义）。
# 退出码：0 成功；2 环境/参数不满足；1 七牛接口失败。
set -euo pipefail

usage() {
  cat <<'EOF'
用法：qiniu-cert-sync.sh

  把 acme.sh 落盘的证书上传到七牛，并 sslize 绑定到 CDN 域名。
  凭据从环境变量读（先 `set -a; . /etc/blogx/qiniu/env`）：

  QINIU_AK / QINIU_SK   七牛密钥
  CDN_DOMAIN            CDN 域名
  KEY_FILE              私钥路径
  CHAIN_FILE            证书链路径
  CERT_NAME             可选，七牛证书名（默认 CDN_DOMAIN）

  通常由 acme.sh --reloadcmd 调用，不需要手动执行。
EOF
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  "") ;;
  *) printf '错误：未知参数 %s\n' "$1" >&2; exit 2 ;;
esac

missing=""
for name in QINIU_AK QINIU_SK CDN_DOMAIN KEY_FILE CHAIN_FILE; do
  value="${!name:-}"
  [ -n "$value" ] || missing="$missing $name"
done
if [ -n "$missing" ]; then
  printf '错误：缺少环境变量%s（先 `set -a; . /etc/blogx/qiniu/env`）\n' "$missing" >&2
  exit 2
fi

for tool in curl openssl python3; do
  command -v "$tool" >/dev/null 2>&1 || { printf '错误：找不到 %s\n' "$tool" >&2; exit 2; }
done
for file in "$KEY_FILE" "$CHAIN_FILE"; do
  [ -f "$file" ] || { printf '错误：找不到证书文件 %s\n' "$file" >&2; exit 2; }
done

NAME="${CERT_NAME:-$CDN_DOMAIN}"

# 七牛既不用 OAuth 也不用 AWS 签名：ACL 用 HMAC-SHA1 后做 base64url。
b64url() { openssl base64 -A | tr '+/' '-_'; }
hmac()   { openssl dgst -sha1 -hmac "$QINIU_SK" -binary | b64url; }

# ── 1) 上传证书（QBox 鉴权：JSON body 不进签名，只签路径） ──
printf '==> 上传证书 %s（%s）\n' "$NAME" "$CDN_DOMAIN"
PAYLOAD=$(python3 -c 'import json,sys
print(json.dumps({"name":sys.argv[1],"pri":open(sys.argv[2]).read(),"ca":open(sys.argv[3]).read()}))' \
  "$NAME" "$KEY_FILE" "$CHAIN_FILE")
RESP=$(curl -sS -X POST https://fusion.qiniuapi.com/sslcert \
  -H "Authorization: QBox $QINIU_AK:$(printf '/sslcert\n' | hmac)" \
  -H 'Content-Type: application/json' --data-binary "$PAYLOAD")
CERT_ID=$(printf '%s' "$RESP" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("certID",""))' 2>/dev/null || true)
if [ -z "$CERT_ID" ]; then
  printf '错误：上传证书失败，七牛响应：%s\n' "$RESP" >&2
  exit 1
fi

# ── 2) 绑定到 CDN 域名（Qiniu 鉴权：body 必须与签名时字节一致） ──
printf '==> 绑定证书 %s 到 %s\n' "$CERT_ID" "$CDN_DOMAIN"
DATE=$(date -u +%Y%m%dT%H%M%SZ)
BODY=$(printf '{"certId":"%s","forceHttps":true,"http2Enable":true}' "$CERT_ID")
SIGN=$( { printf 'PUT /domain/%s/sslize\n' "$CDN_DOMAIN"
          printf 'Host: api.qiniu.com\n'
          printf 'Content-Type: application/json\n'
          printf 'X-Qiniu-Date: %s\n\n' "$DATE"
          printf '%s' "$BODY"; } | hmac )
BIND=$(curl -sS -X PUT "https://api.qiniu.com/domain/$CDN_DOMAIN/sslize" \
  -H "Authorization: Qiniu $QINIU_AK:$SIGN" \
  -H "X-Qiniu-Date: $DATE" \
  -H 'Content-Type: application/json' \
  --data-binary "$BODY" -w '\nHTTP_STATUS=%{http_code}')
STATUS=$(printf '%s' "$BIND" | tail -n1)
BODY_TEXT=$(printf '%s\n' "$BIND" | sed '$d')
case "$STATUS" in
  *HTTP_STATUS=200*) ;;
  *)
    printf '错误：sslize 失败（%s）%s\n' "$STATUS" "$BODY_TEXT" >&2
    exit 1
    ;;
esac
printf 'sslize 返回：%s %s\n' "$STATUS" "$BODY_TEXT"

# ── 3) 校验（CDN 配置 5–10 分钟生效，首次推送后立刻访问可能还没生效） ──
printf '==> 校验 https://%s/\n' "$CDN_DOMAIN"
if curl -sSI --max-time 10 "https://$CDN_DOMAIN/" | head -1; then
  :
else
  printf '提示：CDN 生效需要 5–10 分钟；若仍不可用，检查证书 SAN 是否覆盖 %s。\n' "$CDN_DOMAIN" >&2
fi

printf '完成：已把证书 %s 绑定到 %s（certId=%s）\n' "$NAME" "$CDN_DOMAIN" "$CERT_ID"
