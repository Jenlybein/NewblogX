import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  DECO_ANCHOR_PARAMETERS,
  DECO_FIXTURE_CONSUMERS,
  DECO_FIXTURE_FILE,
  DECO_SOURCE_FILES,
  checkDecoRangeParity,
  evaluateDecoFixture,
  evaluateDecoRangeParity,
  parseBackendRanges,
} from "../../../scripts/check-deco-range-parity.mjs";

/**
 * 跨端装饰参数一致性门禁（#43；#72 起不再依赖参数总数、支持负值、加锚点清单）。
 *
 * 为什么必须有它：`deco.charm` 的每个数值参数都有**四份独立定义**——后端 Go 常量
 * （范围 + 默认）、读者站 `decoCharm.ts`、后台 `decoParams.ts`、以及
 * `site_default_settings.yaml` 的默认种子。只改一端不会有任何测试变红：读者站测
 * 读者站那份，后台测后台那份，各自内部都是自洽的。门禁必须**故意只改一端就红**，
 * 否则它等于不存在。
 *
 * 判据是"同一参数的 min / max / default 在每一端都必须相等"，不是"读起来像"：
 * 任何解析宽松都会放过真实的漂移。参数个数**故意不写死**（#72）：判定靠
 * "四源键集合相等 + 锚点参数在场"，新增参数不需要改门禁实现。
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

/** 按整行删除（对 CRLF 也成立：先按 \r?\n 切开，再按 trim 后的整行比对）。 */
function dropLines(source: string, lines: string[]): string {
  const rows = source.split(/\r?\n/u);
  const kept = rows.filter((row) => !lines.includes(row.trim()));
  expect(kept.length, `注入失败：源文件里找不到这几行 ${lines.join(" / ")}`).toBe(
    rows.length - lines.length,
  );
  return kept.join("\n");
}

describe("真实仓库为绿：三端范围与默认值一致", () => {
  it("读到四份源文件，四源键集合相等，且值逐项一致", () => {
    const result = checkDecoRangeParity({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    // 扫描器必须真的读到了内容，而不是在空文件上"一致"。
    expect(result.files.reader).toBeGreaterThan(1000);
    expect(result.files.admin).toBeGreaterThan(1000);
    expect(result.files.backend).toBeGreaterThan(1000);
    expect(result.files.yaml).toBeGreaterThan(1000);
    // 参数个数不写死：只要锚点在场，四源键集合相等就由更小的用例分组负责。
    expect(result.anchors.delivered).toContain("rope_length");
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

  it("共享归一化夹具存在、非空，并被后端 / 读者站 / 后台三套测试共同引用", () => {
    const result = checkDecoRangeParity({ repoRoot });

    expect(result.fixture).toMatchObject({ consumers: DECO_FIXTURE_CONSUMERS });
    // 夹具本身必须真的被读到：空表或没人引用都等于没有这道防线。
    expect(result.fixture.caseCount).toBeGreaterThan(40);
    expect(result.fixture.parameters).toContain("rope_length");
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

describe("负值：负数照常参与四端比较，不会被当成 missing-value（#72）", () => {
  it("只把读者站的 min 改成负数 → 报「-0.5 与 0 不一致」", () => {
    const sources = readSources();
    const reader = mutate(
      sources.reader!,
      "export const CHARM_IDLE_SWAY_AMP_MIN = 0;",
      "export const CHARM_IDLE_SWAY_AMP_MIN = -0.5;",
    );
    const result = evaluateDecoRangeParity({ ...sources, reader });

    expect(result.ok).toBe(false);
    expect(result.failures).toHaveLength(1);
    expect(result.failures[0]).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.idle_sway_amp",
      field: "min",
      sides: { backend: 0, reader: -0.5, admin: 0 },
    });
    expect(result.failures[0].disagreeingSides).toEqual(["reader"]);
  });

  it("四端同时改成同一个负 min → 仍然判绿（负区间是合法值，不是解析失败）", () => {
    const sources = readSources();
    const mutated = {
      backend: mutate(sources.backend!, "CharmIdleSwayAmpMin = 0.0", "CharmIdleSwayAmpMin = -5.0"),
      reader: mutate(
        sources.reader!,
        "export const CHARM_IDLE_SWAY_AMP_MIN = 0;",
        "export const CHARM_IDLE_SWAY_AMP_MIN = -5;",
      ),
      admin: mutate(
        sources.admin!,
        "export const DECO_IDLE_SWAY_AMP_MIN = 0",
        "export const DECO_IDLE_SWAY_AMP_MIN = -5",
      ),
    };
    const result = evaluateDecoRangeParity({ ...sources, ...mutated });

    expect(result.failures).toEqual([]);
    expect(result.ok).toBe(true);
    expect(result.parameters.idle_sway_amp?.backend?.min).toBe(-5);
    expect(result.parameters.idle_sway_amp?.reader?.min).toBe(-5);
    expect(result.parameters.idle_sway_amp?.admin?.min).toBe(-5);
  });
});

describe("键集合相等：判定不再依赖「参数总数」（#72）", () => {
  it("某端整块少了一个参数（min/max/default 三条都没了）→ 报该端缺键，而不是零散缺失", () => {
    const sources = readSources();
    const reader = dropLines(sources.reader!, [
      "export const CHARM_GRAVITY_MIN = 400;",
      "export const CHARM_GRAVITY_MAX = 4000;",
      "export const CHARM_GRAVITY_DEFAULT = 1600;",
    ]);
    const result = evaluateDecoRangeParity({ ...sources, reader });

    expect(result.ok).toBe(false);
    // 一条就够：缺的是"键"，不是三个互不相干的取值。
    expect(result.failures).toHaveLength(1);
    expect(result.failures[0]).toMatchObject({
      kind: "key-set-mismatch",
      side: "reader",
      missingKeys: ["gravity"],
      extraKeys: [],
      path: DECO_SOURCE_FILES.reader,
    });
    expect(result.failures[0].message).toContain("charm.gravity");
  });

  it("某端多出一个别端没有的参数 → 报该端多键，并点名参数", () => {
    const sources = readSources();
    const admin = `${sources.admin!}
export const DECO_GHOST_MIN = 1
export const DECO_GHOST_MAX = 2
export const DECO_GHOST_DEFAULT = 1
`;
    const result = evaluateDecoRangeParity({ ...sources, admin });

    expect(result.ok).toBe(false);
    const failure = result.failures.find(
      (item: { kind: string }) => item.kind === "key-set-mismatch",
    );
    expect(failure).toMatchObject({
      side: "admin",
      missingKeys: [],
      extraKeys: ["ghost"],
      path: DECO_SOURCE_FILES.admin,
    });
    expect(failure.message).toContain("charm.ghost");
  });
});

describe("锚点参数：拦住「参数从四端同时消失」（#72）", () => {
  it("Go 常量名里的缩写也归一成锚点键：CharmFPSCapMin → fps_cap（#74 命名自由度）", () => {
    const result = parseBackendRanges(
      ["CharmFPSCapMin = 30.0", "CharmFPSCapMax = 160.0", "CharmPointerOffsetXMin = -24.0"].join(
        "\n",
      ),
    );

    expect(Object.keys(result.parameters).sort()).toEqual(["fps_cap", "pointer_offset_x"]);
    expect(result.parameters.fps_cap).toMatchObject({ min: 30, max: 160 });
    expect(result.parameters.pointer_offset_x?.min).toBe(-24);
  });

  it("锚点清单就是票面那四个（挂绳长度 / 帧率上限 / 指针端偏移 X / Y）", () => {
    expect(DECO_ANCHOR_PARAMETERS.map((anchor: { key: string }) => anchor.key)).toEqual([
      "rope_length",
      "fps_cap",
      "pointer_offset_x",
      "pointer_offset_y",
    ]);
    // 每个锚点都得说清"现在该不该在场"：未交付的锚点不能把门禁自己搞红。
    for (const anchor of DECO_ANCHOR_PARAMETERS) {
      expect(["delivered", "pending"]).toContain(anchor.status);
    }
  });

  it("未交付的锚点在四源都不存在时门禁仍为绿，只是列进待交付", () => {
    const result = checkDecoRangeParity({ repoRoot });

    expect(result.failures).toEqual([]);
    expect(result.ok).toBe(true);
    expect(result.anchors.delivered).toContain("rope_length");
    // 今天 fps_cap / pointer_offset_* 还没实现（#74/#75）；门禁必须能分辨
    // "还没交付"与"已经丢了"，而不是把两者一起报红。
    expect(result.anchors.pending).toEqual(["fps_cap", "pointer_offset_x", "pointer_offset_y"]);
  });

  it("已交付的锚点从四源同时消失 → 红，并点名参数与四端", () => {
    const sources = readSources();
    const stripped = {
      backend: dropLines(sources.backend!, [
        "CharmRopeLengthMin = 40.0",
        "CharmRopeLengthMax = 240.0",
        "CharmRopeLengthDefault = 120.0",
      ]),
      reader: dropLines(sources.reader!, [
        "export const CHARM_ROPE_LENGTH_MIN = 40;",
        "export const CHARM_ROPE_LENGTH_MAX = 240;",
        "export const CHARM_ROPE_LENGTH_DEFAULT = 120;",
      ]),
      admin: dropLines(sources.admin!, [
        "export const DECO_ROPE_LENGTH_MIN = 40",
        "export const DECO_ROPE_LENGTH_MAX = 240",
        "export const DECO_ROPE_LENGTH_DEFAULT = 120",
      ]),
      yaml: dropLines(sources.yaml!, ["rope_length: 120"]),
    };
    const result = evaluateDecoRangeParity({ ...sources, ...stripped });

    expect(result.ok).toBe(false);
    // 四源键集合仍然相等 —— 键集合判不出这种漂移，必须由锚点兜住。
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "anchor-missing",
    ]);
    const failure = result.failures[0];
    expect(failure).toMatchObject({ parameter: "charm.rope_length" });
    expect(failure.message).toContain("charm.rope_length");
    for (const side of ["backend", "reader", "admin", "yaml"] as const) {
      expect(failure.message).toContain(DECO_SOURCE_FILES[side]);
    }
  });

  it("未交付的锚点只交付了一半（少了默认种子）→ 红，并指出是哪一端缺哪个参数", () => {
    const sources = readSources();
    const result = evaluateDecoRangeParity({
      ...sources,
      backend: `${sources.backend!}
	CharmFpsCapMin = 30.0
	CharmFpsCapMax = 160.0
	CharmFpsCapDefault = 60.0
`,
      reader: `${sources.reader!}
export const CHARM_FPS_CAP_MIN = 30;
export const CHARM_FPS_CAP_MAX = 160;
export const CHARM_FPS_CAP_DEFAULT = 60;
`,
      admin: `${sources.admin!}
export const DECO_FPS_CAP_MIN = 30
export const DECO_FPS_CAP_MAX = 160
export const DECO_FPS_CAP_DEFAULT = 60
`,
    });

    expect(result.ok).toBe(false);
    const failure = result.failures.find(
      (item: { kind: string }) => item.kind === "key-set-mismatch",
    );
    expect(failure).toMatchObject({ side: "yaml", missingKeys: ["fps_cap"] });
    expect(failure.message).toContain("charm.fps_cap");
    expect(failure.message).toContain(DECO_SOURCE_FILES.yaml);
  });

  it("锚点只要四端齐全就自动纳入比较：门禁实现不用改，改一端照样红", () => {
    const sources = readSources();
    const delivered = {
      backend: `${sources.backend!}
	CharmFpsCapMin = 30.0
	CharmFpsCapMax = 160.0
	CharmFpsCapDefault = 60.0
`,
      reader: `${sources.reader!}
export const CHARM_FPS_CAP_MIN = 30;
export const CHARM_FPS_CAP_MAX = 160;
export const CHARM_FPS_CAP_DEFAULT = 60;
`,
      admin: `${sources.admin!}
export const DECO_FPS_CAP_MIN = 30
export const DECO_FPS_CAP_MAX = 160
export const DECO_FPS_CAP_DEFAULT = 60
`,
      yaml: mutate(sources.yaml!, "    idle_sway_amp: 20", "    idle_sway_amp: 20\n    fps_cap: 60"),
    };
    const green = evaluateDecoRangeParity({ ...sources, ...delivered });

    expect(green.failures).toEqual([]);
    expect(green.ok).toBe(true);
    expect(green.anchors.delivered).toContain("fps_cap");
    expect(green.parameters.fps_cap).toMatchObject({
      backend: { min: 30, max: 160, default: 60 },
      reader: { min: 30, max: 160, default: 60 },
      admin: { min: 30, max: 160, default: 60 },
      yaml: { default: 60 },
    });

    // 交付之后它就是普通参数：只改一端必须红，且指出是哪一端。
    const drifted = mutate(delivered.admin, "DECO_FPS_CAP_MAX = 160", "DECO_FPS_CAP_MAX = 200");
    const red = evaluateDecoRangeParity({ ...sources, ...delivered, admin: drifted });

    expect(red.ok).toBe(false);
    expect(red.failures).toHaveLength(1);
    expect(red.failures[0]).toMatchObject({
      kind: "value-mismatch",
      parameter: "charm.fps_cap",
      field: "max",
      disagreeingSides: ["admin"],
    });
  });
});

/** 共享夹具与三套消费方的真实文本（canary 用人造文本调用纯判定函数）。 */
function readFixture(): string {
  return readFileSync(join(repoRoot, DECO_FIXTURE_FILE), "utf8");
}

function readConsumers(): Record<string, string> {
  const texts: Record<string, string> = {};
  for (const relativePath of DECO_FIXTURE_CONSUMERS) {
    texts[relativePath] = readFileSync(join(repoRoot, relativePath), "utf8");
  }
  return texts;
}

describe("共享归一化夹具的 canary：没人用或没有文件都必须红", () => {
  it("真实仓库：夹具存在且三套测试都引用它", () => {
    const result = evaluateDecoFixture({
      fixture: readFixture(),
      consumers: readConsumers(),
    });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    expect(result.caseCount).toBeGreaterThan(40);
    expect(result.parameters).toContain("rope_length");
  });

  it("夹具文件缺失 → 报 fixture-missing，而不是当成「没有用例」", () => {
    const result = evaluateDecoFixture({ fixture: null, consumers: readConsumers() });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toContain(
      "fixture-missing",
    );
  });

  it("某套测试不再引用夹具 → 报 fixture-unreferenced 并点名该文件", () => {
    const consumers = readConsumers();
    const target = DECO_FIXTURE_CONSUMERS[1]!;
    const mutated = consumers[target]!.replace(DECO_FIXTURE_FILE, "");    expect(mutated, `${target} 里没有夹具路径，canary 注入失败`).not.toBe(consumers[target]);

    const result = evaluateDecoFixture({
      fixture: readFixture(),
      consumers: { ...consumers, [target]: mutated },
    });

    expect(result.ok).toBe(false);
    const failure = result.failures.find(
      (item: { kind: string }) => item.kind === "fixture-unreferenced",
    );
    expect(failure).toMatchObject({ path: target });
    expect(failure.message).toContain(DECO_FIXTURE_FILE);
  });

  it("夹具不是合法 JSON → 报 fixture-invalid", () => {
    const result = evaluateDecoFixture({ fixture: "{", consumers: readConsumers() });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toContain(
      "fixture-invalid",
    );
  });
});
