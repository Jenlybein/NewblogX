#!/usr/bin/env bash
# 门禁自检（canary）：证明每一道门禁"真的会失败"。
#
# 为什么必须有这个脚本：
#   项目上一轮的治理之所以失效，是因为那套守卫检查的 5 个符号根本不存在，
#   于是 13 条规则全部永真通过。表面上"有守卫"，实际上没有任何东西被拦住。
#   判断这次是否又变成表演，唯一可靠的办法是：
#   **故意制造违规，看门禁是否会红。**
#
# 覆盖范围为什么要扩容（2026-10-26）：
#   原来只自检 4 道（后端单测 / archcheck 棘轮 / 规则自检 / 前端 typecheck），
#   而后续几轮陆续新增了 7 道门禁 —— 契约未过期、契约类型未过期、契约↔路由对账、
#   admin 鉴权探测、菜单↔页面对账、前端 lint、覆盖率门槛。
#   它们一道都没进过自检，等于"新增的门禁是否有效"从来没被验证过。
#   现在 14 节演练全覆盖：1..13 对应门禁清单（scripts/gates/gates.json）里的
#   13 道仓库级门禁；第 14 节是**清单外**的生成式装饰参数校验（#286）——
#   #287 AC4 只把「清单 / 版本 / 判定 / 重复安装」四件收进清单，参数生成脚本
#   不进清单，但它的演练照旧在这里，证明它会红。
#   第 5 道（契约未过期）与真门禁跨同一个判定实现
#   （scripts/check-contract-freshness.mjs），并真实注入「静默跳过但退出码 0」、
#   「命令失败」、「契约过期」与「工具版本写歪」四种形态，失败原因各自可区分。
#
# 唯一的例外是 admin 鉴权（第 8 道）：
#   要让它红，必须临时把某条后台路由的 AdminMiddleware 摘掉 —— 一旦脚本被中断
#   就会留下"后台无鉴权"的代码。而它本身已带两条对照断言
#   （TestProbeDistinguishesUnprotectedRoute / TestAdminOnlyListCoversContract），
#   破坏性注入的风险大于收益，所以只验证"能跑通"，不注入。
#
# 恢复策略：
#   所有临时改动都用**备份副本**恢复，不用 `git checkout --`。
#   原因：工作区经常有未提交改动，git checkout 会把人的活一起冲掉。
#   每节用完立即恢复；trap 只做兜底。
#
# 与门禁清单（#287）的关系：
#   门禁清单只有一个来源 —— scripts/gates/gates.json，经 scripts/gates/run.mjs 读。
#   本地门禁（scripts/check.sh）、持续集成（.github/workflows/ci.yml）、门禁自检
#   三处跨同一个入口；第 5 节的真门禁与四种注入、第 12/13 节的同源门禁都经
#   runner 调用，不再在自检里另拼命令。
#   开跑前先核对「清单里的每道门禁都登记了演练」：新门禁没演练就拒绝开跑，
#   避免自检"看起来覆盖了"而新门禁从未被验证。
#
# 用法：
#   ./scripts/verify-gates.sh                 # 全跑（默认，也是 CI 的口径）
#   ./scripts/verify-gates.sh --only 5,12     # 只跑受影响演练（改动范围可先用
#                                             #   node scripts/gates/run.mjs --affected <路径> 算）
# 退出码 0 表示"所有（或选中的）门禁都能正确报错"，非 0 表示某道门禁是假的。
#
# 「整体跳过」没有入口：--skip / --skip-all / BLOGX_SKIP_GATES 一律拒绝；
# --only 只接受清单里存在的演练节号，且必须至少选中一项（零项 = 失败）。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ── 不许整体跳过 + 按改动范围只跑受影响演练 ──────────────────────────
# 「全部跳过」在这里没有入口：--skip / --skip-all / BLOGX_SKIP_GATES 一律拒绝；
# --only 只接受门禁清单里存在的演练节号，且必须至少选中一项
# （零项 = 失败，不存在"什么都没跑就算通过"）。
ONLY_IDS=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --only)
      shift
      [ "$#" -gt 0 ] || { echo "✗ --only 需要演练节号（门禁自检不许整体跳过）" >&2; exit 2; }
      ONLY_IDS="$1"; shift ;;
    --only=*) ONLY_IDS="${1#--only=}"; shift ;;
    --skip*|--no-*)
      echo "✗ 门禁自检不许整体跳过：不接受 $1。要只跑一部分请用 --only <节号>，且至少选中一项。" >&2
      exit 2 ;;
    *)
      echo "✗ 未知参数 $1（门禁自检只接受 --only <节号>）" >&2
      exit 2 ;;
  esac
done
if [ -n "${BLOGX_SKIP_GATES:-}${BLOGX_SKIP_ALL_GATES:-}" ]; then
  echo "✗ 门禁自检不许整体跳过：检测到 BLOGX_SKIP_* 环境变量" >&2
  exit 2
fi
ONLY_IDS="$(printf '%s' "$ONLY_IDS" | tr ' ' ',')"

# canary_selected 决定本节要不要跑：默认全跑，--only 只跑选中的节。
canary_selected() {
  [ -z "$ONLY_IDS" ] && return 0
  case ",$ONLY_IDS," in
    *",$1,"*) return 0 ;;
  esac
  echo "  （--only 未选中 ${1}/14，跳过本节；这不是整体跳过）"
  return 1
}

for candidate in "$HOME/scoop/apps/go/current/bin" "$HOME/scoop/apps/go/1.27.3/bin"; do
  [ -x "$candidate/go.exe" ] && export PATH="$candidate:$PATH" && break
done
[ -x "$HOME/scoop/apps/mingw/current/bin/gcc.exe" ] && export PATH="$HOME/scoop/apps/mingw/current/bin:$PATH"
export GOTOOLCHAIN=local CGO_ENABLED=1

# 棘轮基线从唯一来源读：自检必须用"和真实门禁同一个阈值"，
# 否则基线降到 0 之后，这里还写死 250 就会注入 1 处违规却仍然通过，
# 自检反被判成"门禁失效"（2026-10-25 实际发生过）。
# shellcheck source=scripts/arch-baseline.env
. "$ROOT/scripts/arch-baseline.env"
# 生成工具版本单一来源：scripts/tool-versions.env。自检不再自己 source 它，
# 而是经 scripts/gates/run.mjs（真门禁）与 scripts/gates/install.mjs（安装步骤）
# 读同一份 —— 版本只有一处定义，判定与安装都不会在自检里长出第二份。

PASS=0
FAIL=0
BACKUPS=()
# 临时目录固定用 /tmp 而不是 mktemp：mktemp 在 Git Bash 上会返回带盘符的
# Windows 路径（C:/Users/...），备份文件的清理会被路径保护拦下，留下垃圾。
CANARY_TMP="/tmp/blogx-canary-$$"
rm -rf "$CANARY_TMP" 2>/dev/null
mkdir -p "$CANARY_TMP"

CANARY_TEST="backend/internal/canary/canary_test.go"
CANARY_ARCH="backend/api/zz_canary/enter.go"
CANARY_TS="frontend/app/utils/__canary.ts"
CANARY_LINT="frontend/app/utils/__canary_lint.ts"
CANARY_COV="frontend_admin/app/utils/__canary_cov.ts"
SWAGGER="backend/docs/swagger.json"

# backup_file 把文件复制到临时目录并登记，供 restore_file / trap 恢复。
backup_file() {
  local f="$1"
  local bak="$CANARY_TMP/$(echo "$f" | tr '/' '_')"
  cp "$f" "$bak" || return 1
  BACKUPS+=("$f|$bak")
}

# restore_file 立即恢复单个文件（每节用完就调，别等 trap）。
restore_file() {
  local f="$1" pair
  for pair in "${BACKUPS[@]:-}"; do
    [ -z "$pair" ] && continue
    if [ "${pair%%|*}" = "$f" ]; then
      cp "${pair#*|}" "$f"
      return 0
    fi
  done
  return 1
}

# restore_all 是兜底：正常流程下每节已自行恢复，这里只处理中途被打断的情况。
restore_all() {
  local pair
  for pair in "${BACKUPS[@]:-}"; do
    [ -z "$pair" ] && continue
    cp "${pair#*|}" "${pair%%|*}" 2>/dev/null
    rm -f "${pair#*|}"
  done
  rm -f "$CANARY_TEST" "$CANARY_ARCH" "$CANARY_TS" "$CANARY_LINT" "$CANARY_COV" 2>/dev/null
  rmdir backend/internal/canary 2>/dev/null
  rmdir backend/api/zz_canary 2>/dev/null
  rm -rf "$CANARY_TMP" 2>/dev/null
  return 0
}
trap restore_all EXIT

# ── 清单前置：门禁清单与演练清单必须对得上 ───────────────────────────
# 门禁清单只有一个来源：scripts/gates/gates.json（经 scripts/gates/run.mjs 读）。
# 加一道门禁只改那一处；若新门禁还没登记演练，自检直接拒绝开跑 ——
# 否则自检会"看起来覆盖了"，新门禁却从来没被验证过。
if ! node scripts/gates/run.mjs --check-coverage >"$CANARY_TMP/out.txt" 2>&1; then
  echo "✗ 门禁清单与演练清单对不上（新门禁必须先有演练）："
  tail -20 "$CANARY_TMP/out.txt"
  exit 2
fi
cat "$CANARY_TMP/out.txt"

# ── 演练节号与清单逐项一致 ───────────────────────────────────────────
# 节号从清单派生（run.mjs --sections），本脚本里的 `===== n/14` 标记必须与它
# 一一对应：清单加了门禁却没补演练节 → 那道门禁永远没被自检验过；脚本里多演
# 一节清单里没有的门禁 → 自检在演一个不存在的东西。两个方向都在这里拦下。
# 这一项不进 PASS/FAIL：它验的是"自检覆盖了哪些节"，不是"某道门禁会红"。
LIST_SECTIONS="$(node scripts/gates/run.mjs --sections)" || {
  echo "✗ 读不到门禁清单里的演练节号" >&2
  exit 2
}
SCRIPT_SECTIONS="$(grep -oE '===== [0-9]+/14' scripts/verify-gates.sh \
  | sed -E 's|^===== ([0-9]+)/14$|\1|' | sort -n | tr '\n' ' ' | sed 's/ $//')"
if [ "$LIST_SECTIONS" != "$SCRIPT_SECTIONS" ]; then
  echo "✗ 演练节号与门禁清单不一致（清单是唯一来源）：" >&2
  echo "  清单（run.mjs --sections）：$LIST_SECTIONS" >&2
  echo "  本脚本的 ===== n/14 标记：$SCRIPT_SECTIONS" >&2
  echo "  清单里新增门禁就补一节「===== n/14 ...」，否则那道门禁永远不会被自检验过。" >&2
  exit 2
fi
echo "  演练节号与清单一致：$LIST_SECTIONS（门禁清单 scripts/gates/gates.json）"

# --only 的节号必须来自清单，且至少选中一项（零项 = 失败，不是"跳过"）。
if [ -n "$ONLY_IDS" ]; then
  if ! node scripts/gates/run.mjs --list-canaries --json --only "$ONLY_IDS" \
      >"$CANARY_TMP/canaries.json" 2>"$CANARY_TMP/canaries.err"; then
    echo "✗ --only 的节号无效（门禁自检不许整体跳过）：" >&2
    cat "$CANARY_TMP/canaries.err" >&2
    exit 2
  fi
  echo "  只跑受影响演练：$(tr -d '\n ' < "$CANARY_TMP/canaries.json" | sed 's/},{/} {/g' | cut -c1-200)"
fi

ok() {
  echo "✓ 门禁有效：$1"
  PASS=$((PASS+1))
}

bad() {
  echo "✗ 门禁失效：$1"
  FAIL=$((FAIL+1))
}

expect_fail() {
  local name="$1"
  shift
  if "$@" >"$CANARY_TMP/out.txt" 2>&1; then
    bad "$name 没有失败"
  else
    ok "$name 正确失败"
  fi
}

# expect_pass 用于"自带对照断言"的门禁：它内部已有反例，只要整体能跑通即可。
expect_pass() {
  local name="$1"
  shift
  if "$@" >"$CANARY_TMP/out.txt" 2>&1; then
    ok "$name 通过（内部对照断言齐全）"
  else
    echo "✗ $name 失败"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
}

# contract_types_stale 返回 0 表示"已提交的 d.ts 与 swagger.json 不一致"。
# 与 scripts/check.sh 的 run_contract_types 判定口径一致：生成到临时文件再比对，
# 不绑定工作区是否干净（否则改完没提交就会一直红，人会习惯性忽略它）。
contract_types_stale() {
  local app="$1"
  local tmpf="$CANARY_TMP/openapi-types-$app.d.ts"
  if ! ( cd "$app" && corepack pnpm exec openapi-typescript \
        ../backend/docs/swagger.json -o "$tmpf" ) >"$CANARY_TMP/out.txt" 2>&1; then
    return 2
  fi
  if diff -q "$tmpf" "$app/app/types/openapi.d.ts" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

if canary_selected 1; then
echo "===== 1/14 后端单测门禁 ====="
mkdir -p backend/internal/canary
cat > "$CANARY_TEST" <<'EOF'
package canary

import "testing"

// 这个测试必须失败。如果 CI 在它存在时仍然通过，说明单测门禁是假的。
func TestCanaryMustFail(t *testing.T) {
	t.Fatal("canary: 这条断言必须失败")
}
EOF
expect_fail "backend 单测" bash -c 'cd backend && go test ./internal/canary/'
rm -f "$CANARY_TEST"; rmdir backend/internal/canary 2>/dev/null

fi
echo ""
if canary_selected 2; then
echo "===== 2/14 架构规则门禁（archcheck 棘轮） ====="
mkdir -p backend/api/zz_canary
cat > "$CANARY_ARCH" <<'EOF'
package zz_canary

import "gorm.io/gorm"

// 故意违规：api 层持有 *gorm.DB，并定义一个 Deps 结构体。
// 这两件事都已被 archcheck 的 R1 / R2 禁止，门禁必须让它失败。
type Deps struct {
	DB *gorm.DB
}
EOF
# 注入 1 处违规后总数 = 基线 + 1，用当前基线做阈值必然超限。
# 这里必须引用变量而不是写死数字：写死就会在基线下调后失效。
expect_fail "archcheck 棘轮（新增违规即超阈值，基线 $ARCH_MAX_VIOLATIONS）" \
  bash -c "cd backend && go run ./tools/archcheck --mode=error --max-violations=$ARCH_MAX_VIOLATIONS ."
rm -f "$CANARY_ARCH"; rmdir backend/api/zz_canary 2>/dev/null

fi
echo ""
if canary_selected 3; then
echo "===== 3/14 archcheck 规则自检 ====="
# 规则自检本身也是门禁：如果把某条规则改坏（不再报错），rules_test 必须红。
expect_pass "archcheck 规则自检" bash -c 'cd backend && go test ./tools/archcheck/...'

fi
echo ""
if canary_selected 4; then
echo "===== 4/14 前端类型门禁 ====="
cat > "$CANARY_TS" <<'EOF'
// 故意制造类型错误：把一个字符串赋给 number。
// typecheck 必须失败；如果通过，说明类型门禁是假的。
export const canaryNumber: number = "not a number";
EOF
# 注意：tsc 只看被引用的文件，所以用 vue-tsc 全量检查时也会覆盖 app/ 下的文件。
expect_fail "前端 typecheck" bash -c 'cd frontend && NODE_OPTIONS="--max-old-space-size=8192" corepack pnpm typecheck'
rm -f "$CANARY_TS"

fi
echo ""
if canary_selected 5; then
echo "===== 5/14 契约未过期（注解 → swagger.json） ====="
# 门禁原型：CI 的 contract job / check.sh 的「契约未过期」都调用
# scripts/check-contract-freshness.mjs。自检必须跨**同一个判定接口**，
# 而不是在 YAML / 本脚本里另写一份判据。
#
# 五种断言各注入一次，红的原因必须可区分：
#   a) 真门禁本身能跑通（否则下面的红可能来自 node/make 缺失，是假红）；
#   b) 生成器静默跳过但退出码 0 → 必须报 SILENT_SKIP（假绿 / 门禁撒谎）；
#   c) 生成命令失败          → 必须报 COMMAND_FAILED；
#   d) 注解改了但契约没重新生成 → 必须报 STALE（判据抓得住"过期"）；
#   e) 工具版本写歪           → 必须报 TOOL_VERSION_MISMATCH。
# 注入走环境变量 SWAG 覆盖 Makefile 的生成器命令，`make openapi` 本身仍是
# 真门禁那条命令；e 用 --versions-file 指到临时文件，不改仓库里的版本来源。
# 除 d 临时改 main.go 外都不碰仓库文件，且每节用完立即从备份副本恢复。
FRESHNESS_GATE="contract-freshness"
# freshness 调「真门禁」本身：清单里的 contract-freshness 门禁 → 判定实现
# scripts/check-contract-freshness.mjs。自检与本地门禁 / CI 走同一个入口，
# 因此下面五种注入验的就是那条真链路，而不是另写一份判据。
# 额外参数原样透传（`--` 之后）给同一份判定实现。
freshness() { node scripts/gates/run.mjs "$FRESHNESS_GATE" "$@"; }
SWAG_SKIP="$CANARY_TMP/swag-silent-skip.sh"
SWAG_FAIL="$CANARY_TMP/swag-command-fail.sh"
printf '#!/bin/sh\n# 生成器静默跳过：退出码 0，但不写产物\nexit 0\n' > "$SWAG_SKIP"
printf '#!/bin/sh\necho "canary: 模拟生成命令失败" >&2\nexit 17\n' > "$SWAG_FAIL"
chmod +x "$SWAG_SKIP" "$SWAG_FAIL"

# reason_of 读取判定输出的机器可判原因标记；判定失败时仍有 stdout。
reason_of() { grep -oE '\[reason\] [A-Z_]+' "$CANARY_TMP/out.txt" | head -1 | awk '{print $2}'; }

# a) 真门禁跑通（用同一份判定实现）
if freshness >"$CANARY_TMP/out.txt" 2>&1; then
  ok "契约未过期判定（真门禁跑通）"
else
  echo "✗ 契约未过期判定本身跑不通（node / swag / make 缺失？），后续注入会假红"
  tail -5 "$CANARY_TMP/out.txt"
  FAIL=$((FAIL+1))
fi

# b) 静默跳过但退出码 0 —— 真实触发「门禁撒谎」形态
if SWAG="sh $SWAG_SKIP" freshness >"$CANARY_TMP/out.txt" 2>&1; then
  bad "生成器静默跳过但退出码 0，判定却通过了（假绿）"
elif [ "$(reason_of)" = "SILENT_SKIP" ]; then
  ok "生成器静默跳过但退出码 0 会判红（SILENT_SKIP）"
else
  echo "✗ 静默跳过的失败原因不是 SILENT_SKIP：$(reason_of)"
  tail -5 "$CANARY_TMP/out.txt"
  FAIL=$((FAIL+1))
fi

# c) 命令失败 —— 必须与静默跳过报出**不同**原因
if SWAG="sh $SWAG_FAIL" freshness >"$CANARY_TMP/out.txt" 2>&1; then
  bad "生成命令失败，判定却通过了"
elif [ "$(reason_of)" = "COMMAND_FAILED" ]; then
  ok "生成命令失败会判红（COMMAND_FAILED，与静默跳过原因可区分）"
else
  echo "✗ 命令失败的失败原因不是 COMMAND_FAILED：$(reason_of)"
  tail -5 "$CANARY_TMP/out.txt"
  FAIL=$((FAIL+1))
fi

# d) 注解改了但产物没重新生成 —— 判据必须抓得住"契约过期"。
# 这里把 @title 改掉后仍调用同一份判定：生成器会产出与已提交契约不同的内容。
MAIN_GO="backend/cmd/server/main.go"
if backup_file "$MAIN_GO"; then
  sed -i 's|^// @title BlogX API$|// @title BlogX API CANARY|' "$MAIN_GO"
  if ! grep -q "@title BlogX API CANARY" "$MAIN_GO"; then
    echo "✗ 无法注入：main.go 里没有预期的 @title 行"
    FAIL=$((FAIL+1))
  elif freshness >"$CANARY_TMP/out.txt" 2>&1; then
    bad "注解改了、契约没重新生成，判定却通过了"
  elif [ "$(reason_of)" = "STALE" ]; then
    ok "注解变更但契约未重新生成会判红（STALE）"
  else
    echo "✗ 契约过期的失败原因不是 STALE：$(reason_of)"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$MAIN_GO"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

# e) 工具版本单一来源：把写歪的安装 ref 交给同一份判定，必须报版本不一致。
# 用 --versions-file 指到临时副本，不动仓库里的那份单一来源。
VERSIONS="scripts/tool-versions.env"
TAMPERED_VERSIONS="$CANARY_TMP/tool-versions-canary.env"
if cp "$VERSIONS" "$TAMPERED_VERSIONS" && grep -q '^SWAG_VERSION=' "$TAMPERED_VERSIONS"; then
  sed -i 's|^SWAG_VERSION=.*$|SWAG_VERSION=v9.9.9-canary|' "$TAMPERED_VERSIONS"
  if ! grep -q '^SWAG_VERSION=v9.9.9-canary$' "$TAMPERED_VERSIONS"; then
    echo "✗ 无法写歪版本文件（单一来源格式变了？）"
    FAIL=$((FAIL+1))
  elif freshness -- --versions-file "$TAMPERED_VERSIONS" >"$CANARY_TMP/out.txt" 2>&1; then
    bad "写歪工具版本，判定却通过了"
  elif [ "$(reason_of)" = "TOOL_VERSION_MISMATCH" ]; then
    ok "写歪工具版本会判红（TOOL_VERSION_MISMATCH，单一来源生效）"
  else
    echo "✗ 写歪版本的原因不是 TOOL_VERSION_MISMATCH：$(reason_of)"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
else
  echo "✗ 无法复制版本文件或 SWAG_VERSION 未从单一来源读出，跳过版本演练"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 6; then
echo "===== 6/14 前端契约类型未过期 ====="
# 门禁原型（CI 的 contract-types job / check.sh 的 run_contract_types）：
# swagger.json 更新了但 openapi.d.ts 忘了重新生成 → 前端按旧类型编译。
# 注入：往已提交的 d.ts 里加一行，模拟"与契约不一致"。
DTS="frontend/app/types/openapi.d.ts"
if backup_file "$DTS"; then
  printf '\n// canary: 故意与 swagger.json 不一致\n' >> "$DTS"
  contract_types_stale frontend
  case $? in
    0) ok "d.ts 与契约不一致时会被判为过期" ;;
    1) bad "改了 d.ts 却仍判定为一致（类型门禁是假的）" ;;
    *) echo "✗ 生成契约类型失败（node/pnpm 依赖缺失）"; FAIL=$((FAIL+1)) ;;
  esac
  restore_file "$DTS"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 7; then
echo "===== 7/14 契约 ↔ 路由表对账 ====="
# 门禁原型：backend/internal/platform/http/router/contract_routes_test.go
# 它双向对账：契约里有、路由里没有 = 契约说谎（严格失败）；
# 路由里有、契约里没有 = 缺口（必须登记并写理由）。
# 注入：往契约里塞一个不存在的端点，测试必须报"phantom"。
if backup_file "$SWAGGER"; then
  if node -e '
    const fs = require("fs");
    const p = "backend/docs/swagger.json";
    const d = JSON.parse(fs.readFileSync(p, "utf8"));
    d.paths["/canary/phantom"] = {
      get: { summary: "canary", responses: { "200": { description: "ok" } } },
    };
    fs.writeFileSync(p, JSON.stringify(d, null, 4));
  ' 2>"$CANARY_TMP/out.txt"; then
    if ( cd backend && go test ./internal/platform/http/router/ \
          -run '^TestContractMatchesRoutes$' -count=1 ) >"$CANARY_TMP/out.txt" 2>&1; then
      bad "契约里多了一个虚构端点，对账却没报错"
    else
      ok "契约与路由表不一致时会被抓出"
    fi
  else
    echo "✗ 注入契约失败（node 不可用或 swagger.json 结构变了）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$SWAGGER"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 8; then
echo "===== 8/14 admin 鉴权门禁（不注入，理由见文件头） ====="
# 这三条用例自带反例：
#   TestAdminRoutesRejectOrdinaryUser       普通用户令牌探 16 个后台操作，必须停在中间件
#   TestProbeDistinguishesUnprotectedRoute  对照用例：放行与拦住必须能分辨
#   TestAdminOnlyListCoversContract         契约里新增的后台接口不许逃过探测
expect_pass "admin 鉴权探测 + 契约覆盖面" \
  bash -c "cd backend && go test ./internal/platform/http/router/ -run '^(TestAdminRoutesRejectOrdinaryUser|TestProbeDistinguishesUnprotectedRoute|TestAdminOnlyListCoversContract)\$' -count=1"

fi
echo ""
if canary_selected 9; then
echo "===== 9/14 菜单 ↔ 页面文件对账 ====="
# 门禁原型：frontend_admin/test/unit/menu.test.ts
# 起因：侧边栏挂着「日志中心 → /logs」而 app/pages/logs/ 不存在，点进去 404。
# 注入：往 ADMIN_NAV 里加一个没有对应页面文件的 path。
MENU="frontend_admin/app/utils/menu.ts"
if backup_file "$MENU"; then
  sed -i "s|{ label: '数据概览', path: '/', icon: 'chart' },|{ label: '数据概览', path: '/', icon: 'chart' },\n  { label: 'CANARY', path: '/canary-missing-page', icon: 'chart' },|" "$MENU"
  if ! grep -q "canary-missing-page" "$MENU"; then
    echo "✗ 无法注入：menu.ts 里没找到预期的 ADMIN_NAV 首项"
    FAIL=$((FAIL+1))
  elif ( cd frontend_admin && corepack pnpm exec vitest run test/unit/menu.test.ts ) \
        >"$CANARY_TMP/out.txt" 2>&1; then
    bad "菜单指向不存在的页面，对账却没报错"
  else
    ok "菜单里的死链会被抓出"
  fi
  restore_file "$MENU"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 10; then
echo "===== 10/14 前端 lint 门禁 ====="
# 注入一个 debugger：两端 eslint 都把 no-debugger 设为 error。
printf 'export function canaryLint(): void {\n  debugger\n}\n' > "$CANARY_LINT"
if ( cd frontend && corepack pnpm lint ) >"$CANARY_TMP/out.txt" 2>&1; then
  bad "debugger 竟然通过了 lint"
else
  ok "违反规则的代码会被 lint 拦下"
fi
rm -f "$CANARY_LINT"

fi
echo ""
if canary_selected 11; then
echo "===== 11/14 覆盖率门槛 ====="
# 门禁原型：frontend_admin 的 vitest coverage thresholds（95/90/95/95，
# 统计范围只有 app/utils/**）。
#
# 注入方式曾有一个**过期的前提假设**：「现有 6 个文件都是 100%，加一个 0 覆盖
# 文件会把整体拉到 85.7%，必然失败」。实际 admin/app/utils 已达 12 个文件、
# 语句覆盖 99.2%，注入一个 3 条语句的零覆盖文件只掉到 98.08% —— 远够不到
# 95 门槛，于是自检把「门禁失效」判了出来（跑 check.sh 才暴露，CI 因为命令
# 因别的原因返回非零反而"假绿"过）。这里改成与分母无关的注入：
#   1) 仍然注入零覆盖文件（保留原本最真实的场景）；
#   2) 同时把四道阈值临时抬到 100 —— 当前覆盖率远低于 100，必然判红，
#      分母怎么长都不会让注入失效；
#   3) 核对失败原因确实是"覆盖率不达阈值"，而不是 pnpm/node 不可用后
#      因别的原因返回非零（后者是典型的假红，等于门禁不存在）。
ADMIN_VITEST_CFG="frontend_admin/vitest.config.ts"
if backup_file "$ADMIN_VITEST_CFG"; then
  printf 'export function canaryCov(flag: boolean): number {\n  if (flag) {\n    return 1\n  }\n  return 2\n}\n' > "$CANARY_COV"
  sed -i \
    -e 's/^\( *\)statements: 95,$/\1statements: 100,/' \
    -e 's/^\( *\)branches: 90,$/\1branches: 100,/' \
    -e 's/^\( *\)functions: 95,$/\1functions: 100,/' \
    -e 's/^\( *\)lines: 95,$/\1lines: 100,/' \
    "$ADMIN_VITEST_CFG"
  if ( cd frontend_admin && corepack pnpm test:coverage ) >"$CANARY_TMP/out.txt" 2>&1; then
    bad "覆盖率低于阈值时门槛没有拦住"
  elif grep -Eqi "does not meet|threshold|Coverage for" "$CANARY_TMP/out.txt"; then
    ok "覆盖率跌破阈值会让门禁失败"
  else
    echo "✗ 覆盖率门禁没有按预期原因失败（pnpm/node 不可用？）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  rm -f "$CANARY_COV"
  restore_file "$ADMIN_VITEST_CFG"
else
  bad "无法备份 $ADMIN_VITEST_CFG，覆盖率自检无法进行"
fi

fi
echo ""
if canary_selected 12; then
echo "===== 12/14 两端 tokens.css 同源门禁 ====="
# 门禁原型：scripts/check-token-parity.mjs（ADR-0012 约束 14 / #22）
# 判据是"两份文件逐字节相同"。注入：给后台那份末尾追加一个注释 —— 模拟最真实的
# 事故：只改了读者站（或只改了后台），另一端忘记同步。门禁必须变红。
ADMIN_TOKENS="frontend_admin/app/assets/css/tokens.css"
if backup_file "$ADMIN_TOKENS"; then
  printf '\n/* canary: 故意只改一端 */\n' >> "$ADMIN_TOKENS"
  if node scripts/gates/run.mjs token-parity >"$CANARY_TMP/out.txt" 2>&1; then
    bad "两端 tokens.css 不一致时，同源门禁没有失败"
  elif grep -q "两端 token 同源门禁失败" "$CANARY_TMP/out.txt"; then
    # 必须确认"红"的原因是内容不一致，而不是 node 没被找到：
    # 后者也会返回非零，属于典型的"门禁假红/假绿"混淆。
    ok "只改一端 token 会被判为不一致"
  else
    echo "✗ 同源门禁没有按预期原因失败（node 不可用？）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$ADMIN_TOKENS"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 13; then
echo "===== 13/14 两端同源副本门禁（定形工具 + 裁剪窗四份组件） ====="
# 门禁原型：scripts/check-image-shaping-parity.mjs（#200 定形工具；#225 扩到裁剪窗）
# 判据是"两端副本逐字节相同"。后台不能 import 读者站模块（#176 会让生产构建失败），
# 所以封面定形与裁剪窗四份组件只能靠同一份文件 + 这道门禁。
# 注入 1：把**后台副本**里的封面定形尺寸 1600 改成 1500 —— 模拟最真实的漂移：
# 只改一端（这里是只改了后台），另一端忘记同步。门禁必须红，并报出漂移方向。
ADMIN_SHAPING="frontend_admin/app/utils/imageShaping.ts"
if backup_file "$ADMIN_SHAPING"; then
  sed -i 's|{ width: 1600, height: 900 }|{ width: 1500, height: 900 }|' "$ADMIN_SHAPING"
  if ! grep -q "width: 1500, height: 900" "$ADMIN_SHAPING"; then
    echo "✗ 无法注入：imageShaping.ts 里没找到预期的封面目标尺寸常量"
    FAIL=$((FAIL+1))
  elif node scripts/gates/run.mjs image-shaping-parity >"$CANARY_TMP/out.txt" 2>&1; then
    bad "两端定形工具不一致时，同源门禁没有失败"
  elif grep -q "两端定形工具（imageShaping）同源门禁失败" "$CANARY_TMP/out.txt"; then
    # 必须确认"红"的原因是内容不一致（且报了哪一份漂移），
    # 而不是 node 没被找到 —— 后者也返回非零，属于典型的"门禁假红/假绿"混淆。
    if grep -q "漂移方向" "$CANARY_TMP/out.txt"; then
      ok "只改后台副本一个常量会被判为不一致（并报出漂移方向）"
    else
      echo "✗ 门禁红了但没报出哪一份漂移（报错信息退化了）"
      tail -5 "$CANARY_TMP/out.txt"
      FAIL=$((FAIL+1))
    fi
  else
    echo "✗ 同源门禁没有按预期原因失败（node 不可用？）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$ADMIN_SHAPING"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

# 注入 2（#225）：只改后台那份 ImageCropDialog 的一个属性值 —— 裁剪窗四份组件
# 此前没有任何门禁，改成同源判据后同样必须红，并点名是哪一对漂移。
ADMIN_CROP_DIALOG="frontend_admin/app/components/media/ImageCropDialog.vue"
if backup_file "$ADMIN_CROP_DIALOG"; then
  sed -i 's|preset="card"|preset="dial"|' "$ADMIN_CROP_DIALOG"
  if ! grep -q 'preset="dial"' "$ADMIN_CROP_DIALOG"; then
    echo "✗ 无法注入：ImageCropDialog.vue 里没找到预期的 NModal preset"
    FAIL=$((FAIL+1))
  elif node scripts/gates/run.mjs image-shaping-parity >"$CANARY_TMP/out.txt" 2>&1; then
    bad "两端裁剪窗组件不一致时，同源门禁没有失败"
  elif grep -q "两端裁剪窗（ImageCropView / ImageCropDialog）同源门禁失败" "$CANARY_TMP/out.txt"; then
    if grep -q "漂移方向" "$CANARY_TMP/out.txt"; then
      ok "只改后台那份裁剪窗组件会被判为不一致（并报出漂移方向）"
    else
      echo "✗ 门禁红了但没报出哪一份漂移（报错信息退化了）"
      tail -5 "$CANARY_TMP/out.txt"
      FAIL=$((FAIL+1))
    fi
  else
    echo "✗ 裁剪窗同源门禁没有按预期原因失败（node 不可用？）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$ADMIN_CROP_DIALOG"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
if canary_selected 14; then
echo "===== 14/14 生成式装饰参数校验 ====="
# 门禁原型：scripts/generate-deco-parameters.mjs --check（#286）。
# 它按 #287 AC4 不进清单（清单只收清单 / 版本 / 判定 / 重复安装四件），
# 真门禁由 scripts/check.sh 的 gates 段与 ci.yml 的 frontend-lint 直接执行。
# 真门禁先证明四端消费物与同一参数表一致，再验契约与 /site/deco 运行时取值；
# 自检这一节直接调同一个脚本 —— 是否在清单里不改变"它会红"这件事要不要被证明。
# 注入：只改后台生成物一个上限，让「重新生成后四端无差异」这一条变红；
# 恢复后真门禁再跑通，排除 node/go 不可用造成的假红。
ADMIN_DECO="frontend_admin/app/utils/decoParams.ts"
DECO_CHECK="node scripts/generate-deco-parameters.mjs --check"
if $DECO_CHECK >"$CANARY_TMP/out.txt" 2>&1; then
  ok "生成式装饰参数真门禁跑通"
else
  echo "✗ 生成式装饰参数真门禁未跑通"
  tail -8 "$CANARY_TMP/out.txt"
  FAIL=$((FAIL+1))
fi
if backup_file "$ADMIN_DECO"; then
  sed -i 's|^export const DECO_ROPE_LENGTH_MAX = 240$|export const DECO_ROPE_LENGTH_MAX = 260|' "$ADMIN_DECO"
  if ! grep -q "DECO_ROPE_LENGTH_MAX = 260" "$ADMIN_DECO"; then
    echo "✗ 无法注入：decoParams.ts 里没找到预期的生成常量"
    FAIL=$((FAIL+1))
  elif $DECO_CHECK >"$CANARY_TMP/out.txt" 2>&1; then
    bad "只改后台生成物一端，生成式参数校验没有失败"
  elif grep -q "重新生成后四端有差异" "$CANARY_TMP/out.txt"; then
    ok "只改后台生成物一端会被判为重新生成后有差异"
  else
    echo "✗ 生成式参数校验没有按预期原因失败（node/go 不可用？）"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$ADMIN_DECO"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

fi
echo ""
echo "─────────────────────────────"
echo "通过：$PASS  失败：$FAIL"
if [ -n "$ONLY_IDS" ]; then
  echo "  范围：--only $ONLY_IDS（只验证了受影响的演练；其余节要靠全量运行覆盖，不存在整体跳过路径）"
fi
if [ "$FAIL" -ne 0 ]; then
  echo "✗ 存在失效门禁，M1 不能验收"
  echo "  如果怀疑有文件没恢复干净，用 git status 复查（本脚本不会动已提交内容）"
  exit 1
fi
if [ -n "$ONLY_IDS" ]; then
  echo "✓ 选中的演练均能在违规时报错（--only $ONLY_IDS）"
else
  echo "✓ 全部门禁均能在违规时报错"
fi
