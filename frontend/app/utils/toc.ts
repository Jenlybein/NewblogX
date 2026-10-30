/**
 * 目录层级缩进与进度文案（纯函数）。
 *
 * ArticleTocAnchor 里的缩进逻辑原来是模板中的 `ml-3 / ml-6` 三元表达式 —— 把级别
 * 映射压成 0/1/2 三档后，组件只负责套 class，边界（异常级别）也能单测。
 */

const MAX_INDENT_LEVEL = 2;

/** h1/h2 顶格、h3 一级、h4+ 两级。 */
export function tocIndentLevel(level: number): number {
  if (level >= 4) {
    return MAX_INDENT_LEVEL;
  }
  if (level === 3) {
    return 1;
  }
  return 0;
}

/** 目录右上角的进度文案（整数百分比，钳制在 0..100）。 */
export function formatProgressPercent(percent: number): string {
  const clamped = Math.min(100, Math.max(0, percent));
  return `${Math.round(clamped)}%`;
}
