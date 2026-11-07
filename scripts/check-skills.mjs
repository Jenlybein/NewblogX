#!/usr/bin/env node
// 运行时策略 skill 门禁（#309 / ADR-0033）。
//
// 为什么需要它：`skills/` 是仓库根的 Markdown 目录，**archcheck 完全看不见**
// （它只扫 backend/**/*.go）。不补这道门禁，"策略文本写歪了"就是一片永远绿的暗礁：
// 进程启动会失败，但那要等部署之后、由最先撞上的人发现。
//
// 判定规则与后端加载器（backend/internal/platform/skills）逐条对齐：名称与目录名
// 一致、name/description 必填且长度受限、metadata 只能是 map[string]string、
// 正文非空。规范会加字段，所以**未知顶层字段一律容忍**——两处实现都不硬拒，
// 否则规范一加字段，服务和门禁会同时挡人。
//
// 为什么不在 Go 门禁里做：这道校验要能在 CI 之外的任何地方跑（改文案的人不一定装 Go）。
// 官方参考校验器 `skills-ref` 不是本仓库的依赖，这里用等价规则实现，不新增 CI 依赖；
// 规则表以 <https://agentskills.io/specification> 为准。
//
// 用法：
//   node scripts/check-skills.mjs                    # 校验仓库根 skills/
//   node scripts/check-skills.mjs --root <dir> ...   # 指定一个或多个目录
//   node scripts/check-skills.mjs --self-test        # 证明它对人造违规用例真的会红
// 退出码：0 = 全部合规；1 = 有不合规；2 = 用法错误
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/** 仓库根下的默认策略目录（与 conf.DefaultSkillsDirs 同值）。 */
export const DEFAULT_ROOT = "skills";

/** 规范硬性约束，与 Go 加载器同值（改一处必须同时改两处，负向用例两边都有）。 */
export const LIMITS = { name: 64, description: 1024, compatibility: 500 };

const NAME_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/** 从工作目录向上找到仓库根（含 scripts/check-skills.mjs 的目录）。 */
export function findRepoRoot(fromDir = process.cwd()) {
  let dir = resolve(fromDir);
  for (;;) {
    if (existsSync(join(dir, "scripts", "check-skills.mjs"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) return resolve(fromDir);
    dir = parent;
  }
}

/** 按 Unicode 码点数长度：中文一个字算一个字符，与服务端口径一致。 */
function charLength(text) {
  return [...String(text)].length;
}

/**
 * 把 SKILL.md 拆成 frontmatter 与正文。
 *
 * 只认文件开头的一对 `---`；没有 frontmatter 就不是 skill，不做"尽力猜"。
 */
export function splitFrontmatter(text) {
  const lines = String(text).replace(/^\uFEFF/u, "").split(/\r?\n/u);
  if (lines[0].trim() !== "---") {
    return { ok: false, error: "缺少 YAML frontmatter：文件必须以 --- 开头" };
  }
  for (let index = 1; index < lines.length; index += 1) {
    if (lines[index].trimEnd() === "---") {
      return {
        ok: true,
        frontmatter: lines.slice(1, index).join("\n"),
        body: lines.slice(index + 1).join("\n"),
      };
    }
  }
  return { ok: false, error: "YAML frontmatter 没有结束标记 ---" };
}

/** 去掉行尾注释：只在引号之外认 `#`。 */
function stripComment(line) {
  let quote = null;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (quote) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }
    if (char === "#") return line.slice(0, index);
  }
  return line;
}

/**
 * 解析一个 YAML 标量，只区分「字符串」与「其他」。
 *
 * 为什么按 YAML 的标量类型而不是"看起来像不像数字"：规范要求 metadata 是
 * map[string]string，`revision: 1` 在 YAML 里是整数，到 Go 那边解不进 string，
 * 门禁必须与运行期得出同一个结论。
 */
function parseScalar(raw) {
  const value = raw.trim();
  if (value === "") return { kind: "empty", value };
  if (value.startsWith("[") || value.startsWith("{")) return { kind: "collection", value };
  if (value.startsWith('"') && value.endsWith('"') && value.length >= 2) {
    const inner = value.slice(1, -1).replace(/\\"/gu, '"').replace(/\\\\/gu, "\\");
    return { kind: "string", value: inner };
  }
  if (value.startsWith("'") && value.endsWith("'") && value.length >= 2) {
    return { kind: "string", value: value.slice(1, -1).replace(/''/gu, "'") };
  }
  if (/^(?:true|false|null|~|yes|no|on|off)$/iu.test(value)) return { kind: "scalar", value };
  if (/^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?$/u.test(value)) return { kind: "scalar", value };
  return { kind: "string", value };
}

/**
 * 解析 frontmatter 的顶层字段。
 *
 * 只认 `key: value` 与 `metadata:` 下的缩进块；其它形状一律当成"不是字符串"报出来，
 * 而不是猜。未知顶层字段容忍（规范会加字段），但它们的缩进内容同样不解释。
 */
export function parseFrontmatter(text) {
  const top = new Map();
  const errors = [];
  let current = null;

  for (const rawLine of text.split(/\r?\n/u)) {
    const line = stripComment(rawLine);
    if (line.trim() === "") continue;

    if (/^\s/u.test(line)) {
      // 缩进行：属于上一个顶层键。
      if (!current) {
        errors.push(`第 ${line} 行缩进不属于任何顶层字段`);
        continue;
      }
      if (current !== "metadata") continue; // 未知顶层字段的嵌套内容：容忍且不解释
      const nested = line.trim();
      if (nested.startsWith("- ")) {
        errors.push(`metadata 必须是 map[string]string，出现了列表项「${nested}」`);
        continue;
      }
      const match = nested.match(/^([^:]+):(.*)$/u);
      if (!match) {
        errors.push(`metadata 里有一行不是 key: value「${nested}」`);
        continue;
      }
      const key = match[1].trim();
      const scalar = parseScalar(match[2]);
      if (scalar.kind !== "string") {
        errors.push(`metadata.${key} 的值必须是字符串，实际是${scalar.kind === "empty" ? "空值" : ` ${scalar.kind} ${JSON.stringify(scalar.value)}`}`);
        continue;
      }
      const bucket = top.get("metadata");
      bucket.value.set(key, scalar.value);
      continue;
    }

    const match = line.match(/^([A-Za-z0-9_.-]+):(.*)$/u);
    if (!match) {
      errors.push(`顶层字段不是 key: value 形状「${line.trim()}」`);
      continue;
    }
    const key = match[1];
    current = key;
    const rest = match[2].trim();
    if (rest === "") {
      // 块状值：只有 metadata 会被解释，其余作为未知字段容忍。
      top.set(key, { kind: "block", value: new Map() });
      continue;
    }
    top.set(key, { kind: "scalar", scalar: parseScalar(rest) });
  }

  return { ok: errors.length === 0, top, errors };
}

/** 取一个必须是字符串的字段。 */
function stringField(top, key, errors) {
  const field = top.get(key);
  if (!field) return null;
  if (field.kind !== "scalar" || field.scalar.kind !== "string") {
    errors.push(`${key} 必须是字符串`);
    return null;
  }
  return field.scalar.value.trim();
}

/**
 * 校验一份 SKILL.md 的内容。
 *
 * @returns {{ ok: boolean, problems: string[], name: string | null, description: string | null }}
 */
export function validateSkill({ dirName, content }) {
  const problems = [];
  const split = splitFrontmatter(content);
  if (!split.ok) return { ok: false, problems: [split.error], name: null, description: null };

  const parsed = parseFrontmatter(split.frontmatter);
  problems.push(...parsed.errors);
  const top = parsed.top;

  const unknown = [...top.keys()].filter(
    (key) => !["name", "description", "license", "compatibility", "metadata", "allowed-tools"].includes(key),
  );
  if (unknown.length > 0) {
    // 容忍但要看得见：私有信息该进 metadata，不该自造顶层字段。
    console.log(`  ⚠ ${dirName}/SKILL.md 有未知顶层字段（已容忍）：${unknown.join(", ")}`);
  }

  const name = stringField(top, "name", problems);
  const description = stringField(top, "description", problems);
  const compatibility = stringField(top, "compatibility", problems);
  stringField(top, "license", problems);
  stringField(top, "allowed-tools", problems);

  if (!name) problems.push("缺少必填字段 name");
  if (name) {
    if (charLength(name) > LIMITS.name) problems.push(`name 长度 ${charLength(name)} 超过 ${LIMITS.name} 字符上限`);
    if (!NAME_PATTERN.test(name)) {
      problems.push(`name = ${JSON.stringify(name)} 不合规：只能用小写字母、数字与连字符，且不能以连字符开头或结尾`);
    }
    if (name !== dirName) problems.push(`name = ${JSON.stringify(name)} 与目录名 ${JSON.stringify(dirName)} 不一致（规范要求两者相同）`);
  }

  if (!description) problems.push("缺少必填字段 description（写「什么时候用这份 skill」，触发全靠它）");
  if (description && charLength(description) > LIMITS.description) {
    problems.push(`description 长度 ${charLength(description)} 超过 ${LIMITS.description} 字符上限`);
  }
  if (compatibility && charLength(compatibility) > LIMITS.compatibility) {
    problems.push(`compatibility 长度 ${charLength(compatibility)} 超过 ${LIMITS.compatibility} 字符上限`);
  }

  if (split.body.trim() === "") {
    problems.push("正文为空：frontmatter 之后必须是不为空的 Markdown 正文");
  }

  return { ok: problems.length === 0, problems, name, description };
}

/**
 * 扫 roots 下的每个子目录，逐份校验。
 *
 * @returns {{ ok: boolean, skills: {name: string, root: string}[], problems: {file: string, message: string}[] }}
 */
export function checkSkills({ roots }) {
  const skills = [];
  const problems = [];
  const seen = new Map();

  if (!roots || roots.length === 0) {
    return { ok: false, skills, problems: [{ file: "(配置)", message: "没有配置任何 skill 目录" }] };
  }

  for (const root of roots) {
    if (!existsSync(root)) {
      problems.push({ file: root, message: `策略目录不存在：${root}（读不到就不启动，没有代码内兜底文本）` });
      continue;
    }
    const entries = readdirSync(root, { withFileTypes: true })
      .filter((entry) => entry.isDirectory() && !entry.name.startsWith("."))
      .map((entry) => entry.name)
      .sort();

    for (const dirName of entries) {
      const file = join(root, dirName, "SKILL.md");
      if (!existsSync(file)) {
        problems.push({ file, message: `子目录 ${join(root, dirName)} 缺少 SKILL.md` });
        continue;
      }
      const result = validateSkill({ dirName, content: readFileSync(file, "utf8") });
      for (const message of result.problems) problems.push({ file, message });
      if (!result.ok) continue;
      if (seen.has(result.name)) {
        problems.push({ file, message: `skill 名称重复：${result.name}（已出现在 ${seen.get(result.name)}）` });
        continue;
      }
      seen.set(result.name, file);
      skills.push({ name: result.name, root });
    }
  }

  return { ok: problems.length === 0, skills, problems };
}

// ── 自检：人造违规用例必须真的变红 ────────────────────────────────────
//
// 门禁自己也要被证明会红，否则它只是"永远打印 ✓ 的脚本"。
// 这里每组用例都写进临时目录、走同一条 checkSkills 通道，断言：
//   - 合规样本必须通过（否则脚本在无条件失败）；
//   - 每题人造违规必须失败，且错误里出现预期片段（否则红得没有道理）。

/** 合规样本：name 与目录名一致、metadata 全字符串、正文非空。 */
const SAMPLE_OK = `---
name: demo-skill
description: 示例 skill，用来验证门禁本身。
metadata:
  myblogx.revision: "1"
---

正文。
`;

/** 自检用例：dir 是目录名，content 是 SKILL.md 内容，expect 是错误里必须出现的片段。 */
export const SELF_TEST_CASES = [
  {
    name: "名称与目录名不一致",
    dir: "demo-skill",
    content: SAMPLE_OK.replace("name: demo-skill", "name: other-skill"),
    expect: "目录名",
  },
  {
    name: "描述超长",
    dir: "demo-skill",
    content: SAMPLE_OK.replace("description: 示例 skill，用来验证门禁本身。", `description: ${"描".repeat(1025)}`),
    expect: "description",
  },
  {
    name: "正文为空",
    dir: "demo-skill",
    content: "---\nname: demo-skill\ndescription: 示例。\n---\n\n   \n",
    expect: "正文",
  },
  {
    name: "metadata 值非字符串",
    dir: "demo-skill",
    content: SAMPLE_OK.replace('myblogx.revision: "1"', "myblogx.revision: 1"),
    expect: "metadata.myblogx.revision",
  },
  {
    name: "缺少 frontmatter",
    dir: "demo-skill",
    content: "# 只有正文\n",
    expect: "frontmatter",
  },
];

/** 对照用例：这些必须通过，否则门禁会误杀合法文本。 */
export const SELF_TEST_PASS_CASES = [
  { name: "合规样本", dir: "demo-skill", content: SAMPLE_OK },
  {
    name: "未知顶层字段（规范会加字段，容忍）",
    dir: "demo-skill",
    content: SAMPLE_OK.replace("---\n\n正文。", "x-future-field: whatever\n---\n\n正文。"),
  },
];

/** 把一组目录 + 文件写进临时目录，跑一次 checkSkills。 */
function checkFixture({ dir, content }) {
  const root = mkdtempSync(join(tmpdir(), "blogx-skills-gate-"));
  try {
    mkdirSync(join(root, dir), { recursive: true });
    writeFileSync(join(root, dir, "SKILL.md"), content, "utf8");
    return checkSkills({ roots: [root] });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/**
 * 跑自检：每个违规用例必须红且理由对得上，每个对照用例必须绿。
 *
 * @returns {{ ok: boolean, lines: {ok: boolean, text: string}[] }}
 */
export function runSelfTest() {
  const lines = [];

  for (const testCase of SELF_TEST_CASES) {
    const result = checkFixture(testCase);
    if (result.ok) {
      lines.push({ ok: false, text: `人造违规「${testCase.name}」没有被判红（门禁失效）` });
      continue;
    }
    const messages = result.problems.map((problem) => `${problem.file}: ${problem.message}`).join("\n");
    if (!messages.includes(testCase.expect)) {
      lines.push({
        ok: false,
        text: `人造违规「${testCase.name}」红了，但理由不对（应出现「${testCase.expect}」）：\n${messages}`,
      });
      continue;
    }
    lines.push({ ok: true, text: `人造违规「${testCase.name}」被判红，并点明「${testCase.expect}」` });
  }

  for (const testCase of SELF_TEST_PASS_CASES) {
    const result = checkFixture(testCase);
    if (!result.ok) {
      const messages = result.problems.map((problem) => `${problem.file}: ${problem.message}`).join("\n");
      lines.push({ ok: false, text: `对照用例「${testCase.name}」被误杀：\n${messages}` });
      continue;
    }
    lines.push({ ok: true, text: `对照用例「${testCase.name}」通过（门禁不是无条件失败）` });
  }

  return { ok: lines.every((line) => line.ok), lines };
}

function runCli(argv) {
  const roots = [];
  let selfTest = false;

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--self-test") selfTest = true;
    else if (arg === "--root") {
      if (!argv[index + 1]) {
        console.error("✗ --root 需要一个目录");
        return 2;
      }
      roots.push(argv[(index += 1)]);
    } else if (arg.startsWith("--root=")) roots.push(arg.slice("--root=".length));
    else {
      console.error(`✗ 未知参数 ${arg}（用法：node scripts/check-skills.mjs [--root <dir>]... [--self-test]）`);
      return 2;
    }
  }

  if (selfTest) {
    const result = runSelfTest();
    for (const line of result.lines) console[line.ok ? "log" : "error"](`${line.ok ? "✓" : "✗"} ${line.text}`);
    if (!result.ok) {
      console.error("✗ skill 门禁自检失败：它没有像说的那样变红");
      return 1;
    }
    console.log(
      `✓ skill 门禁自检通过（${SELF_TEST_CASES.length} 个人造违规用例 + ${SELF_TEST_PASS_CASES.length} 个对照用例）`,
    );
    return 0;
  }

  const repoRoot = findRepoRoot();
  const resolved = (roots.length > 0 ? roots : [DEFAULT_ROOT]).map((root) => resolve(repoRoot, root));
  const result = checkSkills({ roots: resolved });

  if (result.ok) {
    const names = result.skills.map((skill) => skill.name).join(", ");
    console.log(`✓ 运行时策略 skill 门禁通过（${result.skills.length} 份：${names}）`);
    return 0;
  }

  console.error("✗ 运行时策略 skill 门禁失败");
  for (const problem of result.problems) console.error(`  ${problem.file}: ${problem.message}`);
  console.error("  规则见 skills/README.md；读不到或不合规的 skill 会让进程启动失败（ADR-0033）。");
  return 1;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
  process.exit(runCli(process.argv.slice(2)));
}
