// 「跨端装饰参数一致性」门禁（#43；#72 升级为键集合派生 + 锚点清单 + 负值支持）。
//
// 为什么必须有它（票 #35 用户故事 36/37）：`deco.charm` 的每个数值参数（外加一个
// 布尔总开关）都有**四份独立定义**：
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
// 有人只改了一端。数值字面量允许符号 —— 负区间（如指针端偏移的 `-24..24`）是
// 合法取值，不是解析失败（ADR-0018 决策 9）。
//
// 刻意**不**维护第三张权威参数表，也**不**写死参数个数（#72）：参数名从四份源文件
// 里**发现**（Go 的 `CharmXxxMin`、读者站 `CHARM_XXX_MIN`、后台 `DECO_XXX_MIN`
// 按命名约定归一成 `xxx`），判定只有两条：
//   1. **四源键集合相等**：某端整块少一个参数、或多出一个别端没有的参数，都报红并
//      点名是哪一端、哪个参数（`key-set-mismatch`）；
//   2. **锚点参数在场**：`rope_length` / `fps_cap` / `pointer_offset_x` /
//      `pointer_offset_y`（`DECO_ANCHOR_PARAMETERS`）。键集合相等判不出"参数从四端
//      同时消失"，锚点负责兜住；尚未实现的锚点在四端都不存在时不算失败（见该常量
//      与 `evaluateAnchors` 的注释）。
//
// canary 由两处保证它会红：
//   1. `frontend/test/gates/deco-range-parity.test.ts`：在内存里只改一端的文本，
//      断言失败并点名"哪两端、哪个参数、差在哪"（含负值、缺键、锚点丢失）；
//   2. `scripts/verify-gates.sh`：在真实仓库里故意只改后台一个常量（含一个负值），
//      断言门禁红。
// 只有"真实仓库为绿 + 人造不一致为红"同时成立，这道门禁才算存在。
//
// 用法：node scripts/check-deco-range-parity.mjs [--report]
//   --report：打印每个参数在每一端的取值 + 锚点现状（排查用，不做判定）
// 退出码：0 = 全部一致；1 = 有不一致 / 缺键 / 锚点丢失 / 源文件缺失
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

/** 四份源文件（相对仓库根）。`yaml` 同时是默认种子与参数完整性的键集。 */
export const DECO_SOURCE_FILES = {
  backend: "backend/conf/conf_deco.go",
  yaml: "backend/config/site_default_settings.yaml",
  reader: "frontend/app/utils/decoCharm.ts",
  admin: "frontend_admin/app/utils/decoParams.ts",
};

/**
 * 共享归一化用例表（相对仓库根）：同一组输入在三端必须给出同一组输出。
 *
 * 范围门禁只比较 min/max/default，抓不到"三端都定义对了范围、但取整规则不同"
 * 的漂移（segments 一端 math.Round、一端直接截断；line_width 一端 math.Round、
 * 一端 toFixed —— 半值结果不同）。这张表把归一化本身也钉成同一份契约。
 */
export const DECO_FIXTURE_FILE = "backend/conf/testdata/deco-charm-normalization.json";

/** 夹具文件名：Go 测试按包内相对路径（`testdata/...`）引用，比对文件名即可。 */
export const DECO_FIXTURE_BASENAME = "deco-charm-normalization.json";

/** 必须消费共享夹具的三套测试（少一套 = 那一端可以静默漂移）。 */
export const DECO_FIXTURE_CONSUMERS = [
  "backend/conf/conf_deco_test.go",
  "frontend/test/utils/decoCharm.test.ts",
  "frontend_admin/test/unit/decoParams.test.ts",
];

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

/**
 * Go 的驼峰参数名 → 跨端统一键：`LogoSize` → `logo_size`，`IdleSwayHz` → `idle_sway_hz`。
 *
 * 缩写也要能被拆开（#72）：Go 习惯把 `FPS` 整段大写，朴素实现会得到 `f_p_s_cap`，
 * 与 yaml 的 `fps_cap` 撞不上，于是锚点报"缺参数"却没人看得懂原因。这里先按
 * "大写串 + 大写+小写" 断一次（`FPSCap` → `FPS_Cap`），再按"小写/数字 + 大写"断。
 */
function snakeKey(camel) {
  return camel
    .replace(/([A-Z]+)([A-Z][a-z0-9])/gu, "$1_$2")
    .replace(/([a-z0-9])([A-Z])/gu, "$1_$2")
    .toLowerCase();
}

/**
 * 数值字面量：允许符号（#72）。
 *
 * 不带符号时，`pointer_offset_x.min = -24` 这种源文件会**整行匹配不上**，于是该
 * 取值被静默跳过、后续报 `missing-value` —— 看起来像"某端忘了定义"，实际是
 * "门禁不认识负数"。允许负区间是 ADR-0018 决策 9 的硬要求。
 */
const NUMBER_TOKEN = String.raw`[+-]?[0-9]+(?:\.[0-9]*)?(?:[eE][+-]?[0-9]+)?`;

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
    const match = new RegExp(
      String.raw`^Charm([A-Z][A-Za-z0-9]*)(Min|Max|Default)\s*=\s*(${NUMBER_TOKEN})`,
      "u",
    ).exec(raw.trim());
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
    const match = new RegExp(
      String.raw`^export const CHARM_([A-Z0-9_]+)_(MIN|MAX|DEFAULT)\s*=\s*(${NUMBER_TOKEN})`,
      "u",
    ).exec(raw.trim());
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
    const match = new RegExp(
      String.raw`^export const DECO_([A-Z0-9_]+)_(MIN|MAX|DEFAULT)\s*=\s*(${NUMBER_TOKEN})`,
      "u",
    ).exec(raw.trim());
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
 * 键集合判定（#72）：四份源文件必须拥有**同一组参数键**。
 *
 * 为什么不能再靠"参数总数"：写死一个 8 既要每加参数改一串数字，又只能在
 * "某处少了一个"时红，而任何"数字对得上、名字对不上"的漂移（一端改名、一端
 * 加了一个别端没有的参数）它都看不见。
 *
 * 参照组取"签名相同的端里最多的那一组"：只改一端时另外几端形成多数，漂移端
 * 会同时报缺键与多键；平票时按源文件顺序取第一个（与 buildMismatch 同一口径）。
 */
function buildKeySetFailures(sideKeys) {
  const failures = [];
  const signatures = DEFAULT_SIDES.map((side) => [...sideKeys[side]].sort().join(","));
  const groups = new Map();
  DEFAULT_SIDES.forEach((side, index) => {
    const signature = signatures[index];
    const group = groups.get(signature);
    if (group) group.sides.push(side);
    else groups.set(signature, { sides: [side], signature });
  });
  if (groups.size <= 1) return failures;

  const grouped = [...groups.values()];
  const reference = grouped.reduce((best, group) => {
    if (group.sides.length > best.sides.length) return group;
    if (group.sides.length < best.sides.length) return best;
    return DEFAULT_SIDES.indexOf(group.sides[0]) < DEFAULT_SIDES.indexOf(best.sides[0])
      ? group
      : best;
  });
  const referenceKeys = new Set(reference.signature.split(",").filter(Boolean));

  for (const side of DEFAULT_SIDES) {
    if (reference.sides.includes(side)) continue;
    const own = sideKeys[side];
    const missingKeys = [...referenceKeys].filter((key) => !own.has(key)).sort();
    const extraKeys = [...own].filter((key) => !referenceKeys.has(key)).sort();
    if (missingKeys.length === 0 && extraKeys.length === 0) continue;

    const parts = [];
    if (missingKeys.length > 0) parts.push(`缺少 ${missingKeys.map((key) => `charm.${key}`).join(" / ")}`);
    if (extraKeys.length > 0) parts.push(`多出 ${extraKeys.map((key) => `charm.${key}`).join(" / ")}`);
    failures.push({
      kind: "key-set-mismatch",
      side,
      path: DECO_SOURCE_FILES[side],
      missingKeys,
      extraKeys,
      referenceSides: [...reference.sides],
      message:
        `[${[...missingKeys, ...extraKeys].map((key) => `charm.${key}`).join(" / ")}] ` +
        `${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]} 的参数键集合与其他端不一致：` +
        `${parts.join("；")}（参照组：${reference.sides.map((item) => SIDE_LABELS[item]).join(" / ")}）。` +
        "四源键集合必须相等 —— 缺参数或多参数都会让那一端读不到／多写一个取值。",
    });
  }
  return failures;
}

/**
 * 锚点参数清单（#72，ADR-0018 决策 10）：拦住"某个参数从四份源文件里同时消失"。
 *
 * 为什么键集合相等不够：键集合判的是"四端彼此一样"。真要有人把一个参数从四端
 * 一起删干净，四份键集合还是相等的，门禁照样全绿。锚点就是那几张"这些参数必须
 * 在场"的票根。
 *
 * 为什么还要 `status`：清单里曾经含**将来才会存在**的参数（`fps_cap` 属 #74、
 * `pointer_offset_*` 属 #75）。如果只认"必须在场"，那些票落地之前门禁就会因为
 * 这几个还没实现而变红，等于用门禁挡住正常交付顺序。所以：
 *   - `pending`（尚未实现）：四端都没有 → **不算失败**，只在报告里列为"待交付"；
 *   - `delivered`（已实现）：四端都没有 → **失败**（`anchor-missing`）；
 *   - 两者共同的硬约束：只要在任意一端出现，四端就必须都有 —— 交付到一半是
 *     漂移，由键集合判定报红（不会因为"它是新参数"被放过）。
 * 现在四个锚点全部 `delivered`（#74 / #75 都已落地），`status` 仍留着：下一批
 * 参数（比如将来的第二个装饰子块）还会用到它，而"交付那一笔把 status 改成
 * `delivered`"这条纪律正是它挡住"门禁自己变红"的方式。
 */
export const DECO_ANCHOR_PARAMETERS = [
  { key: "rope_length", label: "挂绳长度", status: "delivered" },
  // #74 已交付：四端都有 `fps_cap`，状态位随之转正（否则报告会一直提示"待交付"）。
  { key: "fps_cap", label: "帧率上限", status: "delivered", issue: "#74" },
  // #75 已交付：四端都有指针端偏移（负数下限也由门禁的符号支持覆盖）。
  { key: "pointer_offset_x", label: "指针端偏移 X", status: "delivered", issue: "#75" },
  { key: "pointer_offset_y", label: "指针端偏移 Y", status: "delivered", issue: "#75" },
];

const EMPTY_ANCHORS = { delivered: [], pending: [], partial: [], notes: [] };

/**
 * 锚点判定（#72）：返回失败记录与"锚点现状"。
 *
 * 三种局面必须分得开，否则不是误红就是漏红：
 *   1. 四端都有 → 普通参数（值一致性由主判定负责），记进 delivered；
 *   2. 只有部分端有 → 交付到一半，键集合判定会报 key-set-mismatch，这里只记账，
 *      不重复刷同一条红灯；
 *   3. 四端都没有 → 看 status：`delivered` 是**丢了**（失败），`pending` 是
 *      **还没交付**（不是失败）。
 */
function evaluateAnchors(sideKeys) {
  const failures = [];
  const anchors = { delivered: [], pending: [], partial: [], notes: [] };

  for (const anchor of DECO_ANCHOR_PARAMETERS) {
    const presentSides = DEFAULT_SIDES.filter((side) => sideKeys[side]?.has(anchor.key));

    if (presentSides.length === DEFAULT_SIDES.length) {
      anchors.delivered.push(anchor.key);
      if (anchor.status !== "delivered") {
        anchors.notes.push(
          `charm.${anchor.key}（${anchor.label}）四端都已交付，但锚点清单里仍标 ${anchor.status}` +
            "（预计 " +
            (anchor.issue ?? "后续票") +
            "）：交付那一笔提交里应把 status 改成 delivered。",
        );
      }
      continue;
    }

    if (presentSides.length > 0) {
      anchors.partial.push(anchor.key);
      continue;
    }

    if (anchor.status === "delivered") {
      failures.push({
        kind: "anchor-missing",
        parameter: `charm.${anchor.key}`,
        missingSides: [...DEFAULT_SIDES],
        message:
          `[charm.${anchor.key}] 锚点参数（${anchor.label}）在四份源文件里都不存在：` +
          DEFAULT_SIDES.map((side) => `${SIDE_LABELS[side]} ${DECO_SOURCE_FILES[side]}`).join("、") +
          " 全都没有它。四源键集合仍然「相等」，所以只有锚点清单拦得住这种「参数从四端同时被删」的漂移；" +
          "确实要删它，就得先把它从锚点清单里摘掉并说明理由。",
      });
      continue;
    }

    anchors.pending.push(anchor.key);
  }

  return { failures, anchors };
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
  const sideKeys = {};
  for (const side of DEFAULT_SIDES) {
    sideKeys[side] = new Set(Object.keys(parsed[side].parameters));
    for (const key of sideKeys[side]) keys.add(key);
  }

  // 键集合相等：整块缺参数 / 多参数在这里一次说清（见 buildKeySetFailures）。
  // 一个参数都没解析到的端已经报过 source-unreadable，不再重复列全部键。
  const readableSideKeys = {};
  for (const side of DEFAULT_SIDES) {
    if (Object.keys(parsed[side].parameters).length === 0) continue;
    readableSideKeys[side] = sideKeys[side];
  }
  if (Object.keys(readableSideKeys).length >= 2) {
    failures.push(...buildKeySetFailures(readableSideKeys));
  }

  // 锚点参数：拦住"键集合仍然相等、但参数从四端同时消失"（见 DECO_ANCHOR_PARAMETERS）。
  const anchorResult = evaluateAnchors(sideKeys);
  failures.push(...anchorResult.failures);

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
        // 整个参数都不在这一端：已由 key-set-mismatch 点名，别再刷三条 missing-value。
        if (!slot) continue;
        if (slot[field] === undefined) {
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
    anchors: anchorResult.anchors,
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
      anchors: { ...EMPTY_ANCHORS },
      files: {},
      fixture: { caseCount: 0, parameters: [], consumers: [] },
    };
  }

  const result = evaluateDecoRangeParity({
    backend: contents.backend,
    yaml: contents.yaml,
    reader: contents.reader,
    admin: contents.admin,
  });

  // 共享归一化夹具：文件必须存在，且三套测试都必须引用它。
  const fixturePath = join(repoRoot, DECO_FIXTURE_FILE);
  const fixtureText = existsSync(fixturePath) ? readFileSync(fixturePath, "utf8") : null;
  const consumerTexts = {};
  for (const relativePath of DECO_FIXTURE_CONSUMERS) {
    const fullPath = join(repoRoot, relativePath);
    consumerTexts[relativePath] = existsSync(fullPath) ? readFileSync(fullPath, "utf8") : null;
  }
  const fixtureResult = evaluateDecoFixture({ fixture: fixtureText, consumers: consumerTexts });

  // 夹具必须覆盖每一个被门禁发现的参数，不能只留几行应付检查。
  const fixtureFailures = [...fixtureResult.failures];
  for (const key of Object.keys(result.parameters)) {
    if (!fixtureResult.parameters.includes(key)) {
      fixtureFailures.push({
        kind: "fixture-coverage",
        parameter: `charm.${key}`,
        path: DECO_FIXTURE_FILE,
        message: `[charm.${key}] 共享归一化用例表 ${DECO_FIXTURE_FILE} 没有这个参数的用例：三端取整规则可以静默漂移`,
      });
    }
  }

  const allFailures = [...result.failures, ...fixtureFailures];
  return {
    ...result,
    ok: result.ok && allFailures.length === 0,
    failures: allFailures,
    files: Object.fromEntries(
      Object.entries(contents).map(([side, text]) => [side, Buffer.byteLength(text, "utf8")]),
    ),
    fixture: {
      caseCount: fixtureResult.caseCount,
      parameters: fixtureResult.parameters,
      consumers: DECO_FIXTURE_CONSUMERS,
    },
  };
}

/**
 * 判定共享归一化夹具：文件必须存在、是合法 JSON、用例字段齐全，且三套测试
 * 的源码里真的出现夹具路径（"存在但没人读"等于没有）。
 *
 * 纯函数收文件文本，便于 canary：删文件 / 把某套测试里的引用拿掉，都必须红。
 */
export function evaluateDecoFixture({ fixture, consumers }) {
  const failures = [];
  let caseCount = 0;
  const parameters = [];

  if (fixture === null || fixture === undefined) {
    failures.push({
      kind: "fixture-missing",
      path: DECO_FIXTURE_FILE,
      message: `找不到共享归一化用例表 ${DECO_FIXTURE_FILE}：三端归一化规则没有共同的期望值`,
    });
  } else {
    let parsed = null;
    try {
      parsed = JSON.parse(fixture);
    } catch (error) {
      failures.push({
        kind: "fixture-invalid",
        path: DECO_FIXTURE_FILE,
        message: `共享归一化用例表 ${DECO_FIXTURE_FILE} 不是合法 JSON：${error.message}`,
      });
    }
    if (parsed) {
      const cases = Array.isArray(parsed.cases) ? parsed.cases : null;
      if (!cases || cases.length === 0) {
        failures.push({
          kind: "fixture-empty",
          path: DECO_FIXTURE_FILE,
          message: `共享归一化用例表 ${DECO_FIXTURE_FILE} 没有 cases：三端夹具失效`,
        });
      } else {
        caseCount = cases.length;
        for (const [index, testCase] of cases.entries()) {
          if (
            !testCase ||
            typeof testCase.parameter !== "string" ||
            testCase.input === undefined ||
            typeof testCase.expected !== "number"
          ) {
            failures.push({
              kind: "fixture-case-invalid",
              path: DECO_FIXTURE_FILE,
              message:
                `共享归一化用例表 ${DECO_FIXTURE_FILE} 第 ${index + 1} 行缺少 ` +
                "parameter / input / expected，或 expected 不是数字",
            });
            continue;
          }
          if (!parameters.includes(testCase.parameter)) parameters.push(testCase.parameter);
        }
      }
    }
  }

  for (const relativePath of DECO_FIXTURE_CONSUMERS) {
    const text = consumers?.[relativePath];
    if (text === null || text === undefined) {
      failures.push({
        kind: "fixture-consumer-missing",
        path: relativePath,
        message: `找不到共享归一化用例表的消费方 ${relativePath}：那一端的归一化规则不再被夹具约束`,
      });
      continue;
    }
    if (!text.includes(DECO_FIXTURE_BASENAME)) {
      failures.push({
        kind: "fixture-unreferenced",
        path: relativePath,
        message:
          `${relativePath} 没有引用共享归一化用例表 ${DECO_FIXTURE_FILE}：` +
          "该测试套件可以静默停用夹具而不被发现",
      });
    }
  }

  return { ok: failures.length === 0, failures, caseCount, parameters };
}

/** 锚点现状的一句话摘要（#72）：报告与成功提示共用，免得两处口径不同。 */
function anchorSummary(anchors) {
  const parts = [];
  if (anchors.delivered.length > 0) parts.push(`已交付 ${anchors.delivered.join(" / ")}`);
  if (anchors.pending.length > 0) parts.push(`待交付 ${anchors.pending.join(" / ")}`);
  if (anchors.partial.length > 0) parts.push(`交付不完整 ${anchors.partial.join(" / ")}`);
  return parts.length > 0 ? parts.join("；") : "无";
}

function printReport(result, repoRoot) {
  console.log(`跨端装饰参数（仓库根：${repoRoot}）`);
  console.log(`共享归一化夹具：${DECO_FIXTURE_FILE}（${result.fixture.caseCount} 条用例）`);
  console.log(`锚点参数：${anchorSummary(result.anchors)}`);
  for (const note of result.anchors.notes) console.log(`  提示：${note}`);
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
      "✓ 跨端装饰参数一致性门禁通过（四源键集合相等：本次派生 " +
        `${parameterCount} 个数值参数 × min/max/default + 布尔默认值（后端种子 ↔ 后台恢复默认）逐项一致；` +
        `锚点参数 ${anchorSummary(result.anchors)}；` +
        `共享归一化夹具 ${DECO_FIXTURE_FILE}（${result.fixture.caseCount} 条用例）` +
        "被后端 / 读者站 / 后台三套测试共同消费）",
    );
    for (const note of result.anchors.notes) console.log(`  提示：${note}`);
    return 0;
  }

  console.error("✗ 跨端装饰参数一致性门禁失败");
  for (const failure of result.failures) console.error(`  ${failure.message}`);
  return 1;
}

if (process.argv[1] && /check-deco-range-parity\.mjs$/u.test(process.argv[1])) {
  process.exit(runCli(process.argv.slice(2)));
}
