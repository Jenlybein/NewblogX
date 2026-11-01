// 聚合口径的单元测试：中位 / 最差 / 不可测次数。
// 期望值全部手算写死（不通过被测代码反推），保证测试能真的判错。
import { test } from "node:test";
import assert from "node:assert/strict";
import { median, worst, summarize } from "../lib/aggregate.mjs";

test("median：奇数个取中间值", () => {
  assert.equal(median([300, 100, 200]), 200);
});

test("median：偶数个取中间两值的均值（四舍五入到整数）", () => {
  assert.equal(median([100, 200, 300, 400]), 250);
  assert.equal(median([100, 200, 300, 400, 500, 600]), 350);
});

test("median：忽略 null（不可测的运行），只对测到的次数取中位", () => {
  assert.equal(median([null, 120, null]), 120);
  assert.equal(median([null, 100, 200, 300]), 200);
});

test("median：全部不可测时返回 null，不返回 0", () => {
  assert.equal(median([null, null]), null);
  assert.equal(median([]), null);
});

test("worst：取最大值并忽略 null", () => {
  assert.equal(worst([300, 500, 100]), 500);
  assert.equal(worst([null, 400]), 400);
  assert.equal(worst([null, null]), null);
});

test("summarize：区分测得次数与不可测次数", () => {
  assert.deepEqual(summarize([100, null, 300]), {
    measured: 2,
    unmeasured: 1,
    median: 200,
    worst: 300,
  });
});

test("summarize：空输入时四个字段都是 null / 0，而不是 NaN", () => {
  assert.deepEqual(summarize([]), {
    measured: 0,
    unmeasured: 0,
    median: null,
    worst: null,
  });
});
