#!/usr/bin/env bash
# 本地一键跑 M1 的全部门禁，与 .github/workflows/ci.yml 保持一致。
#
# 为什么需要它：CI 只在你 push 之后才跑，而重构期要频繁验证。
# 这个脚本让"本地跑过"和"CI 绿"是同一件事，避免出现
# "本地随便过、推上去全红"或者反过来"CI 根本没在跑"。
#
# 用法：
#   ./scripts/check.sh              # 全量
#   ./scripts/check.sh backend      # 只跑后端
#   ./scripts/check.sh frontend     # 只跑两个前端
#   ./scripts/check.sh gates        # 只做门禁自检（证明门禁会红）
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Go 不在 PATH 时尝试常见位置（本项目通过 scoop 安装）
if ! command -v go >/dev/null 2>&1; then
  for candidate in "$HOME/scoop/apps/go/current/bin" "$HOME/scoop/apps/go/1.27.3/bin" "/c/Program Files/Go/bin"; do
    if [ -x "$candidate/go" ] || [ -x "$candidate/go.exe" ]; then
      export PATH="$candidate:$PATH"
      break
    fi
  done
fi

# sqlite 测试需要 CGO，cgo 需要 gcc（本项目通过 scoop mingw 提供）
if ! command -v gcc >/dev/null 2>&1; then
  candidate="$HOME/scoop/apps/mingw/current/bin"
  [ -x "$candidate/gcc.exe" ] && export PATH="$candidate:$PATH"
fi

export GOTOOLCHAIN=local
export CGO_ENABLED=1

# 架构违规棘轮：当前存量 319 处，只准减少不准增加。
# 每迁完一个模块就下调，最终目标 0。
# 演进：M1 基线 324 → M2 321 → M3-D（AI 客户端，R7 -1）320
#       → M3-C1 拆分 article_sync.go（R6 -1）319。
ARCH_MAX_VIOLATIONS=319
# nuxt typecheck 在 Windows 上默认堆不够，需要抬高上限
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=8192}"

FAILED=0
run() {
  local name="$1"
  shift
  echo ""
  echo "──── $name ────"
  if "$@"; then
    echo "✓ $name"
  else
    echo "✗ $name"
    FAILED=1
  fi
}

# nuxt prepare 会重写 .nuxt/ 下的生成文件。Windows 上偶发被其他进程
# （杀软/索引/编辑器）短暂占用，报 EPERM: open '.nuxt/eslint.config.mjs'。
# 这不是代码问题，重试即可；Linux CI 上不会出现。
run_prepare() {
  local name="$1"
  shift
  echo ""
  echo "──── $name ────"
  local attempt
  for attempt in 1 2 3; do
    if "$@" >/tmp/nuxt-prepare.log 2>&1; then
      echo "✓ $name"
      return 0
    fi
    if grep -q "EPERM" /tmp/nuxt-prepare.log; then
      echo "  (第 $attempt 次遇到文件占用，重试)"
      sleep 1
      continue
    fi
    break
  done
  echo "✗ $name"
  tail -15 /tmp/nuxt-prepare.log
  FAILED=1
  return 1
}

TARGET="${1:-all}"

if [ "$TARGET" = "all" ] || [ "$TARGET" = "backend" ]; then
  (
    cd backend
    run "backend: go build"     go build ./...
    run "backend: go vet"       go vet ./...
    run "backend: archcheck 规则自检" go test ./tools/archcheck/...
    run "backend: archcheck 棘轮($ARCH_MAX_VIOLATIONS)" \
      go run ./tools/archcheck --mode=error --max-violations="$ARCH_MAX_VIOLATIONS" .
    run "backend: go test (race, short)" go test -race -short -count=1 ./...
  ) || FAILED=1
fi

if [ "$TARGET" = "all" ] || [ "$TARGET" = "frontend" ]; then
  for app in frontend frontend_admin; do
    (
      cd "$app"
      # .nuxt 是生成产物，typecheck / lint 都依赖它先存在
      run_prepare "$app: nuxt prepare" corepack pnpm exec nuxt prepare
      run "$app: lint"         corepack pnpm lint
      run "$app: typecheck"    corepack pnpm typecheck
      run "$app: test"         corepack pnpm test
    ) || FAILED=1
  done
  # 覆盖率门槛只在读者站生效（管理后台暂无测试）
  (
    cd frontend
    run "frontend: coverage 门槛" corepack pnpm test:coverage
  ) || FAILED=1
fi

if [ "$TARGET" = "all" ] || [ "$TARGET" = "gates" ]; then
  run "门禁自检（证明门禁会红）" bash scripts/verify-gates.sh
fi

echo ""
if [ "$FAILED" -ne 0 ]; then
  echo "✗ 门禁未通过"
  exit 1
fi
echo "✓ 全部门禁通过"
