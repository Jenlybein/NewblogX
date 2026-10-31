// 「跨端装饰参数一致性」门禁（#43）。
//
// 为什么必须有它（票 #35 用户故事 36/37）：`deco.charm` 的九个参数——一个布尔
// 默认值 + 八个数值范围——现在有**三份独立定义**：
//   1. 后端 `backend/conf/conf_deco.go`：写入路径的钳位边界与缺省回落值；
//   2. 读者站 `frontend/app/utils/decoCharm.ts`：第二道钳位与门控默认；
//   3. 后台 `frontend_admin/app/utils/decoParams.ts`：表单元数据与"恢复默认"。
// 外加 `backend/config/site_default_settings.yaml` 的**默认种子**（库里缺这段时
// 补种的值，也是票面参数表的默认值出处）。
//
// 三端各自内部都是自洽的，所以任何一端的单测都不会因为另一端漂移而变红：
// 只改后端会得到"后台能填 260、读者站却按 240 钳"的线上异常，而 CI 全绿。
// 这道门禁的唯一职责就是让"只改一端"立刻变红。
//
// 判据：**同一参数的 min / max / default 在每一端都必须相等**（布尔参数只有
// 默认值）。不是"解析后看起来差不多"：浮点用字面量精确比较，任何一点差别都是
// 有人只改了一端。
//
// 刻意**不**维护第三张权威参数表：参数名从三份源文件里**发现**（Go 的
// `CharmXxxMin`、读者站 `CHARM_XXX_MIN`、后台 `DECO_XXX_MIN` 按命名约定归一成
// `xxx`），脚本只做比较。这样新增/删除参数时门禁自动跟着变，不会出现"门禁表
// 和真表漂移"的第二层事故。参数完整性由 yaml 的 `deco.charm` 键集兜底：
// 哪一端少了某个键，就报"缺少参数"，而不是静默跳过。
//
// canary 由两处保证它会红：
//   1. `frontend/test/gates/deco-range-parity.test.ts`：在内存里只改一端的文本，
//      断言失败并点名"哪两端、哪个参数、差在哪"；
//   2. `scripts/verify-gates.sh`：在真实仓库里故意只改后台一个常量，断言门禁红。
// 只有"真实仓库为绿 + 人造不一致为红"同时成立，这道门禁才算存在。
//
// 用法：node scripts/check-deco-range-parity.mjs [--report]
//   --report：打印每个参数在每一端的取值（排查用，不做判定）
// 退出码：0 = 全部一致；1 = 有不一致 / 缺参数 / 源文件缺失
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

/** 四份源文件（相对仓库根）。`yaml` 同时是默认种子与参数完整性的键集。 */
export const DECO_SOURCE_FILES = {
  backend: "backend/conf/conf_deco.go",
  yaml: "backend/config/site_default_settings.yaml",
  reader: "frontend/app/utils/decoCharm.ts",
  admin: "frontend_admin/app/utils/decoParams.ts",
};

/** 三份**代码**源文件：min / max / default 都必须齐全。 */
const CODE_SIDES = ["backend", "reader", "admin"];
/** 默认值多一个 yaml 种子：默认是"票面参数表的出处"，它也参与比较。 */
const DEFAULT_SIDES = ["backend", "reader", "admin", "yaml"];
/** 布尔默认值只有两处独立定义：后端种子与后台"恢复默认"。 */
const BOOLEAN_SIDES = ["yaml", "admin"];

const SIDE_LABELS = {
  backend: "后端 Go 常量",
  reader: "读者站",
  admin: "后台",
  yaml: "后端默认种子（yaml）",
};

const FIELD_LABELS = {
  min: "下限",
  max: "上限",
  default: "默认值",
};

/** 从工作目录向上找到仓库根（该模块也被 vitest import，不能用 import.meta.url）。 */
export function findRepoRoot(fromDir = process.cwd()) {
  let dir = resolve(fromDir);
  for (;;) {
    const scriptPath = join(dir, "scripts", "check-deco-range-parity.mjs");
    if (existsSync(join(dir, DECO_SOURCE_FILES.reader)) && existsSync(scriptPath)) return dir;
    const parent = dirname(dir);
    if (parent === dir) return resolve(fromDir);
    dir = parent;
  }
}

/** Go 的驼峰参数名 → 跨端统一键：`LogoSize` → `logo_size`，`IdleSwayHz` → `idle_sway_hz`。 */
function snakeKey(camel) {
  return camel.replace(/[A-Z]/gu, (char) => `_${char.toLowerCase()}`).replace(/^_/u, "");
}

/** 三端共用的"放进参数槽"逻辑：同一端同一参数同一字段出现两次是源文件写重了。 */
function put(parameters, key, field, value, side, duplicates) {
  const slot = (parameters[key] ??= {});
  if (slot[field] === undefined) {
    slot[field] = value;
    return;
  }
  if (slot[field] !== value) {
    duplicates.push({
      kind: "duplicate-value",
      parameter: `charm.${key}`,
      field,
      side,
      path: DECO_SOURCE_FILES[side],
      message: `[charm.${key}.${field}] ${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]} 里重复定义为 ${slot[field]} 与 ${value}：源文件自身已漂移`,
    });
  }
}

/** 解析后端 Go 常量：`CharmXxxMin|Max|Default = <number>`。行首锚定，绝不匹配代码里的引用。 */
export function parseBackendRanges(source) {
  const parameters = {};
  const failures = [];
  const duplicates = [];
  for (const raw of source.split(/\r?\n/u)) {
    const match = /^Charm([A-Z][A-Za-z0-9]*)(Min|Max|Default)\s*=\s*([0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?)/u.exec(
      raw.trim(),
    );
    if (!match) continue;
    const [, camel, suffix, token] = match;
    const value = Number(token);
    if (!Number.isFinite(value)) {
      failures.push({
        kind: "parse-error",
        parameter: `charm.${snakeKey(camel)}`,
        source: "backend",
        message: `后端 ${DECO_SOURCE_FILES.backend} 的 Charm${camel}${suffix} 值不是数字：${token}`,
      });
      continue;
    }
    put(parameters, snakeKey(camel), suffix.toLowerCase(), value, "backend", duplicates);
  }
  return { parameters, failures, duplicates };
}

/** 解析读者站 TS 常量：`export const CHARM_XXX_MIN|MAX|DEFAULT = <number>;`。 */
export function parseReaderRanges(source) {
  const parameters = {};
  const failures = [];
  const duplicates = [];
  for (const raw of source.split(/\r?\n/u)) {
    const match = /^export const CHARM_([A-Z0-9_]+)_(MIN|MAX|DEFAULT)\s*=\s*([0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?)/u.exec(
      raw.trim(),
    );
    if (!match) continue;
    const [, key, suffix, token] = match;
    const value = Number(token);
    if (!Number.isFinite(value)) {
      failures.push({
        kind: "parse-error",
        parameter: `charm.${key.toLowerCase()}`,
        source: "reader",
        message: `读者站 ${DECO_SOURCE_FILES.reader} 的 CHARM_${key}_${suffix} 值不是数字：${token}`,
      });
      continue;
    }
    put(parameters, key.toLowerCase(), suffix.toLowerCase(), value, "reader", duplicates);
  }
  return { parameters, failures, duplicates };
}

/** 解析后台 TS 常量：`export const DECO_XXX_MIN|MAX|DEFAULT = <number>`。 */
export function parseAdminRanges(source) {
  const parameters = {};
  const failures = [];
  const duplicates = [];
  for (const raw of source.split(/\r?\n/u)) {
    const match = /^export const DECO_([A-Z0-9_]+)_(MIN|MAX|DEFAULT)\s*=\s*([0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?)/u.exec(
      raw.trim(),
    );
    if (!match) continue;
    const [, key, suffix, token] = match;
    const value = Number(token);
    if (!Number.isFinite(value)) {
      failures.push({
        kind: "parse-error",
        parameter: `charm.${key.toLowerCase()}`,
        source: "admin",
        message: `后台 ${DECO_SOURCE_FILES.admin} 的 DECO_${key}_${suffix} 值不是数字：${token}`,
      });
      continue;
    }
    put(parameters, key.toLowerCase(), suffix.toLowerCase(), value, "admin", duplicates);
  }
  return { parameters, failures, duplicates };
}

/** 解析后台"恢复默认"的布尔默认值：只认 `defaultDecoConfig()` 函数体里的 enable。 */
export function parseAdminBoolean(source) {
  const start = source.indexOf("export function defaultDecoConfig");
  if (start < 0) return null;
  const match = /charm\s*:\s*\{[\s\S]*?enable\s*:\s*(true|false)/u.exec(source.slice(start));
  return match ? match[1] === "true" : null;
}

/**
 * 解析后端默认种子 `site_default_settings.yaml` 的 `deco.charm` 子块。
 *
 * 只认缩进结构（deco: → charm: → 键），跳过注释；这样 `site`/`ai` 段里的
 * `enable` 不会被误读成挂件的总开关。返回数值的 `default` 与布尔默认值。
 */
export function parseDecoSeed(source) {
  const numbers = {};
  let enable = null;
  let decoIndent = -1;
  let charmIndent = -1;

  for (const raw of source.split(/\r?\n/u)) {
    const withoutComment = raw.replace(/#.*$/u, "");
    if (!withoutComment.trim()) continue;
    const indent = withoutComment.length - withoutComment.trimStart().length;
    const text = withoutComment.trim();

    if (text === "deco:") {
      decoIndent = indent;
      charmIndent = -1;
      continue;
    }
    if (decoIndent < 0) continue;
    if (indent <= decoIndent) {
      decoIndent = -1;
      charmIndent = -1;
      continue;
    }
    if (text === "charm:") {
      charmIndent = indent;
      continue;
    }
    if (charmIndent < 0 || indent <= charmIndent) continue;

    const match = /^([a-z0-9_]+)\s*:\s*(.*)$/u.exec(text);
    if (!match) continue;
    const [, key, value] = match;
    if (key === "enable") {
      if (value === "true") enable = true;
      else if (value === "false") enable = false;
      continue;
    }
    const parsed = Number(value);
    if (Number.isFinite(parsed)) numbers[key] = parsed;
  }

  const parameters = {};
  for (const [key, value] of Object.entries(numbers)) parameters[key] = { default: value };
  return { parameters, boolean: enable };
}

/** 参数值的可读形式：布尔写 true/false，数字去掉浮点尾巴。 */
function displayValue(value) {
  return typeof value === "boolean" ? String(value) : String(Number(value));
}

/**
 * 构造一条"哪两端、哪个参数、差在哪"的失败记录。
 *
 * 取"多数值"为参照组：只改一端时，另一端 + 种子会形成多数，少数就是肇事端；
 * 三方各不同时按源文件顺序取第一个为参照。无论哪种，message 都列全每一端的
 * 取值，不让人再猜。
 */
function buildMismatch({ parameter, field, values, order }) {
  const present = order.filter((side) => values[side] !== undefined);
  const groups = new Map();
  for (const side of present) {
    const value = values[side];
    const group = groups.get(value);
    if (group) group.sides.push(side);
    else groups.set(value, { value, sides: [side] });
  }
  if (groups.size <= 1) return null;

  const grouped = [...groups.values()];
  const majority = grouped.reduce((best, group) => {
    if (group.sides.length > best.sides.length) return group;
    if (group.sides.length < best.sides.length) return best;
    const groupIndex = order.indexOf(group.sides[0]);
    const bestIndex = order.indexOf(best.sides[0]);
    return groupIndex < bestIndex ? group : best;
  });
  const minoritySides = present.filter((side) => values[side] !== majority.value);
  const entries = present
    .map((side) => `${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]} = ${displayValue(values[side])}`)
    .join("；");

  const message =
    `[${parameter}.${field}] ${FIELD_LABELS[field]}不一致：` +
    `${entries}。` +
    `${minoritySides.map((side) => SIDE_LABELS[side]).join(" / ")} 与 ` +
    `${majority.sides.map((side) => SIDE_LABELS[side]).join(" / ")} 不一致 —— ` +
    `请把同一取值同步到每一端。`;

  return {
    kind: "value-mismatch",
    parameter,
    field,
    values: { ...values },
    sides: { ...values },
    majoritySides: [...majority.sides],
    disagreeingSides: minoritySides,
    message,
  };
}

/**
 * 纯判定函数：收四份源文件的文本，断言同一参数在每一端的 min/max/default 一致。
 *
 * 参数名不是写死的清单，而是从四份源文件里发现出来的并集；某一端少了一个
 * 参数会报 `missing-value`，而不是因为"没读到"就跳过。
 */
export function evaluateDecoRangeParity({ backend, yaml, reader, admin }) {
  const backendResult = parseBackendRanges(backend);
  const readerResult = parseReaderRanges(reader);
  const adminResult = parseAdminRanges(admin);
  const parsed = {
    backend: { parameters: backendResult.parameters },
    reader: { parameters: readerResult.parameters },
    admin: { parameters: adminResult.parameters, boolean: parseAdminBoolean(admin) },
    yaml: parseDecoSeed(yaml),
  };

  const failures = [
    ...backendResult.failures,
    ...backendResult.duplicates,
    ...readerResult.failures,
    ...readerResult.duplicates,
    ...adminResult.failures,
    ...adminResult.duplicates,
  ];

  for (const side of CODE_SIDES) {
    if (Object.keys(parsed[side].parameters).length === 0) {
      failures.push({
        kind: "source-unreadable",
        side,
        path: DECO_SOURCE_FILES[side],
        message: `${DECO_SOURCE_FILES[side]} 里一个装饰参数都没解析到：源文件格式变了或命名不再符合约定`,
      });
    }
  }

  const keys = new Set();
  for (const side of DEFAULT_SIDES) {
    for (const key of Object.keys(parsed[side].parameters)) keys.add(key);
  }

  const parameters = {};
  for (const key of [...keys].sort((left, right) => left.localeCompare(right))) {
    parameters[key] = {};
    for (const side of DEFAULT_SIDES) {
      const slot = parsed[side].parameters[key];
      if (slot) parameters[key][side] = slot;
    }

    for (const field of ["min", "max", "default"]) {
      const expectedSides = field === "default" ? DEFAULT_SIDES : CODE_SIDES;
      const values = {};
      for (const side of expectedSides) {
        const slot = parsed[side].parameters[key];
        if (!slot || slot[field] === undefined) {
          failures.push({
            kind: "missing-value",
            parameter: `charm.${key}`,
            field,
            side,
            path: DECO_SOURCE_FILES[side],
            message:
              `[charm.${key}.${field}] ${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]} 没有这个取值：` +
              `三端必须都定义 ${key} 的${FIELD_LABELS[field]}`,
          });
          continue;
        }
        values[side] = slot[field];
      }
      const mismatch = buildMismatch({
        parameter: `charm.${key}`,
        field,
        values,
        order: expectedSides,
      });
      if (mismatch) failures.push(mismatch);
    }
  }

  const booleanDefault = {
    yaml: parsed.yaml.boolean,
    admin: parsed.admin.boolean,
  };
  const booleanValues = {};
  for (const side of BOOLEAN_SIDES) {
    if (booleanDefault[side] === null || booleanDefault[side] === undefined) {
      failures.push({
        kind: "missing-value",
        parameter: "charm.enable",
        field: "default",
        side,
        path: DECO_SOURCE_FILES[side],
        message:
          `[charm.enable.default] ${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]} 没有布尔默认值：` +
          "总开关默认必须与后端种子一致",
      });
      continue;
    }
    booleanValues[side] = booleanDefault[side];
  }
  const booleanMismatch = buildMismatch({
    parameter: "charm.enable",
    field: "default",
    values: booleanValues,
    order: BOOLEAN_SIDES,
  });
  if (booleanMismatch) failures.push(booleanMismatch);

  return {
    ok: failures.length === 0,
    failures,
    parameters,
    booleanDefault,
  };
}

/** 读真实文件并判定。任一源文件缺失都算失败，不能把"没读到"当成"两端都空"。 */
export function checkDecoRangeParity({ repoRoot = findRepoRoot() } = {}) {
  const contents = {};
  const failures = [];

  for (const [side, relativePath] of Object.entries(DECO_SOURCE_FILES)) {
    const fullPath = join(repoRoot, relativePath);
    if (!existsSync(fullPath)) {
      failures.push({
        kind: "file-missing",
        side,
        path: relativePath,
        message: `找不到 ${relativePath}（仓库根 ${repoRoot}）：跨端参数一致性门禁无法判定`,
      });
      continue;
    }
    contents[side] = readFileSync(fullPath, "utf8");
  }

  if (failures.length > 0) {
    return {
      ok: false,
      failures,
      parameters: {},
      booleanDefault: {},
      files: {},
    };
  }

  const result = evaluateDecoRangeParity({
    backend: contents.backend,
    yaml: contents.yaml,
    reader: contents.reader,
    admin: contents.admin,
  });

  return {
    ...result,
    files: Object.fromEntries(
      Object.entries(contents).map(([side, text]) => [side, Buffer.byteLength(text, "utf8")]),
    ),
  };
}

function printReport(result, repoRoot) {
  console.log(`跨端装饰参数（仓库根：${repoRoot}）`);
  console.log(`参数 | 字段 | 后端 | 读者站 | 后台 | yaml 种子`);
  for (const [key, slots] of Object.entries(result.parameters)) {
    for (const field of ["min", "max", "default"]) {
      const cells = ["backend", "reader", "admin", "yaml"].map((side) => {
        const value = slots[side]?.[field];
        return value === undefined ? "—" : displayValue(value);
      });
      console.log(`charm.${key} | ${field} | ${cells.join(" | ")}`);
    }
  }
  if (result.failures.length > 0) {
    console.error("当前判定：不一致");
    for (const failure of result.failures) console.error(`  ${failure.message}`);
  } else {
    console.log("当前判定：一致");
  }
}

function runCli(argv) {
  const report = argv.includes("--report");
  const repoRoot = findRepoRoot();
  const result = checkDecoRangeParity({ repoRoot });
  const parameterCount = Object.keys(result.parameters).length;

  if (report) {
    printReport(result, repoRoot);
    return 0;
  }

  if (result.ok) {
    console.log(
      `✓ 跨端装饰参数一致性门禁通过（${parameterCount} 个数值参数 × min/max/default + ` +
        "布尔默认值（后端种子 ↔ 后台恢复默认），三端逐项一致）",
    );
    return 0;
  }

  console.error("✗ 跨端装饰参数一致性门禁失败");
  for (const failure of result.failures) console.error(`  ${failure.message}`);
  return 1;
}

if (process.argv[1] && /check-deco-range-parity\.mjs$/u.test(process.argv[1])) {
  process.exit(runCli(process.argv.slice(2)));
}
