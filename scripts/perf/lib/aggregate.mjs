// 聚合口径：每个场景每种模式跑 n 次，报告取中位与最差；没测到的次数记 null，
// 不能悄悄当成 0（0 会把"测不出来"伪装成"很快"）。

/**
 * 中位：忽略 null；偶数个取中间两值的均值并四舍五入到整数（毫秒/字节都用整数比较口径）。
 * 全部不可测时返回 null。
 */
export function median(values) {
  const nums = values.filter(isNumber).sort((a, b) => a - b);
  if (nums.length === 0) return null;
  const mid = nums.length / 2;
  if (nums.length % 2 === 1) return nums[Math.floor(mid)];
  return Math.round((nums[mid - 1] + nums[mid]) / 2);
}

/** 最差：最大值；忽略 null；全部不可测时返回 null。 */
export function worst(values) {
  const nums = values.filter(isNumber);
  if (nums.length === 0) return null;
  return Math.max(...nums);
}

/** 中位 / 最差 / 测得次数 / 不可测次数。 */
export function summarize(values) {
  const nums = values.filter(isNumber);
  return {
    measured: nums.length,
    unmeasured: values.length - nums.length,
    median: median(values),
    worst: worst(values),
  };
}

function isNumber(v) {
  return typeof v === "number" && Number.isFinite(v);
}
