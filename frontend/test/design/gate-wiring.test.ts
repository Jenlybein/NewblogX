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

  it("生成式装饰参数校验出现在 check.sh 的 gates 段、门禁自检与 CI（#286）", () => {
    const cli = "node scripts/generate-deco-parameters.mjs --check";

    const gatesStart = checkSh.indexOf('[ "$TARGET" = "gates" ]');
    expect(gatesStart).toBeGreaterThan(-1);
    const gatesSection = checkSh.slice(gatesStart);
    expect(gatesSection).toContain(cli);
    expect(gatesSection).toContain("bash scripts/verify-gates.sh");

    expect(ciJobBlock("frontend-lint")).toContain(cli);
    expect(verifyGates).toContain(cli);
    expect(verifyGates).toContain("14/14 生成式装饰参数校验");
    expect(verifyGates).toContain("只改后台生成物一端会被判为重新生成后有差异");
  });

  it("两端同源副本门禁接进了 check.sh、verify-gates.sh 与 CI（票 200；票 225 扩到裁剪窗）", () => {
    // #287：命令只在门禁清单里定义一次，三处都经 scripts/gates/run.mjs 调它。
    const cli = "scripts/check-image-shaping-parity.mjs";
    const gateList = readFileSync(join(repoRoot, "scripts/gates/gates.json"), "utf8");
    expect(gateList).toContain(cli);
    expect(gateList).toContain("scripts/check-token-parity.mjs");

    // 本地：frontend 段的仓库级判据（两端 token / 同源副本）按分组调 runner。
    const frontendStart = checkSh.indexOf('[ "$TARGET" = "frontend" ]');
    const gatesStart = checkSh.indexOf('[ "$TARGET" = "gates" ]');
    expect(frontendStart).toBeGreaterThan(-1);
    expect(gatesStart).toBeGreaterThan(frontendStart);
    const frontendSection = checkSh.slice(frontendStart, gatesStart);
    expect(frontendSection).toContain("scripts/gates/run.mjs");
    expect(frontendSection).toContain("--group frontend");

    // CI：与本地同一个入口，挂在仓库级判据所在的 job 里。
    expect(ciJobBlock("frontend-lint")).toContain("node scripts/gates/run.mjs --group frontend");

    // 自检：verify-gates.sh 必须真的注入"只改后台副本一个常量"并断言门禁变红。
    expect(verifyGates).toContain("check-image-shaping-parity.mjs");
    expect(verifyGates).toContain("13/14 两端同源副本门禁（定形工具 + 裁剪窗四份组件）");
    expect(verifyGates).toContain("frontend_admin/app/utils/imageShaping.ts");
    // 票 225：裁剪窗四份组件也必须被自检覆盖，否则"扩了门禁但没人证明它会红"。
    expect(verifyGates).toContain("frontend_admin/app/components/media/ImageCropDialog.vue");
    expect(verifyGates).toContain("两端裁剪窗（ImageCropView / ImageCropDialog）同源门禁失败");
  });
});
