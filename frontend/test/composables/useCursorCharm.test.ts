import { nextTick, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useCursorCharm } from "~/composables/useCursorCharm";
import {
  CHARM_FINE_POINTER_QUERY,
  CHARM_ROPE_LENGTH_DEFAULT,
  CHARM_SEGMENTS_DEFAULT,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：这段决定挂件"到底有没有开始工作、什么时候停"。
 * 物理模块的不变量已经单独测过；这里测的是接线 —— 首移才开跑、收敛就停帧、
 * 失焦冻结（绝不瞬移）、切后台暂停、卸载摘干净、重复初始化不叠第二份。
 * 这些全是运行时成本：多留一个逐帧循环，读者的风扇就会替他发现。
 */

const REDUCED = "(prefers-reduced-motion: reduce)";

type Listener = (event?: unknown) => void;

function fakeMedia(map: Record<string, boolean>) {
  const listeners = new Set<(event: { matches: boolean }) => void>();
  return {
    matchMedia: (query: string) => ({
      matches: map[query] ?? false,
      addEventListener: vi.fn((_type: "change", listener: (event: { matches: boolean }) => void) => {
        listeners.add(listener);
      }),
      removeEventListener: vi.fn(
        (_type: "change", listener: (event: { matches: boolean }) => void) => {
          listeners.delete(listener);
        },
      ),
    }),
    listeners,
  };
}

/** 一个可以 emit 任意事件类型的宿主（window 或 document 的最小替身）。 */
function fakeHost(initialHidden = false) {
  const listeners = new Map<string, Set<Listener>>();
  const host = {
    hidden: initialHidden,
    addEventListener: vi.fn((type: string, listener: Listener) => {
      const set = listeners.get(type) ?? new Set<Listener>();
      set.add(listener);
      listeners.set(type, set);
    }),
    removeEventListener: vi.fn((type: string, listener: Listener) => {
      listeners.get(type)?.delete(listener);
    }),
  };
  return {
    host,
    listeners,
    emit(type: string, event?: unknown) {
      for (const listener of listeners.get(type) ?? []) listener(event);
    },
    count(type: string): number {
      return listeners.get(type)?.size ?? 0;
    },
    total(): number {
      let sum = 0;
      for (const set of listeners.values()) sum += set.size;
      return sum;
    },
  };
}

/** 可手动推进的动画帧队列 + 时钟（不依赖真时间，也不用 sleep）。 */
function fakeFrames() {
  const queue = new Map<number, Listener>();
  let nextId = 1;
  const clock = { now: 0 };
  return {
    clock,
    now: () => clock.now,
    requestAnimationFrame: vi.fn((callback: Listener) => {
      const id = nextId;
      nextId += 1;
      queue.set(id, callback);
      return id;
    }),
    cancelAnimationFrame: vi.fn((id: number) => {
      queue.delete(id);
    }),
    pending: () => queue.size,
    /** 推进真实时间并触发当前排队的全部帧（一次"刷新"）。 */
    advance(ms: number) {
      clock.now += ms;
      const batch = [...queue.entries()];
      queue.clear();
      for (const [, callback] of batch) callback(clock.now);
    },
    /** 一直推进到环不再请求新帧（收敛 / 暂停 / 卸载）。 */
    runUntilIdle(maxFrames = 2000): number {
      let frames = 0;
      while (queue.size > 0 && frames < maxFrames) {
        this.advance(16.667);
        frames += 1;
      }
      return frames;
    },
  };
}

interface MountOverrides {
  configEnabled?: () => boolean;
  config?: () => { ropeLengthPx?: number; segments?: number; gravity?: number; damping?: number };
  media?: Record<string, boolean>;
}

function mount(overrides: MountOverrides = {}) {
  const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true, ...overrides.media });
  const pointer = fakeHost();
  const doc = fakeHost();
  const frames = fakeFrames();
  const charm = useCursorCharm({
    matchMedia: media.matchMedia,
    target: pointer.host,
    visibilityTarget: doc.host,
    configEnabled: overrides.configEnabled ?? (() => true),
    config: overrides.config,
    requestAnimationFrame: frames.requestAnimationFrame,
    cancelAnimationFrame: frames.cancelAnimationFrame,
    now: frames.now,
  });
  return { charm, media, pointer, doc, frames };
}

const desktop = () => fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true });

describe("useCursorCharm：启用条件", () => {
  it("精确指针 + 未减少动态 + 总开关开着时启用并绑定指针与失焦监听", () => {
    const { charm, pointer, doc } = mount();

    expect(charm.enabled.value).toBe(true);
    expect(pointer.count("pointermove")).toBe(1);
    expect(pointer.count("pointerover")).toBe(1);
    expect(pointer.count("pointerout")).toBe(1);
    expect(pointer.count("blur")).toBe(1);
    expect(pointer.count("focus")).toBe(1);
    expect(doc.count("visibilitychange")).toBe(1);
  });

  it("触屏（无 hover/精确指针）不启用，也不绑定任何指针监听", () => {
    const { charm, pointer, doc } = mount({ media: { [CHARM_FINE_POINTER_QUERY]: false } });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.total()).toBe(0);
    expect(doc.total()).toBe(0);
  });

  it("系统要求减少动态时不启用（整站不出现，不是放慢）", () => {
    const { charm, pointer, doc } = mount({ media: { [REDUCED]: true } });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.total()).toBe(0);
    expect(doc.total()).toBe(0);
  });

  it("后台总开关关闭时不启用，一个监听都不留（运行时成本归零）", () => {
    const { charm, pointer, doc, media } = mount({ configEnabled: () => false });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.total()).toBe(0);
    expect(doc.total()).toBe(0);
    expect(media.listeners.size).toBe(0);
  });

  it("没有 matchMedia（SSR）时关闭且不报错", () => {
    const pointer = fakeHost();
    const frames = fakeFrames();
    const charm = useCursorCharm({
      matchMedia: () => null,
      target: pointer.host,
      configEnabled: () => true,
      requestAnimationFrame: frames.requestAnimationFrame,
      cancelAnimationFrame: frames.cancelAnimationFrame,
      now: frames.now,
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.total()).toBe(0);
    expect(() => charm.dispose()).not.toThrow();
  });

  it("开关后来被打开时补上监听，再关掉时摘掉（后台改完刷新即生效）", async () => {
    const { charm, pointer, media, frames } = mount({ configEnabled: () => switchOn.value });
    switchOn.value = false;
    await nextTick();
    expect(pointer.total()).toBe(0);
    expect(media.listeners.size).toBe(0);

    switchOn.value = true;
    await nextTick();
    expect(charm.enabled.value).toBe(true);
    expect(pointer.total()).toBe(5);
    expect(media.listeners.size).toBe(2);

    switchOn.value = false;
    await nextTick();
    expect(pointer.total()).toBe(0);
    expect(media.listeners.size).toBe(0);
    expect(frames.pending()).toBe(0);
  });

  it("重复初始化防护：两次 pointermove 只请求一帧（HMR / 路由重挂不叠第二份）", () => {
    const { pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 10, clientY: 20 });
    pointer.emit("pointermove", { clientX: 30, clientY: 40 });

    expect(frames.requestAnimationFrame).toHaveBeenCalledTimes(1);
    expect(frames.pending()).toBe(1);
    expect(pointer.count("pointermove")).toBe(1);
  });
});

const switchOn = ref(true);

describe("useCursorCharm：首次移动、绳链与淡入", () => {
  it("首次移动前不可见、也没有动画帧（页面刚打开不该先冒出一个东西）", () => {
    const { charm, frames } = mount();

    expect(charm.visible.value).toBe(false);
    expect(charm.tetherPath.value).toBe("");
    expect(frames.pending()).toBe(0);
  });

  it("首次移动后淡入，并立刻画出竖直静止链（首帧不甩）", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 120, clientY: 240 });

    expect(charm.visible.value).toBe(true);
    expect(charm.pointer.value).toEqual({ x: 120, y: 240 });
    expect(charm.logoCenter.value).toEqual({ x: 120, y: 240 + CHARM_ROPE_LENGTH_DEFAULT });
    const points = charm.tetherPath.value.split("L");
    expect(points).toHaveLength(CHARM_SEGMENTS_DEFAULT + 1);
    expect(points[0]).toBe("M120 240");
    // 第一次移动只是"手放上去"，物理从下一帧开始。
    expect(frames.pending()).toBe(1);
  });

  it("后续移动只更新指针，不会来回切换可见性", () => {
    const { charm, pointer } = mount();
    pointer.emit("pointermove", { clientX: 10, clientY: 20 });
    pointer.emit("pointermove", { clientX: 30, clientY: 40 });

    expect(charm.pointer.value).toEqual({ x: 30, y: 40 });
    expect(charm.visible.value).toBe(true);
  });

  it("没有注入 window（SSR）时移动也不会炸", () => {
    const frames = fakeFrames();
    const charm = useCursorCharm({
      matchMedia: desktop().matchMedia,
      target: null,
      configEnabled: () => true,
      requestAnimationFrame: frames.requestAnimationFrame,
      cancelAnimationFrame: frames.cancelAnimationFrame,
      now: frames.now,
    });

    expect(() => charm.dispose()).not.toThrow();
    expect(charm.visible.value).toBe(false);
  });

  it("节数配置变化后链重建，节点数跟着变且不报错", () => {
    const segments = ref(4);
    const { charm, pointer } = mount({
      config: () => ({ segments: segments.value, ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    expect(charm.tetherPath.value.split("L")).toHaveLength(5);

    segments.value = 12;
    pointer.emit("pointermove", { clientX: 10, clientY: 0 });
    expect(charm.tetherPath.value.split("L")).toHaveLength(13);
  });
});

describe("useCursorCharm：固定步长循环与停帧", () => {
  it("指针移动后按真实时间推进，头节点始终钉在指针上", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(16.667);
    pointer.emit("pointermove", { clientX: 60, clientY: 0 });

    for (let i = 0; i < 6; i += 1) frames.advance(16.667);

    expect(charm.tetherPath.value.startsWith("M60 0")).toBe(true);
    expect(Number(charm.logoCenter.value.x)).not.toBeNaN();
    // 头节点钉住、链被拖在后面：站标不会还停在旧位置。
    expect(charm.logoCenter.value.x).toBeGreaterThan(20);
  });

  it("能量收敛后取消动画帧（静态页面必须零帧开销）", () => {
    const { pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    const framesRun = frames.runUntilIdle(1200);

    expect(framesRun).toBeGreaterThan(0);
    expect(frames.pending()).toBe(0);
    expect(frames.cancelAnimationFrame).toHaveBeenCalled();
  });

  it("收敛后指针再动会重新唤醒循环", () => {
    const { pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    frames.runUntilIdle(1200);
    expect(frames.pending()).toBe(0);

    const requestsBefore = frames.requestAnimationFrame.mock.calls.length;
    pointer.emit("pointermove", { clientX: 200, clientY: 50 });
    expect(frames.requestAnimationFrame.mock.calls.length).toBe(requestsBefore + 1);
    expect(frames.pending()).toBe(1);
  });
});

describe("useCursorCharm：失焦冻结与切后台暂停", () => {
  it("窗口失焦时冻结指针端而不是瞬移：头节点留在原处，链自然收敛", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    pointer.emit("pointermove", { clientX: 160, clientY: 100 });
    frames.advance(20);
    const headBefore = charm.tetherPath.value.split("L")[0];

    pointer.emit("blur");
    // 冻结期间即使收到（浏览器不该发的）pointermove，也不能动头节点。
    pointer.emit("pointermove", { clientX: 500, clientY: 500 });
    const framesRun = frames.runUntilIdle(1200);

    expect(framesRun).toBeGreaterThan(0);
    expect(charm.tetherPath.value.split("L")[0]).toBe(headBefore);
    expect(charm.pointer.value).toEqual({ x: 160, y: 100 });
    expect(frames.pending()).toBe(0);
  });

  it("指针移出文档（pointerout 且 relatedTarget 为空）也按冻结处理；重新进入后解冻", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    frames.advance(20);
    const headBefore = charm.tetherPath.value.split("L")[0];

    pointer.emit("pointerout", { relatedTarget: null });
    pointer.emit("pointermove", { clientX: 500, clientY: 500 });
    frames.runUntilIdle(1200);

    expect(charm.tetherPath.value.split("L")[0]).toBe(headBefore);
    expect(frames.pending()).toBe(0);

    // 指针重新进入（pointerover）后才接受新位置：从头节点原处继续，而不是冻结时瞬移。
    pointer.emit("pointerover");
    pointer.emit("pointermove", { clientX: 500, clientY: 500 });
    expect(charm.pointer.value).toEqual({ x: 500, y: 500 });
  });

  it("文档隐藏时暂停循环，恢复可见后从原状态继续（不推演一整段后台时间）", () => {
    const { charm, pointer, doc, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    pointer.emit("pointermove", { clientX: 100, clientY: 0 });
    frames.advance(20);
    const pathBeforeHidden = charm.tetherPath.value;

    doc.host.hidden = true;
    doc.emit("visibilitychange");
    expect(frames.pending()).toBe(0);
    expect(frames.cancelAnimationFrame).toHaveBeenCalled();

    // 后台期间即使时间流逝，也不该有逐帧运算。
    frames.advance(5000);
    frames.advance(5000);
    expect(charm.tetherPath.value).toBe(pathBeforeHidden);

    const requestsBefore = frames.requestAnimationFrame.mock.calls.length;
    doc.host.hidden = false;
    doc.emit("visibilitychange");
    expect(frames.requestAnimationFrame.mock.calls.length).toBe(requestsBefore + 1);
    expect(frames.pending()).toBe(1);

    // 恢复后继续推进，但不会先把 10 秒后台时间补上（路径不应瞬移）。
    frames.advance(16.667);
    expect(charm.tetherPath.value.startsWith("M100 0")).toBe(true);
  });
});

describe("useCursorCharm：清理", () => {
  it("dispose 取消动画帧并摘掉指针 / 失焦 / 可见性 / 媒体监听", () => {
    const { charm, media, pointer, doc, frames } = mount();
    pointer.emit("pointermove", { clientX: 10, clientY: 20 });
    frames.advance(20);

    charm.dispose();

    expect(frames.pending()).toBe(0);
    expect(frames.cancelAnimationFrame).toHaveBeenCalled();
    expect(pointer.total()).toBe(0);
    expect(doc.total()).toBe(0);
    expect(media.listeners.size).toBe(0);
  });

  it("重复 dispose 不抛错，也不做第二次清理（HMR / 路由重挂）", () => {
    const { charm } = mount();

    charm.dispose();
    expect(() => charm.dispose()).not.toThrow();
  });

  it("没有注入动画帧（SSR）时移动不请求帧也不报错", () => {
    const media = desktop();
    const pointer = fakeHost();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.host,
      configEnabled: () => true,
      requestAnimationFrame: null,
      cancelAnimationFrame: null,
    });

    expect(() => pointer.emit("pointermove", { clientX: 10, clientY: 20 })).not.toThrow();
    expect(charm.tetherPath.value).not.toBe("");
  });
});
