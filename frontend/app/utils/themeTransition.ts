/**
 * 明暗主题切换的「同帧」守卫（票 428，spec 423 第 4 批第 30 条）。
 *
 * 问题：语义 token 本身是瞬时的（同一个根类换一份取值），但页面上到处是颜色
 * 过渡（`transition-colors`、各组件自己的 120/150/200ms）—— 它们会让各处面按
 * 各自的时长慢慢变，于是出现"页面底已经换色、卡片与结构线还在路上"的半明半暗
 * 瞬间（用户故事第 58 条）。
 *
 * 做法：切换时给根元素临时挂上 `.theme-switching`，CSS 在这个类下把**全站**
 * transition 关掉（只关这一帧，不是永久移除过渡），两帧后摘掉：
 *   1. 第一帧：Vue 的根类更新与浏览器的样式重算都发生在这里；
 *   2. 第二帧：切换后的颜色已经落定，此时再放开过渡不会补播任何动画
 *      （transition 只在属性值变化时启动，恢复 transition 本身不触发）。
 *
 * 连续快速切换时由最后一次收尾（用递增的序号让旧的帧回调让位），避免前一次
 * 的定时器把守卫提前摘掉、露出一个"没有守卫的中间帧"。
 *
 * 只在客户端接触 DOM：SSR 下调用直接跳过。
 */
export const THEME_SWITCH_CLASS = "theme-switching";

/** 守卫覆盖的帧数：切换的那次绘制 + 一帧兜底。 */
export const THEME_SWITCH_FRAMES = 2;

export interface ThemeSwitchEnvironment {
  /** 根元素；显式传 null 可模拟"没有 DOM"。 */
  root?: HTMLElement | null;
  /** 帧调度（测试注入假实现，默认 requestAnimationFrame）。 */
  requestFrame?: (callback: () => void) => void;
}

let currentSwitch = 0;

export function beginThemeSwitch(environment: ThemeSwitchEnvironment = {}): void {
  const root = resolveRoot(environment);
  if (!root) {
    return;
  }

  const frame = environment.requestFrame ?? defaultRequestFrame();
  if (!frame) {
    return;
  }

  currentSwitch += 1;
  const token = currentSwitch;
  root.classList.add(THEME_SWITCH_CLASS);

  let framesLeft = THEME_SWITCH_FRAMES;
  const release = () => {
    // 期间又被切了一次：这次收尾作废，交给新的那一次撤守卫。
    if (token !== currentSwitch) {
      return;
    }
    framesLeft -= 1;
    if (framesLeft > 0) {
      frame(release);
      return;
    }
    root.classList.remove(THEME_SWITCH_CLASS);
  };

  frame(release);
}

function resolveRoot(environment: ThemeSwitchEnvironment): HTMLElement | null {
  if (environment.root !== undefined) {
    return environment.root;
  }
  if (typeof document === "undefined") {
    return null;
  }
  return document.documentElement;
}

function defaultRequestFrame(): ((callback: () => void) => void) | null {
  if (typeof requestAnimationFrame !== "function") {
    return null;
  }
  return (callback) => requestAnimationFrame(() => callback());
}
