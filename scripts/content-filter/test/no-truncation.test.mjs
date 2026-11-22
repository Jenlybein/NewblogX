import assert from "node:assert/strict";
import { test } from "node:test";

import { selectWithinLimit } from "../lib/wordlist.mjs";

// 默认不截断（2026-11-22 起）：limit<=0 表示"全收，只排序"。
// 早年默认 30000，按类目比例、类目内按 Unicode 排序砍 —— 不看重要性，会砍掉主干
// 却留下"主干+渠道词"的变体，直接制造出"写变体才命中、写主干不命中"的漏洞。

test("selectWithinLimit：limit 0 / 负数 / 非数字 都表示不截断", () => {
  const entries = [
    { word: "乙词", tier: "block", category: "drugs" },
    { word: "甲词", tier: "block", category: "drugs" },
  ];
  for (const limit of [0, -1, Number.NaN]) {
    assert.deepEqual(
      selectWithinLimit(entries, limit).map((entry) => entry.word),
      ["乙词", "甲词"], // 按词条 Unicode 升序：乙(U+4E59) < 甲(U+7532)
      `limit=${limit} 时应全收且只排序`,
    );
  }
});

test("selectWithinLimit：给正数上限才截断", () => {
  const entries = Array.from({ length: 10 }, (_, i) => ({
    word: `词${String.fromCharCode(0x4e00 + i)}`,
    tier: "block",
    category: "drugs",
  }));
  assert.equal(selectWithinLimit(entries, 3).length, 3);
});

