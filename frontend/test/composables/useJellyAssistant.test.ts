import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref, type Ref } from "vue";

import {
  JELLY_BUBBLE_COUNT_KEY,
  JELLY_BUBBLE_LIMIT,
  JELLY_BUBBLE_TEXTS,
  JELLY_BREATH_MS,
  JELLY_CHIP_PIN_MS,
  JELLY_COLLAPSE_HINT,
  JELLY_DISABLED_BREATH_MS,
  JELLY_DISABLED_HINT,
  JELLY_DRAWER_COLLAPSED_KEY,
  JELLY_EXPAND_HINT,
  JELLY_HINT_MS,
  JELLY_IDLE_BUBBLE_MS,
  provideJellyAssistant,
  useJellyAssistant,
  useJellyAssistantContext,
  type JellyAssistantOptions,
  type JellyStorageAdapter,
  type JellyVisualState,
} from "~/composables/useJellyAssistant";

// useJellyAssistant 的用例（票 9 的 S1 接缝）。
//
// 这个 composable 是「捏捏」的**唯一状态真相源**：外观组件只读它的状态、只调它的动作，
// 票 10 的 AI 面板也消费同一个实例。所以这里不测 DOM、不测样式，只钉对外可观察的行为：
//   - 六种视觉状态怎么切、谁压过谁（状态机）；
//   - 可用性两条不可用路径（AI 总闸关闭、未登录）不能变成失败面板；
//   - 气泡节奏（每会话 ≤2、闲置 90s、点击即关）；
//   - 收起 / 展开与状态提示（不再有位置持久化：角色固定右下角，由 CSS 负责定位）；
//   - 「减少动态」的取值与静态退化。
//
// 依赖全部注入：假存储、假登录态、假 AI 总闸、假 matchMedia —— 不碰真实网络/浏览器。

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
}

interface Harness {
  api: ReturnType<typeof useJellyAssistant>;
  local: JellyStorageAdapter;
  session: JellyStorageAdapter;
  requestLogin: ReturnType<typeof vi.fn>;
  motion: Ref<boolean>;
}

function harness(options: HarnessOptions = {}): Harness {
  const local = options.local ?? memoryStorage();
  const session = options.session ?? memoryStorage();
  const requestLogin = vi.fn();
  const motion = ref(options.reducedMotion ?? false);

  const api = useJellyAssistant({
    isAiEnabled: () => options.aiEnabled ?? true,
    isLoggedIn: () => options.loggedIn ?? true,
    requestLogin,
    isStudioRoute: () => options.studioRoute ?? false,
    motion: { prefersReducedMotion: motion },
    localStore: local,
    sessionStore: session,
  } satisfies JellyAssistantOptions);

  return {
    api,
    local,
    session,
    requestLogin,
    motion,
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

  it("open 压过 hover：面板打开后指针停在把手上，状态不能倒回 hover（票 10 要的把手态）", () => {
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
    } satisfies JellyAssistantOptions);

    api.activate();
    expect(api.isOpen.value).toBe(false);

    api.activate();
    expect(api.isOpen.value).toBe(true);
  });
});

describe("useJellyAssistant · 视觉状态切换", () => {
  it("悬停 → hover，离开 → idle", () => {
    const { api } = harness();

    api.pointerEnter();
    expectState(api, "hover");

    api.pointerLeave();
    expectState(api, "idle");
  });

  it("按下 → press，松开（就是一次点击）→ 打开面板", () => {
    const { api } = harness();

    api.pointerDown();
    expectState(api, "press");

    api.pointerUp();
    expect(api.isOpen.value).toBe(true);
    expectState(api, "open");
  });

  it("按压后没有拖拽态可切换：pointerUp 直接结算为一次点击，不再有拖拽分支", () => {
    const { api } = harness();

    api.pointerDown();
    // 旧口径在这里会先进入 dragging；固定定位后 pointerDown/Up 之间没有位置语义。
    api.pointerUp();

    expect(api.isOpen.value).toBe(true);
    expect(api).not.toHaveProperty("isDragged");
    expect(api).not.toHaveProperty("beginDrag");
    expect(api).not.toHaveProperty("endDrag");
  });
});

describe("useJellyAssistant · 收进窗口右边缘（六态）", () => {
  it("collapse() 进入 collapsed 态，压过 hover / idle", () => {
    const { api } = harness();

    api.collapse();
    expect(api.isCollapsed.value).toBe(true);
    expectState(api, "collapsed");

    // collapsed 让位给 press / open，但压过悬停与闲置。
    api.pointerEnter();
    expectState(api, "collapsed");
    api.pointerLeave();
    expectState(api, "collapsed");
  });

  it("收起态让位给按压：pointerDown 进入 press，离开后回到 collapsed", () => {
    const { api } = harness();

    api.collapse();
    expectState(api, "collapsed");

    api.pointerDown();
    expectState(api, "press");

    api.pointerLeave();
    expectState(api, "collapsed");
  });

  it("open 压过 collapsed：收起态下打开面板，关闭后回到 collapsed", () => {
    const { api } = harness();

    api.collapse();
    api.open();

    expect(api.isOpen.value).toBe(true);
    expectState(api, "open");

    api.close();
    expectState(api, "collapsed");
  });

  it("面板开着时 collapse() 先关面板（不留「面板悬空、角色不见」）", () => {
    const { api } = harness();

    api.open();
    expect(api.isOpen.value).toBe(true);

    api.collapse();
    expect(api.isOpen.value).toBe(false);
    expect(api.isCollapsed.value).toBe(true);
    expectState(api, "collapsed");
  });

  it("收起 / 展开只切换 isCollapsed；位置不再由 composable 记账（固定右下角）", () => {
    const { api } = harness();

    api.collapse();
    expect(api.isCollapsed.value).toBe(true);

    api.expand();
    expect(api.isCollapsed.value).toBe(false);
    expectState(api, "idle");

    for (const gone of [
      "position",
      "collapsedPosition",
      "moveTo",
      "dragTo",
      "persistPosition",
      "handleViewportResize",
    ]) {
      expect(api, missingApiMessage(gone)).not.toHaveProperty(gone);
    }
  });
});

describe("useJellyAssistant · 收起状态持久化（脏值一律按展开）", () => {
  it("collapse() / expand() 把 \"1\" / \"0\" 写进同一个 localStore", () => {
    const { api, local } = harness();

    api.collapse();
    expect(local.getItem(JELLY_DRAWER_COLLAPSED_KEY)).toBe("1");

    api.expand();
    expect(local.getItem(JELLY_DRAWER_COLLAPSED_KEY)).toBe("0");
  });

  it("新会话恢复为收起态，且恢复时不排闲置气泡", () => {
    vi.useFakeTimers();
    const local = memoryStorage({ [JELLY_DRAWER_COLLAPSED_KEY]: "1" });
    const { api } = harness({ local });

    api.startSession();

    expect(api.isCollapsed.value).toBe(true);
    expect(api.bubbleText.value).toBeNull();

    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 3);
    expect(api.bubbleText.value).toBeNull();
  });

  it.each<Readonly<[string, string | null]>>([
    ["缺失", null],
    ["显式展开 \"0\"", "0"],
    ["脏值 \"true\"", "true"],
    ["空串", ""],
    ["带空格的 \"1 \"（不是精确的 \"1\"）", "1 "],
  ])("存储里是 %s 时一律按展开处理", (label, stored) => {
    const local = memoryStorage(
      stored === null ? {} : { [JELLY_DRAWER_COLLAPSED_KEY]: stored },
    );
    const { api } = harness({ local });

    api.startSession();

    expect(api.isCollapsed.value, label).toBe(false);
  });

  it("没有可用存储（隐私模式）时按展开；收起/展开仍只是内存状态", () => {
    const { api } = harness({ local: null });

    api.startSession();
    expect(api.isCollapsed.value).toBe(false);

    api.collapse();
    expect(api.isCollapsed.value).toBe(true);

    api.expand();
    expect(api.isCollapsed.value).toBe(false);
  });

  it("读取存储抛异常时按展开，不把异常冒到页面", () => {
    const local: JellyStorageAdapter = {
      getItem: (key) => {
        if (key === JELLY_DRAWER_COLLAPSED_KEY) throw new Error("storage denied");
        return null;
      },
      setItem: () => undefined,
    };
    const { api } = harness({ local });

    api.startSession();

    expect(api.isCollapsed.value).toBe(false);
  });
});

describe("useJellyAssistant · 收起与气泡节奏", () => {
  it("收起时清空当前气泡与瞬时提示、停掉闲置计时，并播一句收起状态", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.startSession();
    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[0]);

    api.dismissBubble();
    vi.advanceTimersByTime(30_000);
    api.collapse();

    expect(api.bubbleText.value).toBeNull();
    expect(api.hintText.value).toBe(JELLY_COLLAPSE_HINT);

    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 2);
    expect(api.bubbleText.value).toBeNull();
    expect(api.hintText.value).toBeNull();
  });

  it("收起态不再弹第 2 句；展开后按原节奏重新排一次（每会话 ≤2 不变）", () => {
    vi.useFakeTimers();
    const { api, session } = harness();

    api.startSession();
    api.dismissBubble();
    api.collapse();
    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 3);
    expect(api.bubbleText.value).toBeNull();

    api.expand();
    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS);
    expect(api.bubbleText.value).toBe(JELLY_BUBBLE_TEXTS[1]);
    expect(session.getItem(JELLY_BUBBLE_COUNT_KEY)).toBe("2");

    api.dismissBubble();
    vi.advanceTimersByTime(JELLY_IDLE_BUBBLE_MS * 3);
    expect(api.bubbleText.value).toBeNull();
    expect(JELLY_BUBBLE_LIMIT).toBe(2);
  });

  it("收起 / 展开各播一句中文状态，复用 2.5s 瞬时提示机制", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.collapse();
    expect(api.hintText.value).toBe(JELLY_COLLAPSE_HINT);

    api.expand();
    expect(api.hintText.value).toBe(JELLY_EXPAND_HINT);

    vi.advanceTimersByTime(JELLY_HINT_MS);
    expect(api.hintText.value).toBeNull();
  });

  it("提示文案收短、不自称「捏捏」：收起「我先躲起来」、展开「我回来了」", () => {
    expect(JELLY_COLLAPSE_HINT).toBe("我先躲起来");
    expect(JELLY_EXPAND_HINT).toBe("我回来了");

    for (const text of [JELLY_COLLAPSE_HINT, JELLY_EXPAND_HINT]) {
      expect(text).not.toContain("捏捏");
      expect(text).not.toContain("右边缘");
    }
  });

  it("气泡文案里也不出现「捏捏」这个名字（保留「捏」这个动作可以）", () => {
    for (const text of JELLY_BUBBLE_TEXTS) {
      expect(text).not.toContain("捏捏");
    }
    expect(JELLY_BUBBLE_TEXTS[0]).toContain("捏");
  });
});

describe("useJellyAssistant · 长按唤出的收起按钮（chip）", () => {
  it("pinCollapseChip() 挂起 chip，dismissCollapseChip() 收起", () => {
    const { api } = harness();

    expect(api.isChipPinned.value).toBe(false);

    api.pinCollapseChip();
    expect(api.isChipPinned.value).toBe(true);

    api.dismissCollapseChip();
    expect(api.isChipPinned.value).toBe(false);
  });

  it("长按唤出的 chip 在 4 秒后自动消失（JELLY_CHIP_PIN_MS）", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.pinCollapseChip();
    expect(api.isChipPinned.value).toBe(true);

    vi.advanceTimersByTime(JELLY_CHIP_PIN_MS);
    expect(api.isChipPinned.value).toBe(false);
  });

  it("pin 期间松手不触发 activate()（长按只是叫出收起按钮，不是「误开面板」）", () => {
    const { api } = harness();

    api.pointerDown();
    api.pinCollapseChip();
    api.pointerUp();

    expect(api.isOpen.value).toBe(false);
    expectState(api, "idle");
  });

  it("长按唤出的 chip 已经过了 4 秒自动消失后松手，也不打开面板（票 426）", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.pointerDown();
    api.pinCollapseChip();
    vi.advanceTimersByTime(JELLY_CHIP_PIN_MS);
    expect(api.isChipPinned.value).toBe(false);

    api.pointerUp();

    expect(api.isOpen.value).toBe(false);
    expectState(api, "idle");
  });

  it("抑制只属于那一次长按：下一次普通点击照常打开面板（票 426）", () => {
    vi.useFakeTimers();
    const { api } = harness();

    api.pointerDown();
    api.pinCollapseChip();
    vi.advanceTimersByTime(JELLY_CHIP_PIN_MS);
    api.pointerUp();
    expect(api.isOpen.value).toBe(false);

    api.pointerDown();
    api.pointerUp();
    expect(api.isOpen.value).toBe(true);
  });

  it("收起 / 展开都会顺手收掉 chip（不留下过期的挂起态）", () => {
    const { api } = harness();

    api.pinCollapseChip();
    api.collapse();
    expect(api.isChipPinned.value).toBe(false);

    api.pinCollapseChip();
    api.expand();
    expect(api.isChipPinned.value).toBe(false);
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

describe("useJellyAssistant · 六种视觉状态（固定定位后不再有 dragging）", () => {
  it("状态机只产出 idle / hover / press / open / collapsed / disabled 这六种", () => {
    const { api } = harness();
    const seen = new Set<JellyVisualState>();
    const record = () => seen.add(api.state.value);

    record(); // idle
    api.pointerEnter();
    record(); // hover
    api.pointerDown();
    record(); // press
    api.pointerUp();
    record(); // open
    api.activate();
    record(); // idle
    api.collapse();
    record(); // collapsed

    seen.add(harness({ aiEnabled: false }).api.state.value); // disabled

    expect([...seen].sort()).toEqual([
      "collapsed",
      "disabled",
      "hover",
      "idle",
      "open",
      "press",
    ]);
  });

  it("拖拽 / 位置持久化入口整体退场（API 只剩状态与动作）", () => {
    const { api } = harness();

    for (const gone of [
      "position",
      "collapsedPosition",
      "isDragged",
      "beginDrag",
      "endDrag",
      "moveTo",
      "dragTo",
      "persistPosition",
      "restorePosition",
      "handleViewportResize",
    ]) {
      expect(api, missingApiMessage(gone)).not.toHaveProperty(gone);
    }
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

describe("useJellyAssistant · 减少动态", () => {
  it("注入「减少动态」为 true 时：状态不变，但动效退化为静态", () => {
    const { api } = harness({ reducedMotion: true });

    expect(api.isReducedMotion.value).toBe(true);
    expect(api.shouldAnimate.value).toBe(false);
    expect(api.breathDurationMs.value).toBe(0);
    expect(api.state.value).toBe("idle");
  });

  it("默认走票 3 的 useMotionPreference（读系统设置）", () => {
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

describe("useJellyAssistant · 页面级共享实例（票 10 的接缝）", () => {
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

/** 给 `not.toHaveProperty` 的失败信息带上被检查的名字（否则报错只说「false」）。 */
function missingApiMessage(property: string): string {
  return `API 不应再暴露 ${property}`;
}
