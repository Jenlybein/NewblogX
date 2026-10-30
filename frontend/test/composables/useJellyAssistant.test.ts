import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref, type Ref } from "vue";

import {
  JELLY_BUBBLE_COUNT_KEY,
  JELLY_BUBBLE_TEXTS,
  JELLY_BREATH_MS,
  JELLY_DISABLED_BREATH_MS,
  JELLY_DISABLED_HINT,
  JELLY_IDLE_BUBBLE_MS,
  JELLY_LAUNCHER_SIZE,
  JELLY_POSITION_KEY,
  JELLY_VIEWPORT_MARGIN,
  provideJellyAssistant,
  useJellyAssistant,
  useJellyAssistantContext,
  type JellyAssistantOptions,
  type JellyPosition,
  type JellyStorageAdapter,
  type JellyVisualState,
} from "~/composables/useJellyAssistant";

// useJellyAssistant 的用例（票 #9 的 S1 接缝）。
//
// 这个 composable 是「捏捏」的**唯一状态真相源**：外观组件只读它的状态、只调它的动作，
// #10 的 AI 面板也消费同一个实例。所以这里不测 DOM、不测样式，只钉对外可观察的行为：
//   - 六种视觉状态怎么切、谁压过谁（状态机）；
//   - 可用性两条不可用路径（AI 总闸关闭、未登录）不能变成失败面板；
//   - 气泡节奏（每会话 ≤2、闲置 90s、点击即关）；
//   - 位置持久化与视口钳制（刷新回原位、越界收回来）；
//   - 「减少动态」的取值与静态退化。
//
// 依赖全部注入：假存储、假视口、假登录态、假 AI 总闸、假 matchMedia —— 不碰真实网络/浏览器。

/** 内存版 Storage（localStorage / sessionStorage 的最小接口）。 */
function memoryStorage(initial: Record<string, string> = {}): JellyStorageAdapter & {
  read: (key: string) => string | null;
} {
  const map = new Map(Object.entries(initial));
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => {
      map.set(key, value);
    },
    read: (key: string) => map.get(key) ?? null,
  };
}

interface HarnessOptions {
  aiEnabled?: boolean;
  loggedIn?: boolean;
  studioRoute?: boolean;
  reducedMotion?: boolean;
  local?: JellyStorageAdapter;
  session?: JellyStorageAdapter;
  viewport?: { width: number; height: number };
}

interface Harness {
  api: ReturnType<typeof useJellyAssistant>;
  local: JellyStorageAdapter;
  session: JellyStorageAdapter;
  requestLogin: ReturnType<typeof vi.fn>;
  motion: Ref<boolean>;
  nextViewport: (size: { width: number; height: number }) => void;
}

function harness(options: HarnessOptions = {}): Harness {
  const local = options.local ?? memoryStorage();
  const session = options.session ?? memoryStorage();
  const requestLogin = vi.fn();
  const motion = ref(options.reducedMotion ?? false);
  let viewport = options.viewport ?? { width: 1024, height: 768 };

  const api = useJellyAssistant({
    isAiEnabled: () => options.aiEnabled ?? true,
    isLoggedIn: () => options.loggedIn ?? true,
    requestLogin,
    isStudioRoute: () => options.studioRoute ?? false,
    motion: { prefersReducedMotion: motion },
    localStore: local,
    sessionStore: session,
    viewport: () => viewport,
  } satisfies JellyAssistantOptions);

  return {
    api,
    local,
    session,
    requestLogin,
    motion,
    nextViewport: (size) => {
      viewport = size;
    },
  };
}

function expectState(api: { state: Ref<JellyVisualState> }, expected: JellyVisualState) {
  expect(api.state.value).toBe(expected);
}

describe("useJellyAssistant · 可用性与开关", () => {
  it("默认是空闲态、面板未打开、AI 可用", () => {
    const { api } = harness();

    expectState(api, "idle");
    expect(api.isOpen.value).toBe(false);
    expect(api.isAvailable.value).toBe(true);
  });

  it("AI 总闸关闭（aiInfo.enable === false）时呈 disabled 态，且压过悬停/按压", () => {
    const { api } = harness({ aiEnabled: false });

    expectState(api, "disabled");
    expect(api.isAvailable.value).toBe(false);

    api.pointerEnter();
    expectState(api, "disabled");

    api.pointerDown();
    expectState(api, "disabled");
  });

  it("AI 关闭时点击只说原因，不打开面板，提示过一会儿自己消失", () => {
    vi.useFakeTimers();
    const { api } = harness({ aiEnabled: false });

    api.activate();

    expect(api.isOpen.value).toBe(false);
    expect(api.hintText.value).toBe(JELLY_DISABLED_HINT);

    vi.advanceTimersByTime(5000);
    expect(api.hintText.value).toBeNull();
  });

  it("AI 关闭时鼠标「按下再松开」也说原因（不是一颗死按钮）", () => {
    const { api } = harness({ aiEnabled: false });

    api.pointerDown();
    api.pointerUp();

    expect(api.hintText.value).toBe(JELLY_DISABLED_HINT);
    expect(api.isOpen.value).toBe(false);
  });

  it("点击打开面板：isOpen 变 true、状态是 open；再点一次关闭", () => {
    const { api } = harness();

    api.activate();
    expect(api.isOpen.value).toBe(true);
    expectState(api, "open");

    api.activate();
    expect(api.isOpen.value).toBe(false);
    expectState(api, "idle");
  });

  it("open() / close() / toggle() 三个动作都只改 isOpen", () => {
    const { api } = harness();

    api.open();
    expect(api.isOpen.value).toBe(true);
    expectState(api, "open");

    api.close();
    expect(api.isOpen.value).toBe(false);
    expectState(api, "idle");

    api.toggle();
    expect(api.isOpen.value).toBe(true);
  });

  it("open 压过 hover：面板打开后指针停在把手上，状态不能倒回 hover（#10 要的把手态）", () => {
    const { api } = harness();

    api.open();
    api.pointerEnter();

    expectState(api, "open");
  });
});

describe("useJellyAssistant · 未登录路径", () => {
  it("未登录时点击先引导登录，不打开面板、不出现失败态", () => {
    const { api, requestLogin } = harness({ loggedIn: false });

    api.activate();

    expect(requestLogin).toHaveBeenCalledTimes(1);
    expect(api.isOpen.value).toBe(false);
    expectState(api, "idle");
  });

  it("登录成功后同一个实例可以正常打开（登录态是按需读取的，不缓存死）", () => {
    let loggedIn = false;
    const requestLogin = vi.fn(() => {
      loggedIn = true;
    });
    const api = useJellyAssistant({
      isAiEnabled: () => true,
      isLoggedIn: () => loggedIn,
      requestLogin,
      isStudioRoute: () => false,
      motion: { prefersReducedMotion: ref(false) },
      localStore: memoryStorage(),
      sessionStore: memoryStorage(),
      viewport: () => ({ width: 1024, height: 768 }),
    } satisfies JellyAssistantOptions);

    api.activate();
    expect(api.isOpen.value).toBe(false);

    api.activate();
    expect(api.isOpen.value).toBe(true);
  });
});

describe("useJellyAssistant · 六态切换", () => {
  it("悬停 → hover，离开 → idle", () => {
    const { api } = harness();

    api.pointerEnter();
    expectState(api, "hover");

    api.pointerLeave();
    expectState(api, "idle");
  });

  it("按下 → press，松开（没拖动）→ 先捏再触发：打开面板", () => {
    const { api } = harness();

    api.pointerDown();
    expectState(api, "press");

    api.pointerUp();
    expect(api.isOpen.value).toBe(true);
    expectState(api, "open");
  });

  it("拖拽中 → dragging，压过 hover / press", () => {
    const { api } = harness();

    api.pointerEnter();
    api.pointerDown();
    api.beginDrag();
    expectState(api, "dragging");

    api.endDrag();
    expectState(api, "idle");
  });

  it("拖动结束时不会被当成一次点击（松手不开面板）", () => {
    const { api } = harness();

    api.pointerDown();
    api.beginDrag();
    api.endDrag();
    api.pointerUp();

    expect(api.isOpen.value).toBe(false);
    expect(api.isDragged.value).toBe(false);
  });
});

describe("useJellyAssistant · 创作区不渲染", () => {
  it("/studio/** 下 shouldRender 为 false，读者站为 true", () => {
    const reader = harness({ studioRoute: false });
    const studio = harness({ studioRoute: true });

    expect(reader.api.shouldRender.value).toBe(true);
    expect(studio.api.shouldRender.value).toBe(false);
  });
});

describe("useJellyAssistant · 位置默认值", () => {
  it("未持久化时落在右下角（视口内留边）", () => {
    const { api } = harness();

    expect(api.position.value).toEqual(defaultLauncherPosition());
  });
});

describe("useJellyAssistant · 气泡节奏（每会话 ≤2）", () => {
  it("首次进入弹一次气泡，并把计数写进 sessionStorage", () => {
    const { api, session } = harness();

    api.startSession();

    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[0]);
    expect(session.getItem(JELLY_BUBBLE_COUNT_KEY)).toBe("1");
  });

  it("点击气泡即关（关掉不重复弹同一句）", () => {
    const { api } = harness();

    api.startSession();
    api.dismissBubble();

    expect(api.bubbleText.value).toBeNull();
  });

  it("闲置 90s 弹第二句，之后同一会话不再打扰", () => {
    vi.useFakeTimers();
    const { api, session } = harness();

    api.startSession();
    api.dismissBubble();

    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS);
    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[1]);
    expect(session.getItem(JELLY_BUBBLE_COUNT_KEY)).toBe("2");

    api.dismissBubble();
    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 3);
    expect(api.bubbleText.value).toBeNull();
  });

  it("本会话已经用完了两次额度：进入时直接安静", () => {
    const session = memoryStorage({ [JELLY_BUBBLE_COUNT_KEY]: "2" });
    const { api } = harness({ session });

    api.startSession();
    expect(api.bubbleText.value).toBeNull();
  });

  it("本会话已弹过一次：进入不立刻骚扰，只等闲置那一刻", () => {
    vi.useFakeTimers();
    const session = memoryStorage({ [JELLY_BUBBLE_COUNT_KEY]: "1" });
    const { api } = harness({ session });

    api.startSession();
    expect(api.bubbleText.value).toBeNull();

    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS);
    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[1]);
  });

  it("计数坏掉（脏数据）时按 0 处理，不崩", () => {
    const session = memoryStorage({ [JELLY_BUBBLE_COUNT_KEY]: "not-a-number" });
    const { api } = harness({ session });

    api.startSession();

    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[0]);
  });

  it("有交互就把闲置计时重新开始（一直用的人不会被弹第二次）", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.startSession();
    api.dismissBubble();

    vi.advanceTimersByTime(60_000);
    api.pointerEnter();
    expect(api.bubbleText.value).toBeNull();

    vi.advanceTimersByTime(60_000);
    expect(api.bubbleText.value).toBeNull();

    vi.advanceTimersByTime(30_000);
    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[1]);
  });

  it("卸载（stopSession）后定时器不再弹气泡", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.startSession();
    api.dismissBubble();
    api.stopSession();

    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 2);
    expect(api.bubbleText.value).toBeNull();
  });
});

describe("useJellyAssistant · 位置持久化与视口钳制", () => {
  it("拖拽结束后写 localStorage；换一个实例（模拟刷新）恢复原位", () => {
    const local = memoryStorage();
    const first = harness({ local });

    first.api.beginDrag();
    first.api.dragTo({ x: 160, y: 220 });
    first.api.endDrag();

    expect(JSON.parse(local.getItem(JELLY_POSITION_KEY) || "{}")).toEqual({ x: 160, y: 220 });

    const refreshed = harness({ local });
    refreshed.api.startSession();
    expect(refreshed.api.position.value).toEqual({ x: 160, y: 220 });
  });

  it("拖到视口外会被钳制回视口（右下角左上角都收得住）", () => {
    const { api } = harness();

    api.beginDrag();
    api.dragTo({ x: 9_999, y: 9_999 });
    expect(api.position.value).toEqual({ x: 956, y: 700 });

    api.dragTo({ x: -9_999, y: -9_999 });
    expect(api.position.value).toEqual({ x: JELLY_VIEWPORT_MARGIN, y: JELLY_VIEWPORT_MARGIN });
  });

  it("恢复时把存下来的越界位置钳制回当前视口（换小屏后仍点得到）", () => {
    const local = memoryStorage({
      [JELLY_POSITION_KEY]: JSON.stringify({ x: 5_000, y: -40 }),
    });
    const { api } = harness({ local });

    api.startSession();
    expect(api.position.value).toEqual({ x: 956, y: JELLY_VIEWPORT_MARGIN });
  });

  it("视口变小后重新钳制（避免角色跑到屏幕外点不到）", () => {
    const view = { width: 1024, height: 768 };
    const local = memoryStorage({ [JELLY_POSITION_KEY]: JSON.stringify({ x: 956, y: 700 }) });
    const { api, nextViewport } = harness({ local, viewport: view });

    api.startSession();
    expect(api.position.value).toEqual({ x: 956, y: 700 });

    nextViewport({ width: 400, height: 300 });
    api.handleViewportResize();
    expect(api.position.value).toEqual({ x: 332, y: 232 });
  });

  it("存储里是脏 JSON / 缺字段时回落默认位置，不崩", () => {
    const local = memoryStorage({ [JELLY_POSITION_KEY]: "{oops" });
    const { api } = harness({ local });

    api.startSession();
    expect(api.position.value).toEqual(defaultLauncherPosition());
  });

  it("位置永远是整数（避免半像素让拖拽抖动）", () => {
    const { api } = harness();

    api.beginDrag();
    api.dragTo({ x: 110.4, y: 220.7 });

    expect(api.position.value).toEqual({ x: 110, y: 221 });
  });

  it("没有可用存储时不影响拖拽（隐私模式降级）", () => {
    const { api } = harness({ local: null });

    api.beginDrag();
    api.dragTo({ x: 100, y: 100 });
    api.endDrag();

    expect(api.position.value).toEqual({ x: 100, y: 100 });
  });
});

describe("useJellyAssistant · 减少动态", () => {
  it("注入「减少动态」为 true 时：状态不变，但动效退化为静态", () => {
    const { api } = harness({ reducedMotion: true });

    expect(api.isReducedMotion.value).toBe(true);
    expect(api.shouldAnimate.value).toBe(false);
    expect(api.breathDurationMs.value).toBe(0);
    expect(api.state.value).toBe("idle");
  });

  it("默认走 #3 的 useMotionPreference（读系统设置）", () => {
    const quiet = useJellyAssistant();

    expect(quiet.isReducedMotion.value).toBe(false);
    expect(quiet.shouldAnimate.value).toBe(true);
  });

  it("系统设置真的关掉动效时取 true（不是写死 false 的假分支）", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    }));

    const reduced = useJellyAssistant();

    expect(reduced.isReducedMotion.value).toBe(true);
    expect(reduced.breathDurationMs.value).toBe(0);
  });

  it("AI 关闭时呼吸更慢（「睡着」而不是「还活着」）", () => {
    const { api } = harness({ aiEnabled: false });

    expect(api.breathDurationMs.value).toBe(JELLY_DISABLED_BREATH_MS);
  });

  it("系统设置变化时跟着变（不用重新加载页面）", () => {
    const { api, motion } = harness();

    expect(api.shouldAnimate.value).toBe(true);
    expect(api.breathDurationMs.value).toBe(JELLY_BREATH_MS);

    motion.value = true;
    expect(api.shouldAnimate.value).toBe(false);
    expect(api.breathDurationMs.value).toBe(0);
  });
});

describe("useJellyAssistant · 页面级共享实例（#10 的接缝）", () => {
  it("provideJellyAssistant 提供的实例能被 useJellyAssistantContext 读到（同一对象）", () => {
    let provided: ReturnType<typeof provideJellyAssistant> | null = null;
    let injected: ReturnType<typeof useJellyAssistantContext> = null;

    const Child = defineComponent({
      setup() {
        injected = useJellyAssistantContext();
        return () => h("span");
      },
    });

    const Parent = defineComponent({
      setup() {
        provided = provideJellyAssistant({
          isAiEnabled: () => true,
          isLoggedIn: () => true,
          requestLogin: () => undefined,
          isStudioRoute: () => false,
          motion: { prefersReducedMotion: ref(false) },
          localStore: memoryStorage(),
          sessionStore: memoryStorage(),
          viewport: () => ({ width: 1024, height: 768 }),
        });
        return () => h(Child);
      },
    });

    mount(Parent);

    expect(provided).not.toBeNull();
    expect(injected).toBe(provided);
  });

  it("没有布局提供实例时返回 null（组件可以退回自己建一份）", () => {
    let injected: ReturnType<typeof useJellyAssistantContext> = null;
    const Standalone = defineComponent({
      setup() {
        injected = useJellyAssistantContext();
        return () => h("span");
      },
    });

    mount(Standalone);

    expect(injected).toBeNull();
  });
});

/** 右下角默认位置（与常量同一套口径，但这里写死数值，防止实现改了常量还自洽）。 */
function defaultLauncherPosition(): JellyPosition {
  return {
    x: 1024 - JELLY_LAUNCHER_SIZE - JELLY_VIEWPORT_MARGIN,
    y: 768 - JELLY_LAUNCHER_SIZE - JELLY_VIEWPORT_MARGIN,
  };
}
