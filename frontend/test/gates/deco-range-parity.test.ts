import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  DECO_SOURCE_FILES,
  checkDecoRangeParity,
  evaluateDecoRangeParity,
} from "../../../scripts/check-deco-range-parity.mjs";

/**
 * 跨端装饰参数一致性门禁（#43）。
 *
 * 为什么必须有它：`deco.charm` 的九个参数（一个布尔默认 + 八个数值）现在有
 * **三份独立定义**——后端 Go 常量（范围 + 默认）、读者站 `decoCharm.ts`、
 * 后台 `decoParams.ts`（外加 `site_default_settings.yaml` 的默认种子）。
 * 只改一端不会有任何测试变红：读者站测读者站那份，后台测后台那份，各自内部
 * 都是自洽的。门禁必须**故意只改一端就红**，否则它等于不存在。
 *
 * 判据是"同一参数的 min / max / default 在每一端都必须相等"，不是"读起来像"：
 * 任何解析宽松都会放过真实的漂移。
 */

// vitest 以 project root（frontend/）为 cwd；仓库根是上一级。
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");

function readSources(overrides: Partial<Record<string, string>> = {}): Record<string, string> {
  const sources: Record<string, string> = {};
  for (const [side, relativePath] of Object.entries(DECO_SOURCE_FILES) as [string, string][]) {
    sources[side] = overrides[side] ?? readFileSync(join(repoRoot, relativePath), "utf8");
  }
  return sources;
}

function mutate(source: string, from: string, to: string): string {
  const mutated = source.replace(from, to);
  expect(mutated, `注入失败：源文件里找不到 ${from}`).not.toBe(source);
  return mutated;
}

describe("真实仓库为绿：三端范围与默认值一致", () => {
  it("读到四份源文件，八个数值参数在三端齐全，且值逐项一致", () => {
    const result = checkDecoRangeParity({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    // 扫描器必须真的读到了内容，而不是在空文件上"一致"。
    expect(result.files.reader).toBeGreaterThan(1000);
    expect(result.files.admin).toBeGreaterThan(1000);
    expect(result.files.backend).toBeGreaterThan(1000);
    expect(result.files.yaml).toBeGreaterThan(1000);
    expect(Object.keys(result.parameters)).toHaveLength(8);
    // 用票面的独立事实（不是从实现里再算一遍）钉住"读对了"。
    expect(result.parameters.rope_length).toMatchObject({
      backend: { min: 40, max: 240, default: 120 },
      reader: { min: 40, max: 240, default: 120 },
      admin: { min: 40, max: 240, default: 120 },
      yaml: { default: 120 },
    });
    expect(result.booleanDefault).toEqual({ yaml: true, admin: true });
  });

  it("未改动任何一端时纯函数判定为绿", () => {
    const result = evaluateDecoRangeParity(readSources());
    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
  });
});

describe("canary：只改一端就必须红，并指出是哪两端、哪个参数", () => {
  it("只改后台的挂绳长度上限 → 报后台与后端/读者站在 charm.rope_length.max 上不一致", () => {
    const admin = mutate(
      readSources().admin!,
      "DECO_ROPE_LENGTH_MAX = 240",
      "DECO_ROPE_LENGTH_MAX = 260",
    );
    const result = evaluateDecoRangeParity(readSources({ admin }));

    expect(result.ok).toBe(false);
    expect(result.failures).toHaveLength(1);
    const failure = result.failures[0];
    expect(failure).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.rope_length",
      field: "max",
      sides: { backend: 240, reader: 240, admin: 260 },
    });
    expect(failure.disagreeingSides).toEqual(["admin"]);
    // 报错必须同时指出"是哪两端"和"差在哪"：只报一个红灯毫无用处。
    expect(failure.message).toContain(DECO_SOURCE_FILES.backend);
    expect(failure.message).toContain(DECO_SOURCE_FILES.admin);
    expect(failure.message).toContain("240");
    expect(failure.message).toContain("260");
  });

  it("只改后端的重力默认值 → 报后端与读者站/后台/默认种子上不一致", () => {
    const backend = mutate(
      readSources().backend!,
      "CharmGravityDefault = 1600.0",
      "CharmGravityDefault = 1800.0",
    );
    const result = evaluateDecoRangeParity(readSources({ backend }));

    expect(result.ok).toBe(false);
    expect(result.failures).toHaveLength(1);
    const failure = result.failures[0];
    expect(failure).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.gravity",
      field: "default",
      sides: { backend: 1800, reader: 1600, admin: 1600, yaml: 1600 },
    });
    expect(failure.disagreeingSides).toEqual(["backend"]);
    expect(failure.message).toContain(DECO_SOURCE_FILES.backend);
    expect(failure.message).toContain(DECO_SOURCE_FILES.reader);
    expect(failure.message).toContain("1800");
    expect(failure.message).toContain("1600");
  });

  it("只改读者站的节点数上限 → 报读者站与后端在 charm.segments.max 上不一致", () => {
    const reader = mutate(
      readSources().reader!,
      "CHARM_SEGMENTS_MAX = 24",
      "CHARM_SEGMENTS_MAX = 30",
    );
    const result = evaluateDecoRangeParity(readSources({ reader }));

    expect(result.ok).toBe(false);
    expect(result.failures).toHaveLength(1);
    const failure = result.failures[0];
    expect(failure).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.segments",
      field: "max",
      sides: { backend: 24, reader: 30, admin: 24 },
    });
    expect(failure.disagreeingSides).toEqual(["reader"]);
    expect(failure.message).toContain(DECO_SOURCE_FILES.reader);
    expect(failure.message).toContain(DECO_SOURCE_FILES.admin);
  });

  it("只改后台的布尔默认值 enable → 报总开关默认值与后端种子不一致", () => {
    const admin = mutate(
      readSources().admin!,
      /(export function defaultDecoConfig\(\)[\s\S]*?enable:\s*)true/u,
      "$1false",
    );
    const result = evaluateDecoRangeParity(readSources({ admin }));

    expect(result.ok).toBe(false);
    expect(result.failures).toHaveLength(1);
    const failure = result.failures[0];
    expect(failure).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.enable",
      field: "default",
      sides: { yaml: true, admin: false },
    });
    expect(failure.message).toContain(DECO_SOURCE_FILES.yaml);
    expect(failure.message).toContain(DECO_SOURCE_FILES.admin);
  });

  it("只删读者站的一条范围常量 → 报缺失，而不是静默跳过", () => {
    const reader = mutate(
      readSources().reader!,
      "export const CHARM_IDLE_SWAY_AMP_MIN = 0;\n",
      "",
    );
    const result = evaluateDecoRangeParity(readSources({ reader }));

    expect(result.ok).toBe(false);
    const failure = result.failures.find(
      (item: { kind: string }) => item.kind === "missing-value",
    );
    expect(failure).toMatchObject({
      parameter: "charm.idle_sway_amp",
      field: "min",
      side: "reader",
    });
    expect(failure.message).toContain(DECO_SOURCE_FILES.reader);
  });

  it("源文件缺失时失败，而不是把空内容当成一致", () => {
    const result = checkDecoRangeParity({ repoRoot: join(projectDir, "test/gates/__missing") });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "file-missing",
      "file-missing",
      "file-missing",
      "file-missing",
    ]);
  });
});
