import assert from "node:assert/strict";
import { test } from "node:test";

import { deriveCoveringStems, minSupportFor } from "../covering-stems.mjs";

// 两字全中文是完整词（雷管、冰砖），一条变体就算证据；
// 更长的"单变体主干"更可能是长句碎片，仍要求两条。

test("支撑门槛：两字全中文放宽到 1，其余为 2", () => {
  assert.equal(minSupportFor("雷管"), 1);
  assert.equal(minSupportFor("仿真气枪"), 2);
  // 两字但含拉丁：不是"全中文"，不放宽。
  assert.equal(minSupportFor("k粉"), 2);
});

test("推导：两字全中文的单变体主干会被补上", () => {
  const { stems } = deriveCoveringStems([
    { word: "雷管出售", tier: "block", category: "violence" },
    { word: "别的词", tier: "block", category: "fraud" },
  ]);
  assert.deepEqual(stems.map((s) => s.word), ["雷管"]);
  assert.equal(stems[0].category, "violence");
});
