import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");

const ciYml = readFileSync(join(repoRoot, ".github/workflows/ci.yml"), "utf8");
const checkSh = readFileSync(join(repoRoot, "scripts/check.sh"), "utf8");
const verifyGates = readFileSync(join(repoRoot, "scripts/verify-gates.sh"), "utf8");
const baseline = JSON.parse(readFileSync(join(repoRoot, "scripts/design-token-baseline.json"), "utf8")) as {
  frontend: string[];
  frontend_admin: string[];
};

function ciJobBlock(name: string): string {
  const start = ciYml.indexOf(`\n  ${name}:`);
  if (start < 0) throw new Error(`ci.yml 里找不到 job ${name}`);
  const rest = ciYml.slice(start + 1);
  const next = rest.search(/\n {2}[a-z][a-z0-9-]*:\n/u);
  return next < 0 ? rest : rest.slice(0, next);
}

describe("门禁接线：本地脚本与 CI 是同一口径", () => {
  it("CI 的 frontend-lint 在 check:components 之后跑 check:tokens（两端矩阵）", () => {
    const job = ciJobBlock("frontend-lint");
    const componentsIndex = job.indexOf("pnpm check:components");
    const tokensIndex = job.indexOf("pnpm check:tokens");

    expect(componentsIndex).toBeGreaterThan(-1);
    expect(tokensIndex).toBeGreaterThan(componentsIndex);
    expect(job).toContain("matrix");
  });

  it("scripts/check.sh 的 frontend 段也跑 check:tokens（同一个命令）", () => {
    const start = checkSh.indexOf('[ "$TARGET" = "frontend" ]');
    const end = checkSh.indexOf('[ "$TARGET" = "gates" ]');
    const frontendSection = checkSh.slice(start, end);

    expect(start).toBeGreaterThan(-1);
    expect(end).toBeGreaterThan(start);
    expect(frontendSection).toContain("corepack pnpm check:components");
    expect(frontendSection).toContain("corepack pnpm check:tokens");
    expect(frontendSection.indexOf("check:tokens")).toBeGreaterThan(frontendSection.indexOf("check:components"));
  });

  it("baseline 归零：两端都不再有允许文件（此后新增裸色值即失败）", () => {
    expect(baseline.frontend).toEqual([]);
    expect(baseline.frontend_admin).toEqual([]);
  });

  it("门禁自检仍带人造违规 canary（证明门禁会红，不是永远绿）", () => {
    const frontendGateTest = readFileSync(join(projectDir, "test/gates/design-tokens.test.ts"), "utf8");

    expect(frontendGateTest).toContain("__canary.vue");
    expect(frontendGateTest).toContain("canaryViolation");
    expect(frontendGateTest).toContain('kind: "new-color-literal"');
  });

  it("跨端装饰参数门禁同时出现在 check.sh 的 gates 段、verify-gates.sh 与 CI（#43）", () => {
    const cli = "node scripts/check-deco-range-parity.mjs";

    // 本地：gates 段必须跑真命令（验收要求「只改一端 → check.sh gates 变红」）。
    const gatesStart = checkSh.indexOf('[ "$TARGET" = "gates" ]');
    expect(gatesStart).toBeGreaterThan(-1);
    const gatesSection = checkSh.slice(gatesStart);
    expect(gatesSection).toContain(cli);
    expect(gatesSection).toContain("bash scripts/verify-gates.sh");

    // CI：与本地同一条命令，挂在仓库级判据所在的 job 里。
    expect(ciJobBlock("frontend-lint")).toContain(cli);

    // 自检：verify-gates.sh 真的把这道门禁纳入，而不是只在 check.sh 挂个名字。
    expect(verifyGates).toContain("check-deco-range-parity.mjs");
    expect(verifyGates).toContain("13/13 跨端装饰参数一致性门禁");
  });
});
