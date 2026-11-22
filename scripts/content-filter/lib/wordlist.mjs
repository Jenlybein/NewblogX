// 初始词表构建的纯函数：上游标签解析 → 本站七个类目 / 两档档位 → 去噪与截断 → 文本输出。
//
// 与 CLI 入口（build-wordlist.mjs）分开，是为了让映射、过滤、去重与截断这几条最容易
// 出错的规则能被 node:test 直接测到（见 test/wordlist.test.mjs），不需要真的下载词库。
//
// 上游（houbb/sensitive-word-data）的标签只有 5 类，且用数字表示：
//   0 政治 / 1 毒品 / 2 色情 / 3 赌博 / 4 违法
// 本站有 7 类：politics drugs porn gambling fraud fake_docs violence
// 所以 "违法" 必须再细分 —— 这是本仓库对词表做的**改动之一**（见 docs/third-party）。
//
// **暴恐词表不进仓库**（ADR-0032 决策 8 / issue #393 US34）：涉政与暴恐类词只进
// 数据库。所以这里只保留"怎么细分"的规则，枪爆刀那类词从部署侧的 gitignored 文件
// 读（见 lib/violenceHints.mjs）；文件缺失时这类词兜底归 fraud —— 少一个类目
// 比把词写进 git 安全。

/** 上游数字标签 → 含义（仅用于日志与错误文案）。 */
export const UPSTREAM_TAGS = {
  0: "政治",
  1: "毒品",
  2: "色情",
  3: "赌博",
  4: "违法",
};

/**
 * 一个词带多个标签时的归类优先级。
 *
 * 顺序刻意把 block 档的类目排在前面：一个词同时被判为"色情"和"违法"时，归到
 * watch 档的色情会让它在风险页里看起来更轻，实际处置时是错的。
 */
const TAG_PRIORITY = ["0", "1", "4", "3", "2"];

/** 本站七个类目（顺序即输出排序与词云顺序）。 */
export const CATEGORIES = ["drugs", "gambling", "fraud", "porn", "fake_docs", "politics", "violence"];

/**
 * 双字词的最短长度按类目分档：这三个类的双字词几乎都是通用词，误报压过收益。
 *
 * 依据是生产实测（2026-11-17 首轮清洗）：3 万词里"插入 / 导航 / 供应 / 来干"这类
 * 两字通用词把 60 篇技术文章的正文打成了 `**`。毒品与假证发票保留双字
 * （"冰毒 / 大麻 / 发票"辨识度高，且正是事故里命中的那一类）。
 */
export const SHORT_WORD_MIN_RUNES = {
  porn: 3,
  gambling: 3,
  fraud: 3,
};

/** 类目 → 档位（ADR-0032：毒品/枪支、涉政、暴恐、假证发票、诈骗 → 拦截；赌博/色情 → 观察）。 */
export const TIER_BY_CATEGORY = {
  politics: "block",
  drugs: "block",
  violence: "block",
  fake_docs: "block",
  fraud: "block",
  porn: "watch",
  gambling: "watch",
};

/**
 * "违法" 细分关键词表：命中即归入对应类目，都不命中兜底 fraud。
 *
 * 只放"看到就基本能确定"的词，宁可漏判回落到 fraud，也不要把"病毒"判成毒品这类
 * 事故写进表里（所以用"冰毒/大麻"而不是单字"毒"）。
 *
 * 这里**没有 violence 组**：枪爆刀类词属暴恐类，按 ADR-0032 决策 8 只进数据库，
 * 仓库里不出现。运行期由 buildWordlist 的 violenceHints 参数注入（见 lib/violenceHints.mjs）。
 */
const ILLEGAL_KEYWORDS = [
  {
    category: "fake_docs",
    words: [
      "发票", "假证", "办证", "证件", "文凭", "学历", "证书", "刻章", "假币", "伪币",
      "钞票", "印章", "公章", "车牌", "身份证", "护照", "户口", "驾照", "执照",
      "合格证", "税票", "代开", "印花税",
    ],
  },
  {
    category: "drugs",
    words: [
      "冰毒", "大麻", "海洛因", "可卡因", "摇头丸", "鸦片", "吸毒", "麻古", "麻果",
      "氯胺酮", "k粉", "麻黄素", "迷药", "迷魂", "毒品", "制毒", "贩毒", "k他命",
      "安钠咖", "美沙酮",
    ],
  },
];

/**
 * 把一条上游词条解析成 {word, tags}。
 *
 * 上游格式是 `敏感词 tag1,tag2`，中间用**空格**分隔；敏感词本身可能含数字与字母，
 * 所以从最后一个空格切，不用 split(" ")。
 */
export function parseTagLine(line) {
  const trimmed = String(line ?? "").replace(/\r$/, "").trim();
  if (!trimmed) {
    return null;
  }
  const idx = trimmed.lastIndexOf(" ");
  if (idx <= 0) {
    return null;
  }
  const word = trimmed.slice(0, idx).trim();
  const tags = trimmed
    .slice(idx + 1)
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => tag && Object.prototype.hasOwnProperty.call(UPSTREAM_TAGS, tag));
  if (!word || tags.length === 0) {
    return null;
  }
  return { word, tags };
}

/** 解析上游标签文件（`敏感词 0,4` 每行一条）。 */
export function parseTagsFile(text) {
  const tags = new Map();
  for (const line of String(text ?? "").split("\n")) {
    const parsed = parseTagLine(line);
    if (!parsed) {
      continue;
    }
    const merged = new Set(tags.get(parsed.word) ?? []);
    for (const tag of parsed.tags) {
      merged.add(tag);
    }
    tags.set(parsed.word, [...merged]);
  }
  return tags;
}

/** 解析上游黑名单文件（每行一个词，无标签）。 */
export function parseDictFile(text) {
  const words = [];
  for (const line of String(text ?? "").split("\n")) {
    const word = line.replace(/\r$/, "").trim();
    if (word && !word.includes("\t")) {
      words.push(word);
    }
  }
  return words;
}

/** 一个词的 CJK 字符数（只有汉字算数：`0售k粉qq` 的 CJK 是 2）。 */
export function cjkCount(word) {
  const matched = String(word ?? "").match(/[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]/g);
  return matched ? matched.length : 0;
}

/** 按 rune 计的长度（中文按字符算，不用 UTF-16 码元）。 */
export function runeLength(word) {
  return [...String(word ?? "")].length;
}

/**
 * 把上游标签集映射成本站类目。
 *
 * 多标签取 TAG_PRIORITY 里最靠前的那个；"违法" 走 ILLEGAL_KEYWORDS 细分，兜底 fraud。
 */
export function mapCategory(word, tags, violenceHints = []) {
  const ordered = [...new Set(tags)].sort(
    (a, b) => TAG_PRIORITY.indexOf(a) - TAG_PRIORITY.indexOf(b),
  );
  for (const tag of ordered) {
    switch (tag) {
      case "0":
        return "politics";
      case "1":
        return "drugs";
      case "2":
        return "porn";
      case "3":
        return "gambling";
      case "4":
        return classifyIllegal(word, violenceHints);
      default:
        break;
    }
  }
  return null;
}

/**
 * "违法" 再细分：假证发票 / 暴恐 / 毒品，兜底诈骗。
 *
 * violenceHints 是**部署侧**注入的暴恐词（仓库里没有这份清单）。为空时这类词
 * 归 fraud —— 类目会不准，但词本身不会漏（词条仍进库、仍拦截）。
 */
export function classifyIllegal(word, violenceHints = []) {
  const text = String(word ?? "");
  for (const rule of ILLEGAL_KEYWORDS) {
    if (rule.words.some((keyword) => text.includes(keyword))) {
      return rule.category;
    }
  }
  if (violenceHints.some((keyword) => text.includes(keyword))) {
    return "violence";
  }
  return "fraud";
}

/**
 * 词条是否带**类目证据**：命中了某个类目的触发词，而不是走 fraud 兜底。
 *
 * 为什么需要这个判定（而不是直接看 classifyIllegal 的返回值）：`classifyIllegal`
 * 在"什么都没命中"时也返回 fraud（兜底），与"命中了一条诈骗规则"无法区分。
 *
 * 上游 dict 里那两万多条没有标签的词，绝大多数是长句垃圾（`qq出售某某`），但也有
 * `雷管`、`k粉`、`猎枪` 这种完整裸词——它们**只出现在 dict 里、没进 tags**，早年
 * 按"无标签就丢弃"的规则被整批扔掉了，结果词表里有 `qq提供雷管炸药` 却没有 `雷管`：
 * 单独写 `雷管` 不命中，写变体反而命中。用触发词把两者分开，就能只收有证据的那一小撮。
 */
export function hasCategoryEvidence(word, violenceHints = []) {
  const text = String(word ?? "");
  if (ILLEGAL_KEYWORDS.some((rule) => rule.words.some((keyword) => text.includes(keyword)))) {
    return true;
  }
  return violenceHints.some((keyword) => text.includes(keyword));
}

/**
 * 把上游两份文件变成候选词条（已映射类目与档位，未去重、未截断）。
 *
 * 三种丢弃分别计数，导入前后对得上账：
 *   - cjkShort：CJK 少于 2（单字词与纯拉丁/数字词，宽泛到无法使用）；
 *   - tooLong：超过 maxRunes（上游有拼接整句话的噪音词，会整段打星）；
 *   - untagged：只出现在 dict 里、没有标签，且**没有任何类目证据** → 丢弃；
 *   - dirty：含制表符/控制字符，写不进 TSV。
 */
export function buildCandidates({
  tags,
  dictWords = [],
  includeUntagged = false,
  minCjk = 2,
  maxRunes = 24,
  // violenceHints 是部署侧注入的暴恐词（仓库里没有这份清单），见 lib/violenceHints.mjs。
  violenceHints = [],
  // allowShort 关掉"按类目收紧最短长度"的口径（导入人工审核过的双字词时用）。
  allowShort = false,
}) {
  const entries = [];
  const dropped = {
    cjkShort: 0,
    tooLong: 0,
    untagged: 0,
    untaggedEvidence: 0,
    dirty: 0,
    noCategory: 0,
    shortGeneric: 0,
  };
  const seen = new Set();

  const accept = (word, tagset) => {
    if (word.includes("\t") || /[\u0000-\u001f\u007f]/.test(word)) {
      dropped.dirty += 1;
      return;
    }
    // CJK 门槛防的是"毒""赌"这类单字噪音。这条防噪音的目的在有**类目证据**时
    // 就不再成立：`k粉` 只有 1 个中文字，但它是明确的毒品词，丢掉它就等于漏掉
    // "k粉出售"以外的所有 `k粉` 写法。所以有证据时只要求整词长度 ≥ 2。
    if (cjkCount(word) < minCjk) {
      const evidenced = runeLength(word) >= 2 && hasCategoryEvidence(word, violenceHints);
      if (!evidenced) {
        dropped.cjkShort += 1;
        return;
      }
    }
    if (runeLength(word) > maxRunes) {
      dropped.tooLong += 1;
      return;
    }
    const category = mapCategory(word, tagset, violenceHints);
    if (!category) {
      dropped.noCategory += 1;
      return;
    }
    // 双字通用词：只在色情 / 赌博 / 诈骗三类里收紧（见 SHORT_WORD_MIN_RUNES）。
    if (!allowShort) {
      const need = SHORT_WORD_MIN_RUNES[category] ?? 2;
      if (runeLength(word) < need) {
        dropped.shortGeneric += 1;
        return;
      }
    }
    // 去重键用小写：词表是给人看的，两个"看起来一样"的词只会让运营怀疑自己。
    const key = word.toLowerCase();
    if (seen.has(key)) {
      return;
    }
    seen.add(key);
    entries.push({ word, tier: TIER_BY_CATEGORY[category], category });
  };

  for (const [word, tagset] of tags) {
    accept(word, tagset);
  }
  if (includeUntagged) {
    for (const word of dictWords) {
      if (tags.has(word)) {
        continue;
      }
      // 没有标签的词按"违法→兜底诈骗"处理：这是唯一能保证非 watch 档的兜底，
      // 且必须在类目细分之前过一遍，否则暴恐词会被静默归到诈骗。
      accept(word, ["4"]);
    }
  } else {
    // 没有标签的词：只收**带类目证据**的（`雷管`、`k粉` 这类完整裸词），
    // 其余的仍是长句垃圾，丢弃。证据判定与类目细分共用同一套触发词，
    // 所以这里先塞一个"违法"标签，让 accept 里的 mapCategory 走同一个细分。
    let untagged = 0;
    let untaggedEvidence = 0;
    for (const word of dictWords) {
      if (tags.has(word)) {
        continue;
      }
      if (!hasCategoryEvidence(word, violenceHints)) {
        untagged += 1;
        continue;
      }
      untaggedEvidence += 1;
      accept(word, ["4"]);
    }
    dropped.untagged += untagged;
    dropped.untaggedEvidence += untaggedEvidence;
  }

  return { entries, dropped };
}

/** 按类目过滤掉若干类（默认排除 politics 与 violence：涉政/暴恐不进仓库）。 */
export function excludeCategories(entries, excluded) {
  const skip = new Set(excluded);
  return entries.filter((entry) => !skip.has(entry.category));
}

/**
 * 按类目比例截断到 limit 条。
 *
 * 为什么按比例而不是"取前 N 条"：上游的色情与违法两类占了近九成，直接截断会让
 * 词云只剩两类，运营看不出分布。按比例保留能让七个类目都留得下代表性词条，
 * 同时不改变原词库的类目比例。类目内按词条 Unicode 排序后截断 —— 可复现。
 */
export function selectWithinLimit(entries, limit) {
  if (!Number.isFinite(limit) || limit <= 0 || entries.length <= limit) {
    return sortEntries(entries);
  }
  const byCategory = new Map(CATEGORIES.map((category) => [category, []]));
  for (const entry of entries) {
    const bucket = byCategory.get(entry.category);
    if (bucket) {
      bucket.push(entry);
    }
  }
  for (const bucket of byCategory.values()) {
    bucket.sort((a, b) => (a.word < b.word ? -1 : a.word > b.word ? 1 : 0));
  }

  const total = entries.length;
  const quota = new Map();
  const fractions = [];
  let assigned = 0;
  for (const category of CATEGORIES) {
    const size = byCategory.get(category).length;
    if (size === 0) {
      continue;
    }
    const exact = (limit * size) / total;
    const base = Math.min(size, Math.floor(exact));
    quota.set(category, base);
    assigned += base;
    fractions.push({ category, remainder: exact - Math.floor(exact) });
  }

  // 先按小数余数补差额（余数相同时按 CATEGORIES 顺序，保证结果与机器无关）。
  fractions.sort(
    (a, b) => b.remainder - a.remainder
      || CATEGORIES.indexOf(a.category) - CATEGORIES.indexOf(b.category),
  );
  let left = limit - assigned;
  for (const item of fractions) {
    if (left <= 0) {
      break;
    }
    if (quota.get(item.category) < byCategory.get(item.category).length) {
      quota.set(item.category, quota.get(item.category) + 1);
      left -= 1;
    }
  }
  // 有余量（有的类目被 size 卡住）就再轮一遍，直到补满或没有类目还能加。
  while (left > 0) {
    let progressed = false;
    for (const category of CATEGORIES) {
      if (left <= 0) {
        break;
      }
      const current = quota.get(category) ?? 0;
      if (current < byCategory.get(category).length) {
        quota.set(category, current + 1);
        left -= 1;
        progressed = true;
      }
    }
    if (!progressed) {
      break;
    }
  }

  const out = [];
  for (const category of CATEGORIES) {
    out.push(...byCategory.get(category).slice(0, quota.get(category) ?? 0));
  }
  return sortEntries(out);
}

/** 稳定排序：先类目（CATEGORIES 顺序），再词条 Unicode。 */
export function sortEntries(entries) {
  return [...entries].sort((a, b) => {
    const byCategory = CATEGORIES.indexOf(a.category) - CATEGORIES.indexOf(b.category);
    if (byCategory !== 0) {
      return byCategory;
    }
    return a.word < b.word ? -1 : a.word > b.word ? 1 : 0;
  });
}

/** 渲染 TSV（`词\t档位\t类目`），导入命令直接吃这个格式。 */
export function renderTsv(entries) {
  return entries.map((entry) => `${entry.word}\t${entry.tier}\t${entry.category}`).join("\n") + "\n";
}

/** 渲染 NDJSON（每行一个 `{"word","tier","category"}`），与 TSV 等价。 */
export function renderNdjson(entries) {
  return entries.map((entry) => JSON.stringify(entry)).join("\n") + "\n";
}

/** 统计各档位、各类目条数。 */
export function summarize(entries) {
  const tiers = { block: 0, watch: 0 };
  const categories = Object.fromEntries(CATEGORIES.map((category) => [category, 0]));
  for (const entry of entries) {
    if (entry.tier in tiers) {
      tiers[entry.tier] += 1;
    }
    if (entry.category in categories) {
      categories[entry.category] += 1;
    }
  }
  return { total: entries.length, tiers, categories };
}





