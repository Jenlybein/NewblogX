import { motionDurationMs } from "~/utils/motion";

/**
 * 光标挂件的进出场状态机（纯函数）：移出即渐隐、移回满 0.1 秒再渐显。
 *
 * 为什么单独成模块并且做成纯函数：这三条要求在接线里几乎无法断言 ——
 * 「移出就渐隐」「移回等 0.1 秒」「0.1 秒内再次移出就取消显示」全是时间上的
 * 状态迁移。把时钟做成参数之后，"满 0.1 秒"这种边界可以在毫秒级精确断言，
 * 不必依赖真浏览器、也不必真的等 0.1 秒。`decoCharm.ts` 只放物理与参数，
 * 这里只放可见性；两者都由组合式接线，互不引用。
 *
 * 状态迁移（`nowMs` 是统一的毫秒时钟）：
 *
 * ```
 *   hidden --进入(首次登场)-------------------------> visible
 *   hidden --进入(显示过)----------------------------> awaiting-show
 *   visible / awaiting-show --移出-------------------> fading-out
 *   awaiting-show --满 0.1 秒-------------------------> visible
 *   awaiting-show --0.1 秒内再次移出------------------> fading-out（取消显示）
 *   fading-out --渐隐走完-----------------------------> hidden
 * ```
 *
 * 三条容易写错的地方：
 *
 * 1. **移出是"当场"不可见**，不是等渐隐走完才算。渐隐只是 CSS 上的透明度
 *    过渡（快档），状态机这边必须立刻离开可见态 —— 否则"移出"到"渐隐开始"
 *    之间会多出一段仍然可见的时间。
 * 2. **首次登场不走防抖**。页面打开后的第一次进入直接可见：那时没有"移回"
 *    可言，而且它与既有的 200ms 淡入是配对的登场动作。防抖只在"已经显示过、
 *    这次是移回"时生效。
 * 3. **等待渐显期间重复收到"进入"不重置时钟**。指针在窗口里挪动会不断产生
 *    指针事件；若每次都把 0.1 秒重新计时，移回就永远等不到渐显。
 *
 * 「减少动态」不是"把过渡调慢"，是**直接切**：进入即可见、移出即隐藏，
 * 既不经过等待态也不经过渐隐态（ADR-0016 约束 7 的口径）。
 */

/**
 * 移回窗口后等多久才渐显（ms）。
 *
 * 票面口径的 0.1 秒防抖窗口：移回后先等满它再渐显；等待期间再次移出就取消
 * 这次显示。沿窗口边缘反复划过时，每次都短于这个窗口 → 挂件一直不出现，
 * 也就没有"闪一下"的观感。它不进 `deco.charm`：这是交互口径，不是运营旋钮
 * （装饰参数可调，但这种"防抖时长"调错只会变成闪烁）。
 */
export const CHARM_SHOW_DELAY_MS = 100;

/** 挂件的可见性阶段。 */
export type CharmVisibilityPhase = "hidden" | "awaiting-show" | "visible" | "fading-out";

/** 进出场状态快照（不可变：每个迁移都返回新对象，便于逐点断言）。 */
export interface CharmVisibilityState {
  /** 当前阶段。 */
  phase: CharmVisibilityPhase;
  /** 允许渐显的绝对时刻（ms）；只有 `awaiting-show` 有值。 */
  showAtMs: number | null;
  /** 渐隐结束的绝对时刻（ms）；只有 `fading-out` 有值。 */
  fadeEndAtMs: number | null;
  /**
   * 是否曾经显示过。
   *
   * 首次登场直接可见（不走防抖），之后每次移回才受 0.1 秒防抖约束 ——
   * 这个标记就是两者的分界，不能只看"当前是否可见"（渐隐 / 等待时都不可见）。
   */
  hasShown: boolean;
}

/** 迁移的输入偏好。 */
export interface CharmVisibilityOptions {
  /** 系统要求减少动态：直接切可见性，不做过渡。 */
  reducedMotion?: boolean;
}

/** 初始状态：从未显示过、隐藏。 */
export function createCharmVisibilityState(): CharmVisibilityState {
  return { phase: "hidden", showAtMs: null, fadeEndAtMs: null, hasShown: false };
}

/** 当前是否在屏幕上（模板据此挂 `.charm-layer--visible` 与淡入淡出方向）。 */
export function charmVisibilityVisible(state: CharmVisibilityState): boolean {
  return state.phase === "visible";
}

/**
 * 是否还在过渡窗口里（等待渐显 / 渐隐中）。
 *
 * 循环保活的唯一依据：这两个阶段有"到点自动迁移"的截止时刻，若链已经收敛、
 * 动画帧又被停掉，迁移就永远不会发生。保活窗口只有 0.1s 与一个快档渐隐，
 * 不会把"收敛后零帧"变成常驻。
 */
export function charmVisibilityBusy(state: CharmVisibilityState): boolean {
  return state.phase === "awaiting-show" || state.phase === "fading-out";
}

/** 直接可见（首次登场 / 减少动态）。 */
function visibleState(): CharmVisibilityState {
  return { phase: "visible", showAtMs: null, fadeEndAtMs: null, hasShown: true };
}

/** 直接隐藏（减少动态 / 渐隐走完）；保留"曾经显示过"。 */
function hiddenState(state: CharmVisibilityState): CharmVisibilityState {
  return { phase: "hidden", showAtMs: null, fadeEndAtMs: null, hasShown: state.hasShown };
}

/**
 * 指针进入窗口（首次移动 / 移回）。
 *
 * - 已可见：原样返回（指针在窗口里挪动会不断产生进入信号，不该有副作用）；
 * - 正在等待渐显：原样返回，**不重置** 0.1 秒时钟（否则移回永远等不到渐显）；
 * - 减少动态或首次登场：直接可见；
 * - 其余（隐藏 / 渐隐中，且显示过）：进入等待渐显。
 */
export function charmVisibilityEnter(
  state: CharmVisibilityState,
  nowMs: number,
  options: CharmVisibilityOptions = {},
): CharmVisibilityState {
  if (state.phase === "visible" || state.phase === "awaiting-show") return state;
  if (options.reducedMotion || !state.hasShown) return visibleState();
  return {
    phase: "awaiting-show",
    showAtMs: nowMs + CHARM_SHOW_DELAY_MS,
    fadeEndAtMs: null,
    hasShown: state.hasShown,
  };
}

/**
 * 指针移出窗口：可见态**当场**结束，进入渐隐。
 *
 * 等待渐显期间移出 = 取消这次显示（回到渐隐，`showAtMs` 清空），这是"不闪烁"
 * 的关键：沿边缘反复划过时，每次短暂的移回都不会兑现成一次渐显。
 *
 * 已经在渐隐 / 隐藏时再收到移出是幂等的（不重启、也不延长渐隐）。
 * 减少动态下直接落到隐藏，不经过渐隐（不做过渡）。
 */
export function charmVisibilityLeave(
  state: CharmVisibilityState,
  nowMs: number,
  options: CharmVisibilityOptions = {},
): CharmVisibilityState {
  if (options.reducedMotion) return hiddenState(state);
  if (state.phase !== "visible" && state.phase !== "awaiting-show") return state;
  return {
    phase: "fading-out",
    showAtMs: null,
    // 渐隐时长取动效预算里的快档，与组件 CSS 的 `--motion-fast` 同源。
    fadeEndAtMs: nowMs + motionDurationMs("fast", false),
    hasShown: state.hasShown,
  };
}

/**
 * 时间向前推进：等待渐显到点 → 可见；渐隐到点 → 隐藏。
 *
 * 两个截止时刻都是绝对时间，重复推进（每帧都调）是幂等的。没到点、或者
 * 当前阶段没有截止时刻时原样返回。
 */
export function charmVisibilityAdvance(
  state: CharmVisibilityState,
  nowMs: number,
): CharmVisibilityState {
  if (state.phase === "awaiting-show" && state.showAtMs !== null && nowMs >= state.showAtMs) {
    return visibleState();
  }
  if (state.phase === "fading-out" && state.fadeEndAtMs !== null && nowMs >= state.fadeEndAtMs) {
    return hiddenState(state);
  }
  return state;
}
