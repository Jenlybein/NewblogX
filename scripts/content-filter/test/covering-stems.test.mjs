// covering-stems 的用例：推导"缺失主干"这条规则最容易出错的地方
// （把组合词当主干、把单字噪音补进来、把已存在的主干重复补一遍）。

import assert from "node:assert/strict";
import { test } from "node:test";

import {
  deriveCoveringStems,
  isCandidateStem,
  parseTsv,
  renderTsv,
  stripChannelSuffix,
} from "../covering-stems.mjs";

// 一份小词表：主干 `冰毒` 已在表里，`仿真气枪` 与 `雷管` 只以变体存在（后者两条变体）。
const entries = [
  { word: "冰毒", tier: "block", category: "drugs" },
  { word: "冰毒qq", tier: "block", category: "drugs" },
  { word: "qq出售冰毒", tier: "block", category: "drugs" },
  { word: "仿真气枪出售", tier: "block", category: "violence" },
  { word: "仿真气枪批发", tier: "block", category: "violence" },
  { word: "雷管出售", tier: "block", category: "violence" },
  { word: "雷管批发", tier: "block", category: "violence" },
];

test("剥后缀：只剥一层，且要求剥完还有内容", () => {
  assert.equal(stripChannelSuffix("仿真气枪出售"), "仿真气枪");
  assert.equal(stripChannelSuffix("冰毒QQ"), "冰毒");
  // 词本身就是后缀（长度相等）→ 不剥，避免剥成空串。
  assert.equal(stripChannelSuffix("出售"), null);
  // 不带后缀 → null。
  assert.equal(stripChannelSuffix("冰毒"), null);
});

test("候选校验：短、纯拉丁、带后缀的都不算主干", () => {
  assert.equal(isCandidateStem("仿真气枪"), true);
  // 1 个中文字 + 拉丁，共 2 个 rune：允许（这正是 k粉 这类词的口径）。
  assert.equal(isCandidateStem("k粉"), true);
  assert.equal(isCandidateStem("a"), false); // 太短
  assert.equal(isCandidateStem("abcdefgh"), false); // 纯拉丁
  assert.equal(isCandidateStem("出售冰毒"), true); // 含 CJK 且不是后缀结尾
  assert.equal(isCandidateStem("冰毒出售"), false); // 自己还带后缀
  assert.equal(isCandidateStem("一二三四五六七八九"), false); // 超过 8 个 rune
});

test("推导：只补缺失的主干，且要求至少两条变体支撑", () => {
  const { stems, stats } = deriveCoveringStems(entries);
  const words = stems.map((stem) => stem.word);

  assert.ok(words.includes("仿真气枪"), "两条变体支撑 → 应补");
  assert.ok(words.includes("雷管"), "主干缺失且有两处变体 → 应补");
  assert.ok(!words.includes("冰毒"), "主干已在表里 → 不该重复补");
  assert.ok(!words.includes("qq出售冰毒"), "剥不出主干的长词条不该被当主干");
  assert.equal(stats.entries, entries.length);
});

test("推导：单条变体不足以支撑（避免把一次性噪音当主干）", () => {
  const { stems } = deriveCoveringStems([
    { word: "稀有词出售", tier: "block", category: "fraud" },
    { word: "别的词", tier: "block", category: "fraud" },
  ]);
  assert.deepEqual(stems, []);
});

test("推导：沿用变体里出现最多的档位与类目", () => {
  const { stems } = deriveCoveringStems([
    { word: "某某药出售", tier: "watch", category: "porn" },
    { word: "某某药批发", tier: "block", category: "drugs" },
    { word: "某某药供应", tier: "block", category: "drugs" },
  ]);
  assert.equal(stems.length, 1);
  assert.equal(stems[0].tier, "block");
  assert.equal(stems[0].category, "drugs");
});

test("TSV 往返：解析与渲染都可被判分导入直接消费", () => {
  const parsed = parseTsv("# 注释\n冰毒\tblock\tdrugs\n\n仿真气枪\tblock\tviolence\n");
  assert.equal(parsed.length, 2);
  assert.equal(parsed[1].word, "仿真气枪");

  const rendered = renderTsv([{ word: "仿真气枪", tier: "block", category: "violence" }]);
  assert.equal(rendered, "仿真气枪\tblock\tviolence\n");
  assert.deepEqual(parseTsv(rendered), [{ word: "仿真气枪", tier: "block", category: "violence" }]);
});


