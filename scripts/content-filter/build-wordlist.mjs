#!/usr/bin/env node
// 初始词表构建：把 Apache-2.0 的上游词库（houbb/sensitive-word-data）映射成本站
// content_filter 段能吃的 `词<TAB>档位<TAB>类目` TSV，交给 `backend -filter-import` 导入。
//
// 刻意不新增依赖：下载用 Node 自带的 fetch，文件用 node:fs，参数解析手写。
//
// 默认**排除 politics 与 violence 两类**：涉政与暴恐词条不进仓库、不进默认配置、
// 不进 CI 日志（ADR-0032 决策 / #395 验收）。只有显式 `--include-politics` /
// `--include-violence`（或 `--include-all`）才会写进输出，且明确提示"仅用于部署侧导入"。
//
// 用法：
//   node scripts/content-filter/build-wordlist.mjs
//   node scripts/content-filter/build-wordlist.mjs --source ~/data/sensitive-word
//   node scripts/content-filter/build-wordlist.mjs --out .gotmp/words.tsv --format ndjson
//   node scripts/content-filter/build-wordlist.mjs --include-all   # 仅部署侧
//
// 输出默认落 `.gotmp/content-filter-words.tsv`（`.gotmp/` 已被 .gitignore 忽略）：
// 词表数据本身绝不进 git。

import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  buildCandidates,
  CATEGORIES,
  excludeCategories,
  parseDictFile,
  parseTagsFile,
  renderNdjson,
  renderTsv,
  selectWithinLimit,
  summarize,
} from "./lib/wordlist.mjs";
import { loadViolenceHints } from "./lib/violenceHints.mjs";

/** 上游仓库与资源目录（两份文件都在这里）。 */
const UPSTREAM_REPO = "houbb/sensitive-word-data";
const UPSTREAM_BRANCH = "main";
const UPSTREAM_DIR = "src/main/resources";
const UPSTREAM_FILES = {
  dict: "sensitive_word_dict.txt",
  tags: "sensitive_word_tags.txt",
};

/** 本地 `--source` 目录里找文件时依次尝试的子路径（仓库根 / maven 资源目录都认）。 */
const LOCAL_SUBDIRS = ["", "src/main/resources"];

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const DEFAULT_OUT = join(REPO_ROOT, ".gotmp", "content-filter-words.tsv");
const DEFAULT_LIMIT = 30000;

/** 打印用法。 */
function usage() {
  console.log(`用法: node scripts/content-filter/build-wordlist.mjs [选项]

选项:
  --source <dir>        本地 houbb/sensitive-word-data 目录（含 ${UPSTREAM_FILES.dict} 与 ${UPSTREAM_FILES.tags}）；
                        不给则从 raw.githubusercontent.com 下载到临时目录
  --out <file>          输出路径，默认 .gotmp/content-filter-words.tsv
  --format <tsv|ndjson> 输出格式，默认 tsv（词<TAB>档位<TAB>类目）
  --limit <n>           词条上限，默认 ${DEFAULT_LIMIT}（按类目比例截断）
  --min-cjk <n>         一个词至少几个汉字，默认 2（单字词不入表）
  --max-runes <n>       词条最大字符数，默认 24（去掉平台拼接的长噪音词）
  --include-untagged    把上游黑名单里没有标签的词也纳入（按"违法→兜底诈骗"处理），默认丢弃
  --exclude <类目,类目> 额外/替代排除的类目，默认 politics,violence
  --include-politics    包含 politics 类（仅用于部署侧导入，不要提交输出）
  --include-violence    包含 violence 类（仅用于部署侧导入，不要提交输出）
  --include-all         包含全部七类（= 上面两个都开）
  --help                显示本帮助`);
}

/** 解析命令行参数（手写：不引依赖，也避免 subarg 之类的第三方包）。 */
export function parseArgs(argv) {
  const opts = {
    source: "",
    out: DEFAULT_OUT,
    format: "tsv",
    limit: DEFAULT_LIMIT,
    minCjk: 2,
    maxRunes: 24,
    includeUntagged: false,
    includePolitics: false,
    includeViolence: false,
    exclude: null,
    help: false,
  };
  const needValue = (flag, value) => {
    if (value === undefined || value.startsWith("--")) {
      throw new Error(`${flag} 需要一个值`);
    }
    return value;
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--source":
        opts.source = resolve(expandHome(needValue(arg, argv[++i])));
        break;
      case "--out":
        opts.out = resolve(expandHome(needValue(arg, argv[++i])));
        break;
      case "--format": {
        const value = needValue(arg, argv[++i]);
        if (value !== "tsv" && value !== "ndjson") {
          throw new Error(`--format 只支持 tsv 或 ndjson，收到 ${value}`);
        }
        opts.format = value;
        break;
      }
      case "--limit":
        opts.limit = parsePositiveInt(arg, needValue(arg, argv[++i]));
        break;
      case "--min-cjk":
        opts.minCjk = parsePositiveInt(arg, needValue(arg, argv[++i]));
        break;
      case "--max-runes":
        opts.maxRunes = parsePositiveInt(arg, needValue(arg, argv[++i]));
        break;
      case "--include-untagged":
        opts.includeUntagged = true;
        break;
      case "--include-politics":
        opts.includePolitics = true;
        break;
      case "--include-violence":
        opts.includeViolence = true;
        break;
      case "--include-all":
        opts.includePolitics = true;
        opts.includeViolence = true;
        break;
      case "--exclude":
        opts.exclude = needValue(arg, argv[++i])
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean);
        break;
      case "--help":
      case "-h":
        opts.help = true;
        break;
      default:
        throw new Error(`不认识的参数: ${arg}（用 --help 看用法）`);
    }
  }
  if (opts.exclude) {
    const unknown = opts.exclude.filter((item) => !CATEGORIES.includes(item));
    if (unknown.length > 0) {
      throw new Error(`--exclude 里有不认识的类目: ${unknown.join(", ")}`);
    }
  }
  return opts;
}

/** 展开 `~/`（Windows 下也用 USERPROFILE）。 */
function expandHome(path) {
  if (!path.startsWith("~")) {
    return path;
  }
  return join(homedir(), path.slice(1));
}

/** 解析正整数参数。 */
function parsePositiveInt(flag, raw) {
  const value = Number(raw);
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${flag} 需要正整数，收到 ${raw}`);
  }
  return value;
}

/** 在 `--source` 目录里找上游文件；找不到返回 null。 */
function findLocalSource(dir) {
  for (const sub of LOCAL_SUBDIRS) {
    const base = sub ? join(dir, sub) : dir;
    const dictPath = join(base, UPSTREAM_FILES.dict);
    const tagsPath = join(base, UPSTREAM_FILES.tags);
    if (existsSync(dictPath) && existsSync(tagsPath)) {
      return { dictPath, tagsPath, origin: base };
    }
  }
  return null;
}

/**
 * 从上游下载两份文件到临时目录。
 *
 * 为什么落临时目录而不是仓库：词库文件本身就是敏感数据，写进工作区就等于给了
 * `git add -A` 一个把它们提交进去的机会。
 */
async function downloadSource(logger) {
  const dir = mkdtempSync(join(tmpdir(), "blogx-wordlist-"));
  const dictPath = join(dir, UPSTREAM_FILES.dict);
  const tagsPath = join(dir, UPSTREAM_FILES.tags);
  for (const [name, target] of [[UPSTREAM_FILES.dict, dictPath], [UPSTREAM_FILES.tags, tagsPath]]) {
    const url = `https://raw.githubusercontent.com/${UPSTREAM_REPO}/${UPSTREAM_BRANCH}/${UPSTREAM_DIR}/${name}`;
    logger(`下载 ${url}`);
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`下载 ${url} 失败: HTTP ${res.status}`);
    }
    writeFileSync(target, Buffer.from(await res.arrayBuffer()));
  }
  return { dictPath, tagsPath, origin: `下载（${UPSTREAM_REPO}）→ ${dir}` };
}

/** 主流程：读源 → 映射 → 过滤 → 截断 → 写文件 → 打印统计。 */
export async function buildWordlist(opts, logger = console.log) {
  const source = opts.source ? findLocalSource(opts.source) : null;
  if (opts.source && !source) {
    throw new Error(
      `在 ${opts.source} 里找不到 ${UPSTREAM_FILES.dict} 与 ${UPSTREAM_FILES.tags}` +
        `（试过 ${LOCAL_SUBDIRS.map((sub) => sub || ".").join(" / ")}）`,
    );
  }
  const resolved = source ?? (await downloadSource(logger));

  const tags = parseTagsFile(readFileSync(resolved.tagsPath, "utf8"));
  const dictWords = parseDictFile(readFileSync(resolved.dictPath, "utf8"));
  logger(`数据源: ${resolved.origin}`);
  logger(`上游: 标签词 ${tags.size} 条，黑名单 ${dictWords.length} 条`);

  const violence = loadViolenceHints();
  logger(`暴恐词表: ${violence.source}`);
  const { entries, dropped } = buildCandidates({
    tags,
    dictWords,
    includeUntagged: opts.includeUntagged,
    minCjk: opts.minCjk,
    maxRunes: opts.maxRunes,
    violenceHints: violence.hints,
  });

  // 默认排除 politics 与 violence；`--exclude` 显式给出时以它为准（完全替代默认值，
  // 避免"开了一个类目反而把另一个类目也打开"这种看不出来的行为）。
  const defaultExcluded = [
    ...(opts.includePolitics ? [] : ["politics"]),
    ...(opts.includeViolence ? [] : ["violence"]),
  ];
  const excluded = opts.exclude ?? defaultExcluded;
  const kept = excludeCategories(entries, excluded);
  const excludedCount = entries.length - kept.length;
  const selected = selectWithinLimit(kept, opts.limit);

  const content = opts.format === "ndjson" ? renderNdjson(selected) : renderTsv(selected);
  mkdirSync(dirname(opts.out), { recursive: true });
  writeFileSync(opts.out, content, "utf8");

  const stats = summarize(selected);
  logger("");
  logger(`── 统计 ──`);
  logger(`丢弃: CJK<${opts.minCjk} ${dropped.cjkShort} 条，超过 ${opts.maxRunes} 字 ${dropped.tooLong} 条，` +
    `含非法字符 ${dropped.dirty} 条，无标签 ${dropped.untagged} 条，无类目 ${dropped.noCategory} 条`);
  logger(`排除类目(${excluded.join(",") || "无"}): ${excludedCount} 条`);
  if (kept.length > selected.length) {
    logger(`按类目比例截断: ${kept.length} → ${selected.length} 条（上限 ${opts.limit}）`);
  }
  logger(`输出 ${stats.total} 条 → ${opts.out}`);
  logger(`档位: block ${stats.tiers.block}，watch ${stats.tiers.watch}`);
  for (const category of CATEGORIES) {
    logger(`  ${category.padEnd(10)} ${stats.categories[category]}`);
  }

  if (opts.includePolitics || opts.includeViolence) {
    logger("");
    logger("⚠️  输出含 politics / violence 类：仅用于部署侧导入（直接进数据库），" +
      "不要提交到仓库、不要贴进 issue 或日志。");
  }
  return { out: opts.out, stats, dropped, excluded };
}

/** 入口。 */
async function main() {
  const opts = parseArgs(process.argv.slice(2));
  if (opts.help) {
    usage();
    return 0;
  }
  const result = await buildWordlist(opts);
  if (result.stats.total === 0) {
    throw new Error("一条词也没产出：检查 --source 指向的目录是否含上游两份文件");
  }
  return 0;
}

// 只在被当成脚本直接执行时跑 main：被 node:test import 时不执行。
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const code = await main();
    process.exitCode = code;
  } catch (error) {
    console.error(`构建失败: ${error.message}`);
    process.exitCode = 1;
  }
}

