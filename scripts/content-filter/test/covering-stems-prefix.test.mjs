import assert from "node:assert/strict";
import { test } from "node:test";

import { candidateStemsOf, stripChannelPrefix } from "../covering-stems.mjs";

// 组合词是双向的：`冰毒qq` 主干在前，`出售雷管` 主干在后。
// 只剥尾部永远推不出 `出售雷管` 这类词的主干。

test("剥前缀：连续剥最多三层，剥完还有内容", () => {
  assert.equal(stripChannelPrefix("出售雷管"), "雷管");
  assert.equal(stripChannelPrefix("qq提供雷管炸药"), "雷管炸药");
  assert.equal(stripChannelPrefix("供应出售批发冰毒"), "冰毒");
  assert.equal(stripChannelPrefix("雷管出售"), null); // 前缀没有渠道词
});

test("候选集合：一条词条可同时推出前缀与后缀主干", () => {
  const stems = candidateStemsOf("出售雷管qq");
  assert.ok(stems.includes("雷管qq"), "剥前缀应得到 雷管qq");
  assert.ok(stems.includes("出售雷管"), "剥后缀应得到 出售雷管");
  assert.ok(stems.includes("雷管"), "两头都剥应得到 雷管");
});
