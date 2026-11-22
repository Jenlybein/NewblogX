// 词表构建规则的单元测试：标签解析、类目映射、档位、过滤去重、按类目截断、输出格式。
//
// 期望值都手写（不从被测代码反推）：这些规则一旦被"自动对齐"，测试就只能证明
// 代码和自己一致，证明不了规则是对的。
import { test } from "node:test";
import assert from "node:assert/strict";

import {
  buildCandidates,
  cjkCount,
  classifyIllegal,
  excludeCategories,
  mapCategory,
  parseDictFile,
  parseTagLine,
  parseTagsFile,
  renderNdjson,
  renderTsv,
  runeLength,
  selectWithinLimit,
  sortEntries,
  summarize,
  TIER_BY_CATEGORY,
} from "../lib/wordlist.mjs";
import { parseArgs } from "../build-wordlist.mjs";

test("parseTagLine：从最后一个空格切开，词里的数字与字母原样保留", () => {
  assert.deepEqual(parseTagLine("0售k粉qq 1,4"), { word: "0售k粉qq", tags: ["1", "4"] });
  assert.deepEqual(parseTagLine("08宪章 0"), { word: "08宪章", tags: ["0"] });
});

test("parseTagLine：没有标签、没有空格、标签全在 0-4 之外的词都丢掉", () => {
  assert.equal(parseTagLine("只有词没有标签"), null);
  assert.equal(parseTagLine("  "), null);
  assert.equal(parseTagLine("某词 9"), null);
  assert.equal(parseTagLine("某词 x,y"), null);
  // 部分标签不认识时保留认识的那部分：上游偶尔会追加新标签，不该整行丢掉。
  assert.deepEqual(parseTagLine("某词 1,x"), { word: "某词", tags: ["1"] });
});

test("parseTagsFile：同一个词出现两次时标签取并集", () => {
  const tags = parseTagsFile("某词 1\n另一词 2\n某词 4\n坏行\n");
  assert.deepEqual([...tags.get("某词")].sort(), ["1", "4"]);
  assert.deepEqual(tags.get("另一词"), ["2"]);
  assert.equal(tags.size, 2);
});

test("parseDictFile：逐行一个词，忽略空行", () => {
  assert.deepEqual(parseDictFile("甲词\n\n乙词\n"), ["甲词", "乙词"]);
});

test("cjkCount / runeLength：汉字数与字符数分开算", () => {
  assert.equal(cjkCount("0售k粉qq"), 2);
  assert.equal(cjkCount("abc"), 0);
  assert.equal(runeLength("冰毒qq"), 4);
});

test("mapCategory：上游五类一一对应", () => {
  assert.equal(mapCategory("某词", ["0"]), "politics");
  assert.equal(mapCategory("某词", ["1"]), "drugs");
  assert.equal(mapCategory("某词", ["2"]), "porn");
  assert.equal(mapCategory("某词", ["3"]), "gambling");
});

test("classifyIllegal：发票假证 → fake_docs，毒品 → drugs，其余兜底 fraud", () => {
  assert.equal(classifyIllegal("代开增值税发票"), "fake_docs");
  assert.equal(classifyIllegal("办假证"), "fake_docs");
  assert.equal(classifyIllegal("冰毒批发"), "drugs");
  assert.equal(classifyIllegal("刷单日结"), "fraud");
});

// 暴恐词表**不在仓库里**（ADR-0032 决策 8）：没有注入时这类词兜底 fraud，
// 注入后归 violence。这条用例同时守住"仓库里不得出现暴恐词"这件事：
// 词表只能从 violenceHints 参数来，不能写死在 lib/wordlist.mjs 里。
test("classifyIllegal：没有注入暴恐词时兜底 fraud，注入后归 violence", () => {
  assert.equal(classifyIllegal("某违禁品交易"), "fraud");
  assert.equal(classifyIllegal("某违禁品交易", ["违禁品"]), "violence");
});

test("mapCategory：多标签按 block 档优先归类", () => {
  // 同时是色情(2)与违法(4)：归违法细分，而不是更容易放过的色情。
  assert.equal(mapCategory("卖淫", ["2", "4"]), "fraud");
  // 同时是毒品(1)与违法(4)：毒品优先。
  assert.equal(mapCategory("卖药", ["1", "4"]), "drugs");
  // 政治(0) 在最高优先级。
  assert.equal(mapCategory("某词", ["0", "2"]), "politics");
});

test("档位：politics/drugs/violence/fake_docs/fraud 拦截，porn/gambling 观察", () => {
  assert.deepEqual(TIER_BY_CATEGORY, {
    politics: "block",
    drugs: "block",
    violence: "block",
    fake_docs: "block",
    fraud: "block",
    porn: "watch",
    gambling: "watch",
  });
});

test("buildCandidates：单字、无 CJK、超长、重复都丢弃并计数", () => {
  const tags = new Map([
    ["毒", ["1"]],
    ["冰毒", ["1"]],
    ["冰毒", ["1"]],
    ["abcd", ["2"]],
    // 双字 + 诈骗类：按 SHORT_WORD_MIN_RUNES 收紧，应计入 shortGeneric。
    ["卖淫", ["2", "4"]],
    // 三字 + 诈骗类：保留。
    ["卖淫嫖娼", ["4"]],
  ]);
  const long = "很长很长".repeat(10);
  tags.set(long, ["4"]);
  const { entries, dropped } = buildCandidates({ tags });
  assert.deepEqual(
    entries.map((entry) => entry.word).sort(),
    ["冰毒", "卖淫嫖娼"].sort(),
  );
  assert.equal(dropped.cjkShort, 2, "单字词与纯拉丁词都计入 CJK 不足");
  assert.equal(dropped.tooLong, 1, "超过 24 字的噪音词应丢弃");
  assert.equal(dropped.shortGeneric, 1, "双字通用词按类目收紧后计入 shortGeneric");
  assert.equal(entries.find((entry) => entry.word === "冰毒").tier, "block");
  assert.equal(entries.find((entry) => entry.word === "卖淫嫖娼").category, "fraud");
});

// 生产实测（2026-11-17）的口径回归：三个通用类收紧到 3 rune，毒品 / 假证保留 2 rune。
// 这条用例存在的理由很具体 —— 3 万词里「插入」「导航」「供应」曾把 60 篇技术文章
// 的正文打成 `**`，而「冰毒」「发票」必须仍然命中。
test("buildCandidates：双字词按类目分档（通用类丢、毒品与假证留）", () => {
  const tags = new Map([
    ["插入", ["2"]],
    ["导航", ["4"]],
    ["供应", ["4"]],
    ["冰毒", ["1"]],
    ["发票", ["4"]],
    ["代开发票", ["4"]],
  ]);
  const { entries, dropped } = buildCandidates({ tags });
  assert.deepEqual(entries.map((entry) => entry.word).sort(), ["代开发票", "冰毒", "发票"].sort());
  assert.equal(dropped.shortGeneric, 3);
  // --allow-short 逃生口：显式要求时双字通用词可以被保留。
  const relaxed = buildCandidates({ tags, allowShort: true });
  assert.equal(relaxed.entries.length, 6);
});

test("buildCandidates：去重键是小写，保留首次出现的写法", () => {
  const tags = new Map([
    ["ABC冰毒", ["4"]],
    ["abc冰毒", ["4"]],
  ]);
  const { entries } = buildCandidates({ tags });
  assert.deepEqual(entries.map((entry) => entry.word), ["ABC冰毒"]);
});

test("buildCandidates：无标签词只收带类目证据的，--include-untagged 时才全收（按违法兜底）", () => {
  const tags = new Map([["冰毒", ["1"]]]);
  const dictWords = ["无标签词", "无标签冰毒", "冰毒"];

  // 默认口径：没标签的词只收**带类目证据**的（含触发词），其余丢弃。
  // `无标签冰毒` 含"冰毒"这条毒品触发词 → 收；`无标签词` 没有任何证据 → 丢。
  const withoutUntagged = buildCandidates({ tags, dictWords });
  assert.deepEqual(withoutUntagged.entries.map((entry) => entry.word), ["冰毒", "无标签冰毒"]);
  assert.equal(withoutUntagged.dropped.untagged, 1);
  assert.equal(withoutUntagged.dropped.untaggedEvidence, 1);

  const withUntagged = buildCandidates({ tags, dictWords, includeUntagged: true });
  const byWord = new Map(withUntagged.entries.map((entry) => [entry.word, entry]));
  assert.equal(byWord.get("无标签词").category, "fraud");
  assert.equal(byWord.get("无标签冰毒").category, "drugs", "无标签词也要走违法细分");
});

test("excludeCategories：默认排除 politics 与 violence 后输出里不含这两类", () => {
  const { entries } = buildCandidates({
    // 暴恐词由部署侧注入（仓库里没有这份清单）：这里显式喂一条，才能证明
    // "violence 类目确实会被排除"，而不是"根本没产生 violence 类目"。
    violenceHints: ["违禁品"],
    tags: new Map([
      ["某政治词", ["0"]],
      ["某违禁品", ["4"]],
      ["冰毒", ["1"]],
      ["成人视频", ["2"]],
      ["某赌博词", ["3"]],
    ]),
  });
  // 先确认输入确实产生了 violence 类目（否则下面的排除断言是空转）。
  assert.equal(entries.some((entry) => entry.category === "violence"), true);
  const kept = excludeCategories(entries, ["politics", "violence"]);
  const categories = new Set(kept.map((entry) => entry.category));
  assert.equal(categories.has("politics"), false);
  assert.equal(categories.has("violence"), false);
  assert.deepEqual([...categories].sort(), ["drugs", "gambling", "porn"]);
});

test("selectWithinLimit：按类目比例截断，总数不超上限且七个类目都不被一刀切掉", () => {
  const entries = [];
  const sizes = { porn: 600, fraud: 300, drugs: 60, gambling: 20, fake_docs: 15, politics: 4, violence: 1 };
  for (const [category, size] of Object.entries(sizes)) {
    for (let i = 0; i < size; i += 1) {
      entries.push({ word: `${category}-${String(i).padStart(4, "0")}`, tier: "watch", category });
    }
  }
  const selected = selectWithinLimit(entries, 100);
  assert.equal(selected.length, 100);
  const counts = summarize(selected).categories;
  assert.equal(counts.porn, 60);
  assert.equal(counts.fraud, 30);
  assert.equal(counts.drugs, 6);
  assert.equal(counts.gambling, 2);
  assert.equal(counts.fake_docs, 2, "小类目先按比例取整，再由余数补足");
  assert.equal(counts.politics, 0, "余数不够时按类目顺序补：politics 排在 fake_docs 之后");
  assert.equal(counts.violence, 0);
  // 可复现：同样的输入两次结果一致。
  assert.deepEqual(
    selectWithinLimit(entries, 100).map((entry) => entry.word),
    selected.map((entry) => entry.word),
  );
});

test("selectWithinLimit：不超上限时原样返回（只排序）", () => {
  const entries = [
    { word: "乙词", tier: "watch", category: "porn" },
    { word: "甲词", tier: "block", category: "drugs" },
  ];
  assert.deepEqual(selectWithinLimit(entries, 10).map((entry) => entry.word), ["甲词", "乙词"]);
});

test("sortEntries：先按类目固定顺序，再按词条 Unicode", () => {
  const sorted = sortEntries([
    { word: "b", tier: "watch", category: "porn" },
    { word: "a", tier: "block", category: "drugs" },
    { word: "c", tier: "block", category: "drugs" },
  ]);
  assert.deepEqual(sorted.map((entry) => entry.word), ["a", "c", "b"]);
});

test("renderTsv / renderNdjson：两种格式内容等价", () => {
  const entries = [
    { word: "冰毒", tier: "block", category: "drugs" },
    { word: "成人视频", tier: "watch", category: "porn" },
  ];
  assert.equal(renderTsv(entries), "冰毒\tblock\tdrugs\n成人视频\twatch\tporn\n");
  const ndjson = renderNdjson(entries).trim().split("\n").map((line) => JSON.parse(line));
  assert.deepEqual(ndjson, entries);
});

test("summarize：分别统计档位与类目", () => {
  const stats = summarize([
    { word: "甲", tier: "block", category: "drugs" },
    { word: "乙", tier: "watch", category: "porn" },
    { word: "丙", tier: "block", category: "drugs" },
  ]);
  assert.equal(stats.total, 3);
  assert.deepEqual(stats.tiers, { block: 2, watch: 1 });
  assert.equal(stats.categories.drugs, 2);
  assert.equal(stats.categories.porn, 1);
  assert.equal(stats.categories.violence, 0);
});

test("parseArgs：默认值", () => {
  const opts = parseArgs([]);
  assert.equal(opts.format, "tsv");
  assert.equal(opts.limit, 30000);
  assert.equal(opts.minCjk, 2);
  assert.equal(opts.includeUntagged, false);
  assert.equal(opts.includePolitics, false);
  assert.equal(opts.includeViolence, false);
  assert.ok(opts.out.endsWith(".gotmp\\content-filter-words.tsv") || opts.out.endsWith(".gotmp/content-filter-words.tsv"));
});

test("parseArgs：--include-all 打开两个敏感类，--exclude 自定义类目", () => {
  const opts = parseArgs(["--include-all", "--limit", "100", "--format", "ndjson"]);
  assert.equal(opts.includePolitics, true);
  assert.equal(opts.includeViolence, true);
  assert.equal(opts.limit, 100);
  assert.equal(opts.format, "ndjson");
  assert.deepEqual(parseArgs(["--exclude", "porn,gambling"]).exclude, ["porn", "gambling"]);
});

test("parseArgs：非法参数给出中文错误而不是静默忽略", () => {
  assert.throws(() => parseArgs(["--limit", "0"]), /--limit 需要正整数/);
  assert.throws(() => parseArgs(["--format", "csv"]), /--format 只支持/);
  assert.throws(() => parseArgs(["--exclude", "weapons"]), /不认识的类目/);
  assert.throws(() => parseArgs(["--nope"]), /不认识的参数/);
  assert.throws(() => parseArgs(["--source"]), /需要一个值/);
});


