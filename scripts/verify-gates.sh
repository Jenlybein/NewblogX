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
#   现在 12 道全覆盖。
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
# 用法：./scripts/verify-gates.sh
# 退出码 0 表示"所有门禁都能正确报错"，非 0 表示某道门禁是假的。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

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

echo "===== 1/12 后端单测门禁 ====="
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

echo ""
echo "===== 2/12 架构规则门禁（archcheck 棘轮） ====="
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

echo ""
echo "===== 3/12 archcheck 规则自检 ====="
# 规则自检本身也是门禁：如果把某条规则改坏（不再报错），rules_test 必须红。
expect_pass "archcheck 规则自检" bash -c 'cd backend && go test ./tools/archcheck/...'

echo ""
echo "===== 4/12 前端类型门禁 ====="
cat > "$CANARY_TS" <<'EOF'
// 故意制造类型错误：把一个字符串赋给 number。
// typecheck 必须失败；如果通过，说明类型门禁是假的。
export const canaryNumber: number = "not a number";
EOF
# 注意：tsc 只看被引用的文件，所以用 vue-tsc 全量检查时也会覆盖 app/ 下的文件。
expect_fail "前端 typecheck" bash -c 'cd frontend && NODE_OPTIONS="--max-old-space-size=8192" corepack pnpm typecheck'
rm -f "$CANARY_TS"

echo ""
echo "===== 5/12 契约未过期（注解 → swagger.json） ====="
# 门禁原型（CI 的 contract job）：改注解后忘了 `make openapi`，
# `git diff --exit-code docs/swagger.json` 就会非零。
#
# 这里不能用 git diff 判定：工作区常有未提交改动，那样会假红。
# 改成等价判据：**改一条注解 → 重新生成 → 结果必须与生成前不同**。
# 若相同，说明"注解变化传不到契约"，那道门禁就抓不到过期。
MAIN_GO="backend/cmd/server/main.go"
if backup_file "$SWAGGER" && backup_file "$MAIN_GO"; then
  cp "$SWAGGER" "$CANARY_TMP/swagger-before.json"
  sed -i 's|^// @title BlogX API$|// @title BlogX API CANARY|' "$MAIN_GO"
  if ! grep -q "@title BlogX API CANARY" "$MAIN_GO"; then
    echo "✗ 无法注入：main.go 里没有预期的 @title 行"
    FAIL=$((FAIL+1))
  elif ( cd backend && make openapi ) >"$CANARY_TMP/out.txt" 2>&1; then
    if cmp -s "$CANARY_TMP/swagger-before.json" "$SWAGGER"; then
      bad "注解改了，契约却没变化（contract job 会漏掉过期）"
    else
      ok "注解变更会改变契约内容"
    fi
  else
    echo "✗ 契约生成失败（swag 缺失或注解有误），无法判定"
    tail -5 "$CANARY_TMP/out.txt"
    FAIL=$((FAIL+1))
  fi
  restore_file "$SWAGGER"
  restore_file "$MAIN_GO"
else
  echo "✗ 备份失败，跳过本节"
  FAIL=$((FAIL+1))
fi

echo ""
echo "===== 6/12 前端契约类型未过期 ====="
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

echo ""
echo "===== 7/12 契约 ↔ 路由表对账 ====="
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

echo ""
echo "===== 8/12 admin 鉴权门禁（不注入，理由见文件头） ====="
# 这三条用例自带反例：
#   TestAdminRoutesRejectOrdinaryUser       普通用户令牌探 16 个后台操作，必须停在中间件
#   TestProbeDistinguishesUnprotectedRoute  对照用例：放行与拦住必须能分辨
#   TestAdminOnlyListCoversContract         契约里新增的后台接口不许逃过探测
expect_pass "admin 鉴权探测 + 契约覆盖面" \
  bash -c "cd backend && go test ./internal/platform/http/router/ -run '^(TestAdminRoutesRejectOrdinaryUser|TestProbeDistinguishesUnprotectedRoute|TestAdminOnlyListCoversContract)\$' -count=1"

echo ""
echo "===== 9/12 菜单 ↔ 页面文件对账 ====="
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

echo ""
echo "===== 10/12 前端 lint 门禁 ====="
# 注入一个 debugger：两端 eslint 都把 no-debugger 设为 error。
printf 'export function canaryLint(): void {\n  debugger\n}\n' > "$CANARY_LINT"
if ( cd frontend && corepack pnpm lint ) >"$CANARY_TMP/out.txt" 2>&1; then
  bad "debugger 竟然通过了 lint"
else
  ok "违反规则的代码会被 lint 拦下"
fi
rm -f "$CANARY_LINT"

echo ""
echo "===== 11/12 覆盖率门槛 ====="
# 门禁原型：frontend_admin 的 vitest coverage thresholds（95/90/95/95，
# 统计范围只有 app/utils/**）。现有 6 个文件都是 100%，加一个 0 覆盖文件
# 会把整体拉到 85.7%，必须让 `test:coverage` 失败。
printf 'export function canaryCov(flag: boolean): number {\n  if (flag) {\n    return 1\n  }\n  return 2\n}\n' > "$CANARY_COV"
if ( cd frontend_admin && corepack pnpm test:coverage ) >"$CANARY_TMP/out.txt" 2>&1; then
  bad "新增零覆盖文件后覆盖率门槛没有拦住"
else
  ok "覆盖率跌破门槛会让门禁失败"
fi
rm -f "$CANARY_COV"

echo ""
echo "===== 12/12 两端 tokens.css 同源门禁 ====="
# 门禁原型：scripts/check-token-parity.mjs（ADR-0012 约束 14 / #22）
# 判据是"两份文件逐字节相同"。注入：给后台那份末尾追加一个注释 —— 模拟最真实的
# 事故：只改了读者站（或只改了后台），另一端忘记同步。门禁必须变红。
ADMIN_TOKENS="frontend_admin/app/assets/css/tokens.css"
if backup_file "$ADMIN_TOKENS"; then
  printf '\n/* canary: 故意只改一端 */\n' >> "$ADMIN_TOKENS"
  if node scripts/check-token-parity.mjs >"$CANARY_TMP/out.txt" 2>&1; then
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

echo ""
echo "─────────────────────────────"
echo "通过：$PASS  失败：$FAIL"
if [ "$FAIL" -ne 0 ]; then
  echo "✗ 存在失效门禁，M1 不能验收"
  echo "  如果怀疑有文件没恢复干净，用 git status 复查（本脚本不会动已提交内容）"
  exit 1
fi
echo "✓ 全部门禁均能在违规时报错"
