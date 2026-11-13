import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import { resolveSpringMotion, SPRING_TRAVEL } from "~/utils/motion";

/**
 * 按钮「磁吸」跟随（自写实现，不引第三方包）。
 *
 * 参照 reactbits.dev/animations/magnet 的效果口径：指针进入按钮外扩的激活区后，
 * 按钮按「指针相对中心的位置」做一个小幅位移，离开激活区再用弹簧收回。
 * 与原版不同的是**按按钮各自吸附**而不是整组平移：阅读页互动条是六个并列的
 * 小按钮，整组平移会让整条一起抖，反而不像"按钮被吸过去"。
 *
 * 约束（与全站动效口径一致）：
 *   - 只写 `transform: translate3d(...)`，不碰布局属性，也不改 `left/top`；
 *   - 位移有上限（默认 8px，远低于 12px 的区块级预算）；
 *   - `prefers-reduced-motion: reduce` 下完全不挂监听、不写样式；
 *   - 指针是 touch/pen 时跳过（触屏没有"悬停磁吸"的语义，只会拖慢点击）；
 *   - 卸载 / 指针离开 / 标签页隐藏时把位移复位并停掉 rAF。
 */

/** 弹簧与位移参数。stiffness/damping/mass 来自 `SPRING_TRAVEL`（不在这里写裸数字）。 */
export interface MagnetOptions {
  /** 强度：位移 = 指针偏移 / strength。越大越"钝"。 */
  strength?: number;
  /** 单轴位移上限（px）。 */
  maxOffset?: number;
  /** 激活区外扩（px）：指针在按钮本体外这么多距离内仍然吸附。 */
  padding?: number;
}

const DEFAULTS = {
  strength: 4,
  maxOffset: 8,
  padding: 18,
} as const;

/** 参与磁吸的按钮选择器：组件里给按钮挂 `data-magnet` 即可。 */
export const MAGNET_SELECTOR = "[data-magnet]";

/** 单轴的磁吸目标位移（纯函数，便于单测）。 */
export function magnetTarget(delta: number, strength: number, maxOffset: number): number {
  if (strength <= 0) return 0;
  const raw = delta / strength;
  if (raw > maxOffset) return maxOffset;
  if (raw < -maxOffset) return -maxOffset;
  return raw;
}

interface MagnetState {
  /** 当前位置。 */
  x: number;
  y: number;
  /** 当前速度。 */
  vx: number;
  vy: number;
  /** 目标位置。 */
  tx: number;
  ty: number;
}

/**
 * 把磁吸行为挂到某个容器的按钮上（容器内所有 `[data-magnet]`）。
 *
 * 返回一个复位函数：需要手动清掉全部位移时调用（也供测试驱动）。
 */
export function useMagnetButtons(
  root: Ref<HTMLElement | null>,
  options: MagnetOptions = {},
): { reset: () => void } {
  const config = { ...DEFAULTS, ...options };
  const { prefersReducedMotion, dispose: disposeMotionQuery } = useMotionPreference();
  // 「减少动态」经共享解析器落地：reduce 时拿到的是 duration 0 的落位过渡，
  // 这里直接把弹簧关掉，不自己再判一次。
  const resolvedSpring = resolveSpringMotion(SPRING_TRAVEL, prefersReducedMotion.value);
  const spring = resolvedSpring.type === "spring" ? resolvedSpring : null;

  const states = new WeakMap<HTMLElement, MagnetState>();
  const buttons: HTMLElement[] = [];
  let raf = 0;
  let lastTs = 0;
  let listening = false;

  function stateOf(el: HTMLElement): MagnetState {
    let state = states.get(el);
    if (!state) {
      state = { x: 0, y: 0, vx: 0, vy: 0, tx: 0, ty: 0 };
      states.set(el, state);
    }
    return state;
  }

  function writeTransform(el: HTMLElement, state: MagnetState): void {
    el.style.transform = `translate3d(${state.x.toFixed(2)}px, ${state.y.toFixed(2)}px, 0)`;
  }

  function settle(el: HTMLElement, state: MagnetState): boolean {
    const settled =
      Math.abs(state.tx - state.x) < 0.01 &&
      Math.abs(state.ty - state.y) < 0.01 &&
      Math.abs(state.vx) < 0.01 &&
      Math.abs(state.vy) < 0.01;
    if (settled) {
      state.x = state.tx;
      state.y = state.ty;
      state.vx = 0;
      state.vy = 0;
      // 回到原位时把内联样式清掉：不在 DOM 上留一个"永远是 0"的 transform，
      // 也避免它变成后续 CSS 过渡的起点。
      if (state.x === 0 && state.y === 0) {
        el.style.transform = "";
      }
    }
    return settled;
  }

  function step(timestamp: number): void {
    if (!spring) {
      raf = 0;
      return;
    }
    const dt = Math.min(Math.max((timestamp - lastTs) / 1000, 0), 0.05);
    lastTs = timestamp;
    let moving = false;

    for (const el of buttons) {
      const state = stateOf(el);
      if (!settle(el, state)) {
        const ax =
          (spring.stiffness * (state.tx - state.x) - spring.damping * state.vx) / spring.mass;
        const ay =
          (spring.stiffness * (state.ty - state.y) - spring.damping * state.vy) / spring.mass;
        state.vx += ax * dt;
        state.vy += ay * dt;
        state.x += state.vx * dt;
        state.y += state.vy * dt;
        moving = true;
      }
      writeTransform(el, state);
    }

    raf = moving ? requestAnimationFrame(step) : 0;
  }

  function kick(): void {
    if (raf !== 0) return;
    lastTs = performance.now();
    raf = requestAnimationFrame(step);
  }

  function reset(): void {
    for (const el of buttons) {
      const state = stateOf(el);
      state.tx = 0;
      state.ty = 0;
      state.vx = 0;
      state.vy = 0;
      state.x = 0;
      state.y = 0;
      el.style.transform = "";
    }
    if (raf !== 0) {
      cancelAnimationFrame(raf);
      raf = 0;
    }
  }

  function handlePointerMove(event: PointerEvent): void {
    // 触屏/笔没有悬停意图：磁吸只会让点击更难对准。
    if (event.pointerType !== "" && event.pointerType !== "mouse") return;

    for (const el of buttons) {
      const rect = el.getBoundingClientRect();
      const centerX = rect.left + rect.width / 2;
      const centerY = rect.top + rect.height / 2;
      const dx = event.clientX - centerX;
      const dy = event.clientY - centerY;
      const activeX = Math.abs(dx) <= rect.width / 2 + config.padding;
      const activeY = Math.abs(dy) <= rect.height / 2 + config.padding;

      const state = stateOf(el);
      if (activeX && activeY) {
        state.tx = magnetTarget(dx, config.strength, config.maxOffset);
        state.ty = magnetTarget(dy, config.strength, config.maxOffset);
      } else {
        state.tx = 0;
        state.ty = 0;
      }
    }
    kick();
  }

  function handlePointerLeave(): void {
    for (const el of buttons) {
      const state = stateOf(el);
      state.tx = 0;
      state.ty = 0;
    }
    kick();
  }

  function handleVisibility(): void {
    if (document.hidden) reset();
  }

  function attach(): void {
    const container = root.value;
    if (!container || listening) return;
    if (prefersReducedMotion.value) return;

    buttons.length = 0;
    container.querySelectorAll<HTMLElement>(MAGNET_SELECTOR).forEach((el) => buttons.push(el));
    if (buttons.length === 0) return;

    container.addEventListener("pointermove", handlePointerMove, { passive: true });
    container.addEventListener("pointerleave", handlePointerLeave, { passive: true });
    document.addEventListener("visibilitychange", handleVisibility);
    listening = true;
  }

  function detach(): void {
    const container = root.value;
    if (listening && container) {
      container.removeEventListener("pointermove", handlePointerMove);
      container.removeEventListener("pointerleave", handlePointerLeave);
      document.removeEventListener("visibilitychange", handleVisibility);
    }
    listening = false;
    buttons.length = 0;
    reset();
  }

  onMounted(() => {
    attach();
  });

  // 「减少动态」是运行期可切换的偏好：切到 reduce 时立刻摘掉监听并复位。
  const stopWatch = watch(
    prefersReducedMotion,
    (reduced) => {
      if (reduced) detach();
      else attach();
    },
    { immediate: false },
  );

  onBeforeUnmount(() => {
    stopWatch();
    disposeMotionQuery();
    detach();
  });

  return { reset };
}
