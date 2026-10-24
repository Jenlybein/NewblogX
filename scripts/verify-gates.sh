#!/usr/bin/env bash
# 门禁自检（canary）：证明每一道门禁"真的会失败"。
#
# 为什么必须有这个脚本：
#   项目上一轮的治理之所以失效，是因为那套守卫检查的 5 个符号根本不存在，
#   于是 13 条规则全部永真通过。表面上"有守卫"，实际上没有任何东西被拦住。
#   判断这次是否又变成表演，唯一可靠的办法是：
#   **故意制造违规，看门禁是否会红。**
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

PASS=0
FAIL=0
CLEANUP=()

cleanup() {
  for f in "${CLEANUP[@]:-}"; do
    [ -n "$f" ] && rm -f "$f" 2>/dev/null
  done
  rmdir backend/internal/canary 2>/dev/null
}
trap cleanup EXIT

expect_fail() {
  local name="$1"
  shift
  if "$@" >/tmp/canary-out.txt 2>&1; then
    echo "✗ 门禁失效：$name 没有失败"
    FAIL=$((FAIL+1))
  else
    echo "✓ 门禁有效：$name 正确失败"
    PASS=$((PASS+1))
  fi
}

echo "===== 1/4 后端单测门禁 ====="
CANARY_TEST="backend/internal/canary/canary_test.go"
mkdir -p backend/internal/canary
cat > "$CANARY_TEST" <<'EOF'
package canary

import "testing"

// 这个测试必须失败。如果 CI 在它存在时仍然通过，说明单测门禁是假的。
func TestCanaryMustFail(t *testing.T) {
	t.Fatal("canary: 这条断言必须失败")
}
EOF
CLEANUP+=("$CANARY_TEST")
expect_fail "backend 单测" bash -c 'cd backend && go test ./internal/canary/'
rm -f "$CANARY_TEST"; rmdir backend/internal/canary 2>/dev/null

echo ""
echo "===== 2/4 架构规则门禁（archcheck 棘轮） ====="
CANARY_ARCH="backend/api/zz_canary/enter.go"
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
CLEANUP+=("$CANARY_ARCH")
expect_fail "archcheck 棘轮（新增违规即超阈值）" \
  bash -c 'cd backend && go run ./tools/archcheck --mode=error --max-violations=319 .'
rm -f "$CANARY_ARCH"; rmdir backend/api/zz_canary 2>/dev/null

echo ""
echo "===== 3/4 archcheck 规则自检 ====="
# 规则自检本身也是门禁：如果把某条规则改坏（不再报错），rules_test 必须红。
expect_noop_pass() {
  local name="$1"; shift
  if "$@" >/tmp/canary-out.txt 2>&1; then
    echo "✓ 门禁有效：$name 通过（规则自检齐全）"
    PASS=$((PASS+1))
  else
    echo "✗ $name 失败"
    tail -5 /tmp/canary-out.txt
    FAIL=$((FAIL+1))
  fi
}
expect_noop_pass "archcheck 规则自检" bash -c 'cd backend && go test ./tools/archcheck/...'

echo ""
echo "===== 4/4 前端类型门禁 ====="
CANARY_TS="frontend/app/utils/__canary.ts"
cat > "$CANARY_TS" <<'EOF'
// 故意制造类型错误：把一个字符串赋给 number。
// typecheck 必须失败；如果通过，说明类型门禁是假的。
export const canaryNumber: number = "not a number";
EOF
CLEANUP+=("$CANARY_TS")
# 注意：tsc 只看被引用的文件，所以用 vue-tsc 全量检查时也会覆盖 app/ 下的文件。
expect_fail "前端 typecheck" bash -c 'cd frontend && NODE_OPTIONS="--max-old-space-size=8192" corepack pnpm typecheck'
rm -f "$CANARY_TS"

echo ""
echo "─────────────────────────────"
echo "通过：$PASS  失败：$FAIL"
if [ "$FAIL" -ne 0 ]; then
  echo "✗ 存在失效门禁，M1 不能验收"
  exit 1
fi
echo "✓ 全部门禁均能在违规时报错"
