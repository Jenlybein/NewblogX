import {
  computed,
  getCurrentScope,
  inject,
  onScopeDispose,
  provide,
  ref,
  type InjectionKey,
  type Ref,
} from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";

// 捏捏（Jelly）的**唯一状态真相源**。
//
// 两个模块的分工：本 composable 决定「现在是什么状态、能不能用、说什么」；
// `components/jelly/JellyLauncher.vue` 只负责把这些渲染成外观、把指针/键盘事件翻译成
// 下面这些动作。AI 面板消费同一个实例：面板打开时读 `isOpen`，动作走 `open` /
// `close`，不再自己维护一套开关。
//
// 为什么抽出状态而不是写在组件里：状态机是这块最容易出错、也最值得测的一块（可用性、
// 气泡节奏、收起持久化、减少动态），而 DOM 不是。这里全部依赖可注入 —— 假存储、
// 假登录态、假 AI 总闸、假 matchMedia —— 单测不碰真实浏览器/网络。
//
// 位置不归这里：角色固定右下角、收起把手固定右边缘垂直中部，都由 CSS 负责；
// 这里不再持久化 / 钳制任何坐标，也就没有「拖动」这件事。
//
// 边界：本模块只做状态，不发请求（流式对话是 AI 服务层的事）。

/** 六种视觉状态：角色外观只认这六个数（`collapsed` = 收进右边缘，只留把手）。 */
export type JellyVisualState =
  | "idle"
  | "hover"
  | "press"
  | "open"
  | "collapsed"
  | "disabled";

/** localStorage / sessionStorage 的最小接口（测试注入内存版）。 */
export interface JellyStorageAdapter {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
}

/** 「减少动态」的注入点：默认取 useMotionPreference()。 */
export interface JellyMotionPreference {
  prefersReducedMotion: Ref<boolean>;
  dispose?: () => void;
}

export interface JellyAssistantOptions {
  /** AI 总闸是否开启；默认读 siteStore.aiInfo?.enable !== false。 */
  isAiEnabled?: () => boolean;
  /** 是否已登录；默认读 authStore.isLoggedIn。 */
  isLoggedIn?: () => boolean;
  /** 未登录时的登录引导（复用既有 auth modal）。 */
  requestLogin?: () => void;
  /** 当前是否在创作区（/studio/**）；那里不渲染捏捏。 */
  isStudioRoute?: () => boolean;
  /** 减少动态取值；默认走 useMotionPreference()。 */
  motion?: JellyMotionPreference;
  /** 收起状态持久化存储；默认 window.localStorage。 */
  localStore?: JellyStorageAdapter | null;
  /** 气泡计数存储；默认 window.sessionStorage。 */
  sessionStore?: JellyStorageAdapter | null;
}

/** 收起状态持久化键：`"1"` = 收起，`"0"`（或其它任何值）= 展开。 */
export const JELLY_DRAWER_COLLAPSED_KEY = "jelly:drawer-collapsed";
/** 气泡计数键：每会话最多两次（首次进入 + 闲置 90s）。 */
export const JELLY_BUBBLE_COUNT_KEY = "jelly:bubble-count";

/** 角色本体边长（px）：根元素尺寸，CSS 固定定位也按它留边。 */
export const JELLY_LAUNCHER_SIZE = 56;
/** 与视口边缘的最小留白（px）：固定定位的 right / bottom 基准。 */
export const JELLY_VIEWPORT_MARGIN = 12;

/** 收起态把手的可见宽度（px）：只留一条薄玻璃边。 */
export const JELLY_HANDLE_WIDTH = 16;
/** 收起态把手的高度（px）。 */
export const JELLY_HANDLE_HEIGHT = 56;
/**
 * 收起态把手的命中区宽度（px）：可见胶囊只有 16px，但按钮取 44px，
 * 透明部分向左延伸 —— 保证命中区 ≥44×44 又不撑大任何布局。
 */
export const JELLY_HANDLE_HIT_WIDTH = 44;
/** 长按唤出的收起按钮自动消失的时间（毫秒）。 */
export const JELLY_CHIP_PIN_MS = 4_000;
/** 收起 / 展开时经 `role="status"` 播的一句状态（复用 2.5s 瞬时提示机制）。 */
export const JELLY_COLLAPSE_HINT = "我先躲起来";
export const JELLY_EXPAND_HINT = "我回来了";

/** 闲置多久后弹第二句气泡（毫秒）。 */
export const JELLY_IDLE_BUBBLE_MS = 90_000;
/** 每会话气泡上限：首次进入 1 次 + 闲置 1 次。 */
export const JELLY_BUBBLE_LIMIT = 2;
/** 气泡文案（按出现顺序取，超出上限不再出现）。 */
export const JELLY_BUBBLE_TEXTS = [
  "捏我一下，问我点什么吧。",
  "还在写？我可以帮你查资料、改语气。",
] as const;

/** AI 总闸关闭时点击说的唯一一句话。 */
export const JELLY_DISABLED_HINT = "AI 还开着小差";
/** 瞬时提示自动消失的时间（毫秒）。 */
export const JELLY_HINT_MS = 2_500;
/** 空闲呼吸一次的周期（4s，与 spec 的 1→1.03 缩放一致）。 */
export const JELLY_BREATH_MS = 4_000;
/** 「睡着」（AI 关闭）时的呼吸周期：更慢，读起来是「慢慢起伏」而不是「还活着」。 */
export const JELLY_DISABLED_BREATH_MS = 8_000;

function readBrowserStorage(kind: "local" | "session"): JellyStorageAdapter | null {
  if (typeof window === "undefined") return null;
  try {
    return kind === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    // 隐私模式/被策略禁用时访问会抛：当作没有存储，功能降级但不崩。
    return null;
  }
}

// 以下默认实现只在组件里跑（Nuxt 自动导入的 store / route 只在应用上下文可用）；
// 单测从不触发它们，全部走注入。
function defaultIsAiEnabled(): boolean {
  return useSiteStore().aiInfo?.enable !== false;
}

function defaultIsLoggedIn(): boolean {
  return useAuthStore().isLoggedIn;
}

function defaultRequestLogin(): void {
  useUiStore().openAuthModal();
}

function defaultIsStudioRoute(): boolean {
  return useRoute().path.startsWith("/studio");
}

export function useJellyAssistant(options: JellyAssistantOptions = {}) {
  const resolveAiEnabled = options.isAiEnabled ?? defaultIsAiEnabled;
  const resolveLoggedIn = options.isLoggedIn ?? defaultIsLoggedIn;
  const resolveRequestLogin = options.requestLogin ?? defaultRequestLogin;
  const resolveStudioRoute = options.isStudioRoute ?? defaultIsStudioRoute;
  const localStore = options.localStore === undefined ? readBrowserStorage("local") : options.localStore;
  const sessionStore =
    options.sessionStore === undefined ? readBrowserStorage("session") : options.sessionStore;

  const motion = options.motion ?? useMotionPreference();
  const isReducedMotion = computed(() => Boolean(motion.prefersReducedMotion.value));
  const shouldAnimate = computed(() => !isReducedMotion.value);
  /** 呼吸周期：减少动态时为 0（组件据此把 animation 关掉，退化成静态贴片）；AI 关闭时更慢。 */
  const breathDurationMs = computed(() => {
    if (isReducedMotion.value) return 0;
    return isAvailable.value ? JELLY_BREATH_MS : JELLY_DISABLED_BREATH_MS;
  });

  const isAvailable = computed(() => resolveAiEnabled());
  const shouldRender = computed(() => !resolveStudioRoute());
  const isOpen = ref(false);
  /**
   * 收起态：本体换成贴右边缘的把手。位置由 CSS 固定，所以收起不借 / 不改任何坐标。
   */
  const isCollapsed = ref(false);
  /** 长按唤出的收起按钮是否还挂着（触摸设备没有 hover）。 */
  const isChipPinned = ref(false);

  // 指针阶段；视觉状态是"可用性 + 按压 + 打开 + 收起 + 悬停"的优先级结果。
  const phase = ref<"idle" | "hover" | "press">("idle");
  // 本次按压已经唤出过长按 chip：松手只是"叫出收起按钮"，永远不是一次点击。
  // chip 会在 4 秒后自动消失（`isChipPinned` 归零），所以不能用它判断松手，
  // 否则"按住超过 4 秒再松手"会意外打开面板（票 426 的验收口径）。
  let suppressTapAfterLongPress = false;

  const state = computed<JellyVisualState>(() => {
    if (!isAvailable.value) return "disabled";
    if (phase.value === "press") return "press";
    if (isOpen.value) return "open";
    if (isCollapsed.value) return "collapsed";
    if (phase.value === "hover") return "hover";
    return "idle";
  });

  /** 瞬时提示（收起 / 展开 / AI 关闭）；过了 JELLY_HINT_MS 自己消失。 */
  const hintText = ref<string | null>(null);
  let hintTimer: ReturnType<typeof setTimeout> | null = null;

  function clearHintTimer() {
    if (hintTimer !== null) {
      clearTimeout(hintTimer);
      hintTimer = null;
    }
  }

  function showHint(text: string) {
    clearHintTimer();
    hintText.value = text;
    hintTimer = setTimeout(() => {
      hintText.value = null;
      hintTimer = null;
    }, JELLY_HINT_MS);
  }

  function dismissHint() {
    clearHintTimer();
    hintText.value = null;
  }

  /** 当前气泡文案（null = 不显示任何气泡）。 */
  const bubbleText = ref<string | null>(null);
  let bubbleCount = 0;
  let idleTimer: ReturnType<typeof setTimeout> | null = null;
  let sessionStarted = false;

  function readBubbleCount(): number {
    const raw = sessionStore?.getItem(JELLY_BUBBLE_COUNT_KEY);
    if (!raw) return 0;
    const parsed = Number.parseInt(raw, 10);
    // 脏数据/负数一律当 0：宁可少弹一次，也不能因为坏计数反复骚扰。
    if (!Number.isFinite(parsed) || parsed <= 0) return 0;
    return parsed;
  }

  function writeBubbleCount(next: number) {
    bubbleCount = next;
    sessionStore?.setItem(JELLY_BUBBLE_COUNT_KEY, String(next));
  }

  function clearIdleTimer() {
    if (idleTimer !== null) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
  }

  function showBubble(text: string) {
    clearIdleTimer();
    bubbleText.value = text;
    writeBubbleCount(bubbleCount + 1);
  }

  /** 排下一次闲置气泡；额度用完就不再排（每会话最多 2 次）。 */
  function scheduleIdleBubble() {
    clearIdleTimer();
    if (!sessionStarted) return;
    if (bubbleCount >= JELLY_BUBBLE_LIMIT) return;
    idleTimer = setTimeout(() => {
      idleTimer = null;
      if (bubbleText.value !== null) return;
      const index = Math.min(bubbleCount, JELLY_BUBBLE_TEXTS.length - 1);
      showBubble(JELLY_BUBBLE_TEXTS[index] ?? JELLY_BUBBLE_TEXTS[0]);
    }, JELLY_IDLE_BUBBLE_MS);
  }

  /** 任何交互都重新开始闲置计时：一直在用的人不该被弹第二次。 */
  function noteActivity() {
    if (!sessionStarted) return;
    scheduleIdleBubble();
  }

  function dismissBubble() {
    if (bubbleText.value === null) return;
    bubbleText.value = null;
    scheduleIdleBubble();
  }

  function startSession() {
    if (sessionStarted) return;
    sessionStarted = true;
    restoreCollapsed();
    bubbleCount = readBubbleCount();

    // 收起态恢复：右边缘不挂半句话 —— 不弹第一句，也不排闲置气泡。
    if (isCollapsed.value) return;

    if (bubbleCount === 0) {
      // 首次进入：立刻弹一次（新访客才知道有这么个入口）。
      showBubble(JELLY_BUBBLE_TEXTS[0]);
      return;
    }

    // 已弹过：本会话只等闲置那一刻，不立刻再弹。
    scheduleIdleBubble();
  }

  function stopSession() {
    sessionStarted = false;
    suppressTapAfterLongPress = false;
    clearIdleTimer();
    clearChipPinTimer();
  }

  function open() {
    if (!isAvailable.value) {
      showHint(JELLY_DISABLED_HINT);
      return;
    }
    isOpen.value = true;
  }

  function close() {
    isOpen.value = false;
  }

  function toggle() {
    if (isOpen.value) {
      close();
    } else {
      open();
    }
  }

  /** 长按唤出的收起按钮自动消失的计时器（4s，`JELLY_CHIP_PIN_MS`）。 */
  let chipPinTimer: ReturnType<typeof setTimeout> | null = null;

  function clearChipPinTimer() {
    if (chipPinTimer !== null) {
      clearTimeout(chipPinTimer);
      chipPinTimer = null;
    }
  }

  /** 长按捏捏唤出收起按钮：挂起就代表"这颗按钮还在"，松手不会变成一次点击。 */
  function pinCollapseChip() {
    clearChipPinTimer();
    isChipPinned.value = true;
    // 长按只会从一次按压里来：把这次按压的点击吃掉，chip 自动消失后松手也不能开面板。
    if (phase.value === "press") suppressTapAfterLongPress = true;
    chipPinTimer = setTimeout(() => {
      chipPinTimer = null;
      isChipPinned.value = false;
    }, JELLY_CHIP_PIN_MS);
  }

  /** 收起按钮退场（Esc / 点收起 / 收起态切换都走这里）。 */
  function dismissCollapseChip() {
    clearChipPinTimer();
    isChipPinned.value = false;
  }

  /**
   * 收起状态持久化：`"1"` = 收起，其它任何取值都按展开。
   *
   * 为什么阈值定得这么死：藏起来比露出来更糟 —— 读不到存储、写入失败、
   * 脏数据（`"true"` / `"1 "` / 空串）时宁可让捏捏露出来，读者至少能找到它。
   */
  function readCollapsed(): boolean {
    try {
      return localStore?.getItem(JELLY_DRAWER_COLLAPSED_KEY) === "1";
    } catch {
      // 隐私模式 / 策略禁用时访问会抛：按展开处理，功能降级但不崩。
      return false;
    }
  }

  function persistCollapsed(collapsed: boolean) {
    try {
      localStore?.setItem(JELLY_DRAWER_COLLAPSED_KEY, collapsed ? "1" : "0");
    } catch {
      // 写不进去就算了：状态仍在内存里，刷新后按展开处理。
    }
  }

  function restoreCollapsed() {
    isCollapsed.value = readCollapsed();
  }

  /**
   * 收进窗口右边缘。
   *
   * - 防御性先关面板：否则会出现"面板悬空、角色不见"；
   * - 位置不归状态层：角色固定右下角、把手固定右边缘中部，收起不产生第二份坐标；
   * - 气泡 / 瞬时提示清空、闲置计时停掉，然后用同一个 `role="status"` 通道播一句状态。
   */
  function collapse() {
    if (isCollapsed.value) return;

    close();
    dismissCollapseChip();
    dismissBubble();
    dismissHint();
    clearIdleTimer();

    isCollapsed.value = true;
    persistCollapsed(true);
    showHint(JELLY_COLLAPSE_HINT);
  }

  /** 从右边缘出来：本体回到固定右下角，并按原节奏重排闲置气泡。 */
  function expand() {
    if (!isCollapsed.value) return;

    dismissCollapseChip();
    isCollapsed.value = false;
    persistCollapsed(false);
    showHint(JELLY_EXPAND_HINT);
    scheduleIdleBubble();
  }

  function activate() {
    noteActivity();

    if (!isAvailable.value) {
      showHint(JELLY_DISABLED_HINT);
      return;
    }

    if (!resolveLoggedIn()) {
      // 未登录：先引导登录（复用既有 auth modal），绝不弹失败面板。
      resolveRequestLogin();
      return;
    }

    dismissBubble();
    dismissHint();
    toggle();
  }

  function pointerEnter() {
    noteActivity();
    if (phase.value === "idle") phase.value = "hover";
  }

  function pointerLeave() {
    if (phase.value === "hover" || phase.value === "press") phase.value = "idle";
  }

  function pointerDown() {
    noteActivity();
    // 新的一次按压：上一次长按的"点击抑制"到此作废。
    suppressTapAfterLongPress = false;
    phase.value = "press";
  }

  function pointerUp() {
    if (phase.value !== "press") return;
    phase.value = "idle";
    // 长按唤出的收起按钮还挂着：松手只是"叫出按钮"，不是打开面板。
    if (isChipPinned.value) {
      suppressTapAfterLongPress = false;
      return;
    }
    // chip 已经过期（长按超过 4 秒）也一样：松手仍然不算点击。
    if (suppressTapAfterLongPress) {
      suppressTapAfterLongPress = false;
      return;
    }
    activate();
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      clearHintTimer();
      clearIdleTimer();
      clearChipPinTimer();
      stopSession();
      motion.dispose?.();
    });
  }

  return {
    // 状态（外观组件 + AI 面板唯一消费对象）
    state,
    isAvailable,
    isOpen,
    isCollapsed,
    isChipPinned,
    isReducedMotion,
    shouldAnimate,
    breathDurationMs,
    shouldRender,
    bubbleText,
    hintText,
    // 动作
    activate,
    open,
    close,
    toggle,
    collapse,
    expand,
    pinCollapseChip,
    dismissCollapseChip,
    pointerEnter,
    pointerLeave,
    pointerDown,
    pointerUp,
    dismissHint,
    dismissBubble,
    startSession,
    stopSession,
  };
}

/** composable 的对外类型：布局、角色组件、AI 面板都消费同一份。 */
export type JellyAssistantApi = ReturnType<typeof useJellyAssistant>;

/**
 * 页面级共享实例的 provide/inject 键。
 *
 * 为什么需要：状态是「唯一真相源」，同一页面里角色（捏捏）和它张开的 AI 面板
 * 必须看**同一个实例** —— 两份各自 new 的话，角色以为面板开着、面板以为关着。
 * 约定：布局里 `useJellyAssistant()` 建一次并 provide；组件优先用注入/传入的实例，
 * 只有独立使用（比如单测）时才自己建一份。
 */
export const JELLY_ASSISTANT_KEY: InjectionKey<JellyAssistantApi> = Symbol("jelly-assistant");

/** 读取布局提供的共享实例；不在组件上下文或没 provide 时返回 null。 */
export function useJellyAssistantContext(): JellyAssistantApi | null {
  return inject(JELLY_ASSISTANT_KEY, null);
}

/** 在布局里建一份并 provide：捏捏本体与 AI 面板都消费这一份。 */
export function provideJellyAssistant(options: JellyAssistantOptions = {}): JellyAssistantApi {
  const assistant = useJellyAssistant(options);
  provide(JELLY_ASSISTANT_KEY, assistant);
  return assistant;
}
