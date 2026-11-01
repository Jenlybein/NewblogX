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

/**
 * 目录跟随高亮时的滚动决策（纯函数）。
 *
 * # 为什么必须有这个
 *
 * 目录原先直接调 `scrollIntoView` 跟随高亮。该 API 会滚动**所有祖先滚动容器，
 * 包括整个 document**：宽屏时侧栏 sticky、目录常驻视口，于是它对整页是空操作，
 * 缺陷被掩盖；窄屏侧栏变 static、目录掉到文档底部，它就开始把整页往下拽，
 * 滚动又触发新的高亮、再拽一次——表现为"页面不停自己下跳"。
 *
 * 因此跟随只允许**计算目录自身容器的 scrollTop**，绝不允许触碰整页滚动位置。
 *
 * # 为什么不直接返回"滚动到某个位置"
 *
 * 调用方拿到的是"要不要滚、滚到哪里"。是否真的滚由调用方执行，这样纯函数
 * 不需要 DOM，也就不需要伪造浏览器环境就能覆盖回环条件。
 */
export interface TocFollowInput {
  /** 目录是否可见（窄屏下不渲染目录时为 false）。 */
  visible: boolean;
  /**
   * 是否处于程序化滚动的抑制窗口内。
   * 刚自己滚过就别再跟着滚，否则"滚动 → 高亮变化 → 又滚动"会形成闭环。
   */
  suppressed: boolean;
  /** 目标链接相对目录容器顶部的偏移（px）。 */
  offsetTop: number;
  /** 目录容器当前滚动位置（px）。 */
  scrollTop: number;
  /** 目录容器可视高度（px）。 */
  clientHeight: number;
  /** 目标链接自身高度（px）。 */
  targetHeight: number;
}

export interface TocFollowResult {
  /** 是否需要滚动目录容器。 */
  follow: boolean;
  /** 目标 scrollTop；follow 为 false 时为 null。 */
  scrollTop: number | null;
}

export function resolveTocFollow(input: TocFollowInput): TocFollowResult {
  // 目录不可见时连 DOM 都不该有，更不该滚动任何东西。
  if (!input.visible || input.suppressed) {
    return { follow: false, scrollTop: null };
  }

  const viewTop = input.scrollTop;
  const viewBottom = input.scrollTop + input.clientHeight;
  const targetTop = input.offsetTop;
  const targetBottom = input.offsetTop + input.targetHeight;

  // 已经完整可见：什么都不做，避免无谓滚动（也是回环的起点）。
  if (targetTop >= viewTop && targetBottom <= viewBottom) {
    return { follow: false, scrollTop: null };
  }

  // 只算目录容器自己的 scrollTop：向下滚时让目标底部贴住容器底部，
  // 向上滚时让目标顶部贴住容器顶部。永不涉及 document。
  const next =
    targetTop < viewTop
      ? targetTop
      : targetBottom - input.clientHeight;

  return { follow: true, scrollTop: Math.max(0, next) };
}
