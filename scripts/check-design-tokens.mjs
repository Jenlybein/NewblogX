// 「设计 token 漂移」门禁（两端前端共用）。
//
// 为什么需要它（真实教训）：颜色一旦允许在组件里随手写裸字面量，就会重新长回
// "三处同步"——`:root` token、字面 `rgba()`、Naive `themeOverrides` 各写一遍，
// 之后每次调色都漏一处。spec（issue #1 决策 8）要求颜色只有一个来源：语义 token 层。
//
// 规则：扫描 `<project>/app/**` 的 `.vue` / `.css`，除下面 EXEMPT_PATHS 里列出的
// token 文件、品牌矢量与 OG 生成物之外，禁止出现裸十六进制色值或 `rgb()/rgba()/
// hsl()/hsla()` 字面量。违例直接失败，并打印 文件:行号。
//
// baseline 是**允许文件清单**（存量债），规则只有一条：**只能收缩**。
//   - 清单外的文件出现裸色值 → 失败（新增违规不许再进历史债）；
//   - 清单内的文件被清干净了却还留在清单里 → 失败（逼着删掉，否则清单永远不缩）；
//   - 清单里的文件已不存在 → 失败（防止清单腐化）。
//
// 门禁自带"人造违规"用例（见 frontend/test/gates/design-tokens.test.ts 与
// frontend_admin/test/unit/design-tokens.test.ts）：喂一个合成违规文件断言它会红，
// 再喂一个合规文件断言它会绿。没有这两条，门禁可能永远是绿的、等于不存在。
//
// 用法：node scripts/check-design-tokens.mjs [项目名] [--report]
//       项目名：frontend / frontend_admin（缺省两端都查）
//       --report：只打印存量违例清单（维护 baseline 用，不做判定）
// 退出码：0 = 无新增违规且清单可收缩性成立；1 = 门禁失败
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { basename, dirname, join, relative, resolve, sep } from "node:path";

export const BASELINE_FILENAME = "design-token-baseline.json";
export const PROJECTS = ["frontend", "frontend_admin"];

// 为什么不用 `fileURLToPath(import.meta.url)` 定位仓库根：这个模块同时被 vitest
// 导入，而 vitest（Vite）会把 import.meta.url 改写成非 file 协议的地址，
// `fileURLToPath` 会抛 "The URL must be of scheme file"。改成从项目目录向上找
// baseline 文件，既能被测试用，也能被 CLI 用，且不依赖 cwd 的具体位置。
export function findRepoRoot(fromDir) {
  let dir = resolve(fromDir);
  for (;;) {
    if (existsSync(join(dir, "scripts", BASELINE_FILENAME))) return dir;
    const parent = dirname(dir);
    if (parent === dir) return resolve(fromDir);
    dir = parent;
  }
}

export function baselineFileFor(projectDir) {
  return join(findRepoRoot(projectDir), "scripts", BASELINE_FILENAME);
}

// 扫描范围之外的文件/目录。**新增豁免必须在这里写理由**：
//   - token 文件：唯一允许定义颜色字面量的地方（原始层 + 语义层都在这）；
//   - 品牌矢量与 OG 生成物：品牌资产色（logo 蓝 #1E6FE8）不属于五色语义体系，
//     OG 预览图是构建期生成的，都不参与漂移判定。
export const EXEMPT_PATHS = [
  { path: "app/assets/css/tokens.css", reason: "设计 token 文件：颜色字面量的唯一合法定义处" },
  { path: "app/assets/brand/", reason: "品牌矢量与字标（品牌资产色不属于语义 token）" },
  { path: "app/assets/og/", reason: "社交预览（OG）生成物" },
];

// 裸色值：3/4/6/8 位十六进制（后面不能再跟标识符字符，否则 `#abc` 之类的
// CSS ID 选择器会误报），以及 rgb()/rgba()/hsl()/hsla() 函数调用。
// 注意 rgb()/rgba() 前面不能加 `\b`：Tailwind 任意值里它们常跟在 `_` 后面
// （`shadow-[inset_0_1px_0_rgba(255,255,255,0.65)]`），而 `\b` 在 `_` 与 `r`
// 之间并不成立 —— 加了会漏掉一整类真实违规。
const COLOR_LITERAL_RE =
  /#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})(?![0-9a-zA-Z_-])|(?:rgba?|hsla?)\([^)]*\)/gu;

const SCANNED_EXTENSIONS = [".vue", ".css"];
const SKIPPED_DIRS = new Set(["node_modules", ".nuxt", ".output", "coverage"]);

export function isExemptPath(relPath) {
  const normalized = relPath.split(sep).join("/");
  return EXEMPT_PATHS.some(({ path }) => normalized === path || normalized.startsWith(path));
}

/** 找出源码里的裸颜色字面量，返回 `{ line, match }`（行号 1-based，按出现顺序）。 */
export function findColorLiterals(source) {
  const found = [];
  for (const match of source.matchAll(COLOR_LITERAL_RE)) {
    const before = source.slice(0, match.index);
    const line = before.split(/\r?\n/u).length;
    found.push({ line, match: match[0].replace(/\s+/gu, " ").trim() });
  }
  return found;
}

/** 扫描 `{ file, text }[]`，聚合成违例列表。 */
export function scanEntries(entries) {
  const violations = [];
  for (const entry of entries) {
    for (const literal of findColorLiterals(entry.text)) {
      violations.push({ file: entry.file, line: literal.line, match: literal.match });
    }
  }
  return violations;
}

function walk(dir, files = []) {
  let names;
  try {
    names = readdirSync(dir);
  } catch {
    return files;
  }
  for (const name of names) {
    const full = join(dir, name);
    const info = statSync(full);
    if (info.isDirectory()) {
      if (SKIPPED_DIRS.has(name)) continue;
      walk(full, files);
      continue;
    }
    if (SCANNED_EXTENSIONS.some((extension) => name.endsWith(extension))) files.push(full);
  }
  return files;
}

/** 读取一个前端项目的扫描对象（`app/**` 下的 .vue/.css，豁免文件已剔除），按路径排序。 */
export function readProjectEntries(projectDir) {
  const entries = [];
  for (const file of walk(join(projectDir, "app"))) {
    const relPath = relative(projectDir, file).split(sep).join("/");
    if (isExemptPath(relPath)) continue;
    entries.push({ file: relPath, text: readFileSync(file, "utf8") });
  }
  return entries.sort((a, b) => a.file.localeCompare(b.file));
}

export function readBaselineFile(projectDir) {
  return JSON.parse(readFileSync(baselineFileFor(projectDir), "utf8"));
}

/**
 * 读 baseline 允许清单。
 * @param {string} [project] 项目目录（`<repo>/frontend` 之类）；省略则返回整份 baseline。
 */
export function loadBaseline(project) {
  if (!project) return readBaselineFile(process.cwd());
  const name = basename(project);
  const baseline = readBaselineFile(project);
  const files = baseline[name];
  if (!Array.isArray(files)) {
    throw new Error(`baseline 里没有 "${name}" 这一节：${baselineFileFor(project)}`);
  }
  return files;
}

/**
 * 棘轮判定（纯函数，便于用人造数据断言门禁会红）。
 * @param {{ violations: {file: string, line: number, match: string}[], baselineFiles: string[], scannedFiles: string[] }} input
 * @returns {{ ok: boolean, failures: {kind: string, file: string, line?: number, match?: string}[] }}
 */
export function evaluateDrift({ violations, baselineFiles, scannedFiles }) {
  const allowed = new Set(baselineFiles);
  const scanned = new Set(scannedFiles);
  const dirty = new Set(violations.map((violation) => violation.file));
  const failures = [];

  for (const violation of violations) {
    if (allowed.has(violation.file)) continue;
    failures.push({
      kind: "new-color-literal",
      file: violation.file,
      line: violation.line,
      match: violation.match,
    });
  }

  for (const file of baselineFiles) {
    if (!scanned.has(file)) {
      failures.push({ kind: "baseline-file-missing", file });
      continue;
    }
    if (!dirty.has(file)) {
      failures.push({ kind: "baseline-should-shrink", file });
    }
  }

  return { ok: failures.length === 0, failures };
}

/** 对一个项目跑完整门禁：扫描 → 判定。 */
export function checkProject({ projectDir, baselineFiles }) {
  const entries = readProjectEntries(projectDir);
  const violations = scanEntries(entries);
  const scannedFiles = entries.map((entry) => entry.file);
  return { entries, violations, scannedFiles, ...evaluateDrift({ violations, baselineFiles, scannedFiles }) };
}

function formatFailure(failure) {
  if (failure.kind === "new-color-literal") {
    return `  ✗ ${failure.file}:${failure.line} 出现裸颜色字面量 "${failure.match}"（改用语义 token：var(--color-*)）`;
  }
  if (failure.kind === "baseline-should-shrink") {
    return `  ✗ ${failure.file} 已经没有裸颜色字面量，请从 scripts/design-token-baseline.json 删除该条目（清单只许收缩）`;
  }
  if (failure.kind === "baseline-file-missing") {
    return `  ✗ scripts/design-token-baseline.json 里登记的 ${failure.file} 不存在，请删除这条过期条目`;
  }
  return `  ✗ ${failure.kind}: ${failure.file}`;
}

function runCli(argv) {
  const report = argv.includes("--report");
  const names = argv.filter((arg) => !arg.startsWith("--"));
  const targets = names.length > 0 ? names : PROJECTS;
  const repoRoot = findRepoRoot(process.cwd());
  let failed = false;

  for (const name of targets) {
    if (!PROJECTS.includes(name)) {
      console.error(`✗ 未知项目 "${name}"，可选：${PROJECTS.join(" / ")}`);
      return 1;
    }
    const projectDir = join(repoRoot, name);
    const baselineFiles = loadBaseline(projectDir);
    const result = checkProject({ projectDir, baselineFiles });

    if (report) {
      console.log(`${name}：${result.violations.length} 处存量裸色值 / ${result.scannedFiles.length} 个文件`);
      for (const violation of result.violations) {
        console.log(`  ${violation.file}:${violation.line}  ${violation.match}`);
      }
      continue;
    }

    if (result.ok) {
      console.log(
        `✓ 设计 token 漂移门禁通过（${name}：${result.scannedFiles.length} 个文件，` +
          `其中 ${baselineFiles.length} 个在 baseline 清单内）`,
      );
      continue;
    }
    failed = true;
    console.error(`✗ ${name}：设计 token 漂移门禁失败`);
    for (const failure of result.failures) {
      console.error(formatFailure(failure));
    }
  }

  return failed ? 1 : 0;
}

if (process.argv[1] && basename(process.argv[1]) === "check-design-tokens.mjs") {
  process.exit(runCli(process.argv.slice(2)));
}
