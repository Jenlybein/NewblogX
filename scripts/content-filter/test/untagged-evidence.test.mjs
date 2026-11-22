import assert from "node:assert/strict";
import { test } from "node:test";

import { buildCandidates, hasCategoryEvidence } from "../lib/wordlist.mjs";

// 上游 dict 里有一批"只出现在黑名单、没进 tags"的完整裸词（雷管、k粉、猎枪）。
// 早年按"无标签就丢"整批扔掉，结果词表里有 `qq提供雷管炸药` 却没有 `雷管`。
// 这里钉住新口径：**有类目证据就收**，没证据仍丢。

test("hasCategoryEvidence：命中触发词才算有证据", () => {
  assert.equal(hasCategoryEvidence("k粉"), true, "毒品触发词");
  assert.equal(hasCategoryEvidence("雷管", ["雷管"]), true, "部署侧暴恐词");
  assert.equal(hasCategoryEvidence("雷管"), false, "没注入暴恐词时不能凭空判类");
  assert.equal(hasCategoryEvidence("随便一个词"), false);
  assert.equal(hasCategoryEvidence("qq提供雷管炸药", ["雷管"]), true);
});

test("buildCandidates：无标签裸词按证据收下，并走同一个类目细分", () => {
  const { entries, dropped } = buildCandidates({
    tags: new Map([["冰毒", ["1"]]]),
    dictWords: ["雷管", "k粉", "qq随便一个垃圾串"],
    violenceHints: ["雷管"],
  });
  const byWord = new Map(entries.map((entry) => [entry.word, entry.category]));
  assert.equal(byWord.get("雷管"), "violence");
  assert.equal(byWord.get("k粉"), "drugs");
  assert.equal(byWord.has("qq随便一个垃圾串"), false, "没有证据的长句垃圾仍要丢");
  assert.equal(dropped.untagged, 1);
  assert.equal(dropped.untaggedEvidence, 2);
});
