// covering-stems：从一份词表里推导"缺失的主干词"，供部署侧补导。
//
// ## 为什么需要它
//
// 词表里大量条目是「主干 + 渠道/交易后缀」的组合（冰毒、冰毒qq、qq出售冰毒 …）。
// 过滤引擎是子串匹配（AC），所以只要**主干本身**在表里，「qq出售冰毒」这类变体
// 命中与否无所谓。反过来，如果某条主干**只以变体形式存在**，就出现覆盖漏洞：
//
//   - 表里有「仿真气枪出售」「仿真气枪批发」，没有「仿真气枪」；
//   - 文章里单独写「仿真气枪」时不命中，写成「仿真气枪出售」反而命中。
//
// 2026-11-22 生产实测：33873 条词表里有 3676 个这类"只有变体、没有主干"的缺口，
// 其中 1009 个是 2–4 字的短主干（`雷管`、`仿真气枪` 这类本身就是完整词）。
//
// ## 为什么要额外允许「1 个中文字 + 拉丁/数字」的候选
//
// 上游生成规则是「CJK 少于 2 个字符的词丢弃」。这条规则防的是「毒」「赌」这类
// 单字误报，但会连带丢掉了 `k粉` 这种**只有 1 个中文字却完整成立**的词。
//
// 本工具的立场：只对**已经从既有词条里推导出来**的候选放宽——候选不是凭空进来的，
// 它本身就是表内 ≥2 条词条的前缀，说明这是个真实在用的词。上游生成阶段仍按原规则
// 走（那里没有这层证据）。
//
// ## 输出
//
// TSV（词<TAB>档位<TAB>类目），可直接喂给 `-filter-import`。
// 推导出来的主干沿用**其变体里出现最多的那条**的档位与类目。
//
// 用法：
//   node scripts/content-filter/covering-stems.mjs --in cur.tsv --out supplement.tsv
//   node scripts/content-filter/covering-stems.mjs --in cur.tsv --out supplement.tsv --dry-run

import { readFileSync, writeFileSync } from "node:fs";

/** 渠道/交易后缀：出现在词尾时可以被剥掉。与词表生成的去噪口径独立维护。 */
const CHANNEL_SUFFIXES = [
  "qq",
  "电话",
  "微信",
  "出售",
  "销售",
  "供应",
  "批发",
  "提供",
  "直销",
  "订购",
  "购买",
  "送货上门",
  "到货",
  "出货",
  "配方",
  "制作方法",
  "制作技术",
  "制造方法",
  "教程",
  "价格",
  "联系",
];

/** 候选主干的 rune 长度范围：太短是噪音，太长就是整句话（不该当主干补）。 */
const MIN_STEM_RUNES = 2;
const MAX_STEM_RUNES = 8;

/**
 * 一个候选至少要能解释这么多条既有词条，才算"真实在用的词"。
 *
 * 例外见 minSupportFor：两字全中文的候选只要求 1 条。理由是这类词本身就是完整词
 * （`雷管` 这种），少一条变体不代表它不真实；而三字以上的"单变体主干"更可能是
 * 从长句里截出来的碎片（`0售k粉`），所以仍要求两处证据。
 */
const MIN_SUPPORT = 2;

/** 两字全中文候选的支撑门槛（完整词，1 处证据即可）。 */
const MIN_SUPPORT_SHORT = 1;

/** 统计 rune 数（`Array.from` 按码点切，emoji 之类不会被算成两个）。 */
function runeLen(text) {
  return Array.from(text).length;
}

/** 含多少个 CJK 字符。 */
function cjkCount(text) {
  let count = 0;
  for (const ch of text) {
    const code = ch.codePointAt(0);
    if (
      (code >= 0x4e00 && code <= 0x9fff) ||
      (code >= 0x3400 && code <= 0x4dbf) ||
      (code >= 0xf900 && code <= 0xfaff)
    ) {
      count += 1;
    }
  }
  return count;
}

/** 是否为纯拉丁/数字（含空格与常见分隔符）——这类词没有分类意义，不当主干。 */
function isLatinOnly(text) {
  return /^[a-z0-9 ._-]+$/i.test(text);
}

/**
 * 剥掉一个词尾部的渠道后缀，返回主干。
 *
 * 只剥一次：`冰毒qq电话` 剥掉 `电话` 得到 `冰毒qq`，下一轮由 `冰毒qq` 自己那条
 * （它本身也是词条）贡献 `冰毒`。反复剥会把 `出售冰毒` 也变成 `冰毒`，那属于
 * 前缀噪音，不是我们要补的"主干"。
 */
export function stripChannelSuffix(word) {
  const text = String(word ?? "").trim();
  const lower = text.toLowerCase();
  for (const suffix of CHANNEL_SUFFIXES) {
    if (lower.length > suffix.length && lower.endsWith(suffix)) {
      return text.slice(0, text.length - suffix.length).trim();
    }
  }
  return null;
}

/** 候选要求的支撑条数：两字全中文放宽到 1，其余 2。 */
export function minSupportFor(stem) {
  const runes = Array.from(String(stem ?? ""));
  if (runes.length === 2 && runes.every((ch) => cjkCount(ch) === 1)) {
    return MIN_SUPPORT_SHORT;
  }
  return MIN_SUPPORT;
}

/**
 * 剥掉词**开头**的渠道词（`出售雷管` → `雷管`、`qq提供雷管炸药` → `雷管炸药`）。
 *
 * 为什么前缀也要剥：组合词是双向的，`出售雷管炸药` 里的主干在后面，只剥尾部
 * 永远推不出 `雷管`。最多剥三层，避免把 `供应出售批发冰毒` 这类长串削成一地碎片。
 */
export function stripChannelPrefix(word) {
  let text = String(word ?? "").trim();
  let stripped = 0;
  for (let round = 0; round < 3; round += 1) {
    const lower = text.toLowerCase();
    const hit = CHANNEL_SUFFIXES.find(
      (prefix) => lower.length > prefix.length && lower.startsWith(prefix),
    );
    if (!hit) break;
    text = text.slice(hit.length).trim();
    stripped += 1;
  }
  return stripped === 0 ? null : text;
}

/** 候选主干是否合格（长度、含 CJK、非渠道词、非纯拉丁）。 */
export function isCandidateStem(stem) {
  if (!stem) return false;
  const length = runeLen(stem);
  if (length < MIN_STEM_RUNES || length > MAX_STEM_RUNES) return false;
  if (cjkCount(stem) < 1) return false;
  if (isLatinOnly(stem)) return false;
  // 候选自己还带渠道后缀，说明它是另一层组合，不是主干。
  if (stripChannelSuffix(stem) !== null) return false;
  return true;
}

/**
 * 一条词条可能推出的全部主干候选：剥尾、剥头、两头都剥。
 *
 * 三者都试，是因为组合词的方向不固定（`冰毒qq` 主干在前，`出售雷管` 主干在后）。
 * 返回去重后的数组；同一词条贡献多个候选时，每个候选各自 +1 支撑度。
 */
export function candidateStemsOf(word) {
  const out = new Set();
  const tail = stripChannelSuffix(word);
  if (tail) out.add(tail);
  const head = stripChannelPrefix(word);
  if (head) out.add(head);
  if (tail) {
    const both = stripChannelPrefix(tail);
    if (both) out.add(both);
  }
  if (head) {
    const both = stripChannelSuffix(head);
    if (both) out.add(both);
  }
  return [...out];
}

/** 累加一个候选的支撑度与档位/类目票数。 */
function countCandidate(candidates, key, stem, entry) {
  const current = candidates.get(key);
  const tag = `${entry.tier}\t${entry.category}`;
  if (current) {
    current.support += 1;
    current.tags.set(tag, (current.tags.get(tag) ?? 0) + 1);
    return;
  }
  candidates.set(key, { word: stem, support: 1, tags: new Map([[tag, 1]]) });
}

/**
 * 从词条里推导缺失的主干。
 *
 * @param entries 词条数组 `{ word, tier, category }`
 * @returns `{ stems: [{ word, tier, category, support }], stats }`
 */
export function deriveCoveringStems(entries) {
  const existing = new Set();
  for (const entry of entries) {
    existing.add(String(entry.word).trim().toLowerCase());
  }

  // 候选 → { support 次数, 变体里最常见的档位/类目 }
  const candidates = new Map();
  let stripped = 0;
  for (const entry of entries) {
    const stems = candidateStemsOf(entry.word);
    if (stems.length === 0) continue;
    stripped += 1;
    for (const stem of stems) {
      const key = stem.toLowerCase();
      if (existing.has(key)) continue; // 主干本来就在表里，不需要补
      if (!isCandidateStem(stem)) continue;
      countCandidate(candidates, key, stem, entry);
    }
  }


  const stems = [];
  for (const [key, value] of candidates) {
    if (value.support < minSupportFor(value.word)) continue;
    // 取支持度最高的档位/类目；平票按字符串排序，保证输出可复现。
    let bestTag = "";
    let bestCount = -1;
    for (const tag of [...value.tags.keys()].sort()) {
      const count = value.tags.get(tag);
      if (count > bestCount) {
        bestCount = count;
        bestTag = tag;
      }
    }
    const [tier, category] = bestTag.split("\t");
    stems.push({ word: value.word, tier, category, support: value.support, key });
  }

  // 输出顺序：先按类目（沿用词表顺序），再按支持度降序，最后按词——可复现。
  stems.sort((a, b) => {
    if (a.category !== b.category) return a.category < b.category ? -1 : 1;
    if (a.support !== b.support) return b.support - a.support;
    return a.key < b.key ? -1 : 1;
  });

  return {
    stems,
    stats: {
      entries: entries.length,
      stripped,
      candidates: candidates.size,
      emitted: stems.length,
    },
  };
}

/** 解析 TSV 词表（与 -filter-import 同一格式；忽略空行与 # 注释）。 */
export function parseTsv(text) {
  const entries = [];
  for (const raw of text.split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (!line || line.startsWith("#")) continue;
    const parts = line.split("\t");
    if (parts.length < 3) continue;
    const word = parts[0].trim();
    const tier = parts[1].trim();
    const category = parts[2].trim();
    if (!word || !tier || !category) continue;
    entries.push({ word, tier, category });
  }
  return entries;
}

/** 渲染成 TSV（输出给 -filter-import）。 */
export function renderTsv(stems) {
  return stems.map((stem) => `${stem.word}\t${stem.tier}\t${stem.category}`).join("\n") + "\n";
}

/** 解析命令行参数（手写，与 build-wordlist.mjs 同一风格）。 */
export function parseArgs(argv) {
  const options = { in: "", out: "", dryRun: false };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--in":
        options.in = argv[++i] ?? "";
        break;
      case "--out":
        options.out = argv[++i] ?? "";
        break;
      case "--dry-run":
        options.dryRun = true;
        break;
      case "-h":
      case "--help":
        options.help = true;
        break;
      default:
        throw new Error(`不认识的参数: ${arg}`);
    }
  }
  return options;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help || !options.in) {
    console.log(`用法: node scripts/content-filter/covering-stems.mjs --in <词表.tsv> [--out <补充.tsv>] [--dry-run]

从既有词表推导"只以变体存在、主干缺失"的词，输出可直接 -filter-import 的 TSV。
输出只含词条本身，不含任何内容原文；生成物不要提交仓库。`);
    process.exit(options.help ? 0 : 1);
  }

  const entries = parseTsv(readFileSync(options.in, "utf8"));
  const { stems, stats } = deriveCoveringStems(entries);

  console.log(`读入 ${stats.entries} 条；推得出候选的词条 ${stats.stripped} 条；候选主干 ${stats.candidates} 个；选出 ${stats.emitted} 个`);

  if (options.dryRun || !options.out) {
    console.log("这是预演：没有写文件。带 --out <file> 才会输出。");
    return;
  }
  writeFileSync(options.out, renderTsv(stems), "utf8");
  console.log(`已写入 ${options.out}（${stems.length} 条）`);
}

// 仅当作为脚本直接运行时执行 main（被测试 import 时不跑）。
if (process.argv[1] && process.argv[1].endsWith("covering-stems.mjs")) {
  main();
}





