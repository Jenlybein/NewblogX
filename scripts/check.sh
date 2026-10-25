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

# 架构违规棘轮：当前存量与演进历史见 scripts/arch-baseline.env。
# 数字只从那一个文件读，避免本地 / CI / 自检脚本三处各写一遍后漂移。
# shellcheck source=scripts/arch-baseline.env
. "$ROOT/scripts/arch-baseline.env"
# nuxt typecheck 在 Windows 上默认堆不够，需要抬高上限
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=8192}"

FAILED=0
# run 执行一条门禁命令。
#
# 两个坑都在这里堵掉，否则门禁会"撒谎"：
#   1. 必须 return 1：只设置 FAILED 而不返回非 0，调用方无从判断失败；
#   2. 调用方若在子 shell ( ... ) 里跑，子 shell 的退出码只等于**最后一条**命令，
#      前面某步失败会被吞掉。所以每个子 shell 末尾必须显式 exit "$FAILED"
#      （见下面 backend / frontend 两处）。
run() {
  local name="$1"
  shift
  echo ""
  echo "──── $name ────"
  if "$@"; then
    echo "✓ $name"
    return 0
  fi
  echo "✗ $name"
  FAILED=1
  return 1
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

# 集成测试（-tags integration）需要 Docker：testcontainers 会起 MySQL / ClickHouse。
#
# 关键在于"跳过"不能是静默的 —— 静默跳过正是当年门禁撒谎的老毛病。
# 三种情形分开处理：
#   1. BLOGX_SKIP_INTEGRATION=1：显式跳过，打印警告（本地临时调试用）；
#   2. Docker 不可用：跳过 + 醒目警告；若同时设 BLOGX_REQUIRE_INTEGRATION=1 则直接失败（CI 用）；
#   3. 正常：跑，失败即门禁失败。
#
# 这些测试覆盖 golden（20 篇 × 12 接口逐字节）、article/user 仓储真实 SQL、
# outbox 中继、日志管道 —— 都是只跑 -short 验证不到的东西。
run_integration() {
  local name="backend: go test (integration)"
  echo ""
  echo "──── $name ────"

  if [ "${BLOGX_SKIP_INTEGRATION:-0}" = "1" ]; then
    echo "⚠ 已跳过：BLOGX_SKIP_INTEGRATION=1（golden 与容器契约本次未验证）"
    return 0
  fi

  if ! docker info >/dev/null 2>&1; then
    if [ "${BLOGX_REQUIRE_INTEGRATION:-0}" = "1" ]; then
      echo "✗ $name：Docker 不可用，但设置了 BLOGX_REQUIRE_INTEGRATION=1"
      FAILED=1
      return 1
    fi
    echo "⚠ 已跳过：Docker 不可用（golden 与容器契约本次未验证）"
    echo "  本机启动 Docker Desktop 即可；CI 上这个步骤是强制的"
    return 0
  fi

  # 起一个临时 MySQL 供所有测试包共用。
  #
  # 为什么必须共用：testkit 的 sync.Once 是**包级**的，而 go test ./... 每个包
  # 编译成独立进程 —— 于是 12 个容器测试包会各起一个 MySQL。并行时它们互相抢
  # 资源（Windows 的 Docker Desktop 上实测约五成概率把就绪检测拖到超时），
  # 串行时又要为每个包重复付 12-15 秒启动开销（实测全量 10 分钟）。
  # 共用一个实例后：每个测试仍各建各的独立 schema，实测 1 分 40 秒且稳定。
  local started="" shared_ok=0
  if [ -n "${BLOGX_MYSQL_ADDR:-}" ]; then
    echo "  使用外部 MySQL：$BLOGX_MYSQL_ADDR"
    shared_ok=1
  else
    local ctr="blogx-it-mysql-$$" port=""
    if docker run -d --name "$ctr" -p 0:3306 \
        -e MYSQL_ROOT_PASSWORD=root -e TZ=Asia/Shanghai mysql:8.0 >/dev/null 2>&1; then
      port="$(docker port "$ctr" 3306/tcp 2>/dev/null | head -1 | sed 's/.*://')"
    fi
    if [ -n "$port" ]; then
      local i
      for i in $(seq 1 60); do
        # 容器"已启动"不等于"可连接"，必须等 ping 通。
        if docker exec "$ctr" mysqladmin ping -uroot -proot --silent >/dev/null 2>&1; then
          export BLOGX_MYSQL_ADDR="127.0.0.1:$port"
          started="$ctr"
          shared_ok=1
          echo "  临时 MySQL：$BLOGX_MYSQL_ADDR（跑完自动删除）"
          break
        fi
        sleep 1
      done
    fi
    if [ "$shared_ok" != 1 ]; then
      [ -n "$ctr" ] && docker rm -f "$ctr" >/dev/null 2>&1
      echo "  临时 MySQL 未就绪，回退为每包自起容器（会明显变慢，且包级串行）"
    fi
  fi

  # 没有共享实例时必须 -p 1：否则多个 MySQL 容器并发启动互相拖垮。
  local parallel=""
  [ "$shared_ok" != 1 ] && parallel="-p 1"

  # race 开关。
  #
  # 默认开：race 能抓数据竞争，关掉等于少一类真实缺陷的防线。
  # 但它让整轮慢一倍以上（实测无 race 93s / 带 race 215s），
  # 迭代时可设 BLOGX_FAST=1 跳过，提交前再完整跑一次。
  local race=""
  if [ "${BLOGX_FAST:-0}" = "1" ]; then
    echo "  ⚠ BLOGX_FAST=1：本次未启用 -race（提交前请完整跑一次）"
  else
    race="-race"
  fi

  local ok=0
  # shellcheck disable=SC2086
  if go test $race -tags integration -count=1 -timeout 20m $parallel ./...; then ok=1; fi
  [ -n "$started" ] && docker rm -f "$started" >/dev/null 2>&1

  if [ "$ok" = 1 ]; then
    echo "✓ $name"
    return 0
  fi
  echo "✗ $name"
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
    # 测试只跑一遍。
    #
    # 项目里**没有任何测试**使用 testing.Short()，所以过去"先 -race -short 跑一遍、
    # 再 -tags integration 跑一遍"是重复劳动：后者是前者的超集（多编译 integration
    # 测试文件）。实测 193s + 98s，其中 193s 全是重复。
    # 现在合成一次：-tags integration 已包含全部非 integration 测试。
    #
    # 显式跳过集成时，退回"仅单元测试"，免得一个测试都不跑。
    if [ "${BLOGX_SKIP_INTEGRATION:-0}" = "1" ]; then
      run "backend: go test (race, 仅单元)" go test -race -short -count=1 ./...
      echo "⚠ BLOGX_SKIP_INTEGRATION=1：golden 与容器契约本次未验证"
    else
      run_integration
    fi
    # 关键：把子 shell 内累积的 FAILED 作为退出码带出去，
    # 否则只有最后一条命令的成败能传出去，中间步骤失败会被吞掉。
    exit "$FAILED"
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
      exit "$FAILED"
    ) || FAILED=1
  done
  # 覆盖率门槛只在读者站生效（管理后台暂无测试）
  (
    cd frontend
    run "frontend: coverage 门槛" corepack pnpm test:coverage
    exit "$FAILED"
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
