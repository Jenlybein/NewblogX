import { isRef, nextTick, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useCursorCharm } from "~/composables/useCursorCharm";
import {
  CHARM_FINE_POINTER_QUERY,
  CHARM_IDLE_FRAME_INTERVAL_MS,
  CHARM_IDLE_SWAY_AMP_DEFAULT,
  CHARM_IDLE_SWAY_AMP_MAX,
  CHARM_ROPE_LENGTH_DEFAULT,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：这段决定挂件"到底有没有开始工作、什么时候停、以多快的节奏跑"。
 * 物理模块的不变量已经单独测过；这里测的是接线 —— 首移才开跑、收敛就停帧、
 * 失焦冻结（绝不瞬移）、切后台暂停、卸载摘干净、重复初始化不叠第二份，
 * 以及票 #42 的两条车道：轻晃开启、指针停住时走 ~20fps 的怠速车道；
 * 指针还在主动驱动时走满速 rAF；关闭 / 减少动态时能退回零帧。
 */

const REDUCED = "(prefers-reduced-motion: reduce)";

/** 假动画帧回调是"下一次刷新"：固定 16.667ms 后到期。 */
const RAF_DELAY_MS = 16.667;

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

/**
 * 可手动推进的"动画帧 + 定时器"调度器与时钟。
 *
 * 满速车道用 requestAnimationFrame（下一次刷新 ≈ +16.7ms）；怠速车道用
 * setTimeout(50ms)。把两条队列放在同一个虚拟时钟里推进，才能断言
 * "每秒实际跑了几帧"，而不是只断言"调用过哪种 API"。
 */
function fakeFrames() {
  interface Scheduled {
    id: number;
    due: number;
    callback: Listener;
    kind: "raf" | "timer";
  }
  const queue = new Map<number, Scheduled>();
  let nextId = 1;
  const clock = { now: 0 };
  const delays: number[] = [];
  let executed = 0;

  function schedule(kind: Scheduled["kind"], callback: Listener, delayMs: number): number {
    const id = nextId;
    nextId += 1;
    queue.set(id, { id, due: clock.now + delayMs, callback, kind });
    return id;
  }

  return {
    clock,
    delays,
    now: () => clock.now,
    executed: () => executed,
    requestAnimationFrame: vi.fn((callback: Listener) => schedule("raf", callback, RAF_DELAY_MS)),
    cancelAnimationFrame: vi.fn((id: number) => {
      queue.delete(id);
    }),
    setTimeout: vi.fn((callback: Listener, delayMs: number) => {
      delays.push(delayMs);
      return schedule("timer", callback, delayMs);
    }),
    clearTimeout: vi.fn((id: number) => {
      queue.delete(id);
    }),
    pending: () => queue.size,
    /** 推进真实时间，按到期顺序执行这段时间内排到的每个回调。 */
    advance(ms: number) {
      const end = clock.now + ms;
      for (;;) {
        let next: Scheduled | null = null;
        for (const entry of queue.values()) {
          if (entry.due <= end && (!next || entry.due < next.due)) next = entry;
        }
        if (!next) break;
        queue.delete(next.id);
        clock.now = Math.max(clock.now, next.due);
        executed += 1;
        next.callback(clock.now);
      }
      clock.now = end;
    },
    /** 一直推进到环不再请求新帧（收敛 / 暂停 / 卸载）。 */
    runUntilIdle(maxFrames = 4000): number {
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
  config?: () => {
    ropeLengthPx?: number;
    segments?: number;
    gravity?: number;
    damping?: number;
    lineWidthPx?: number;
    idleSwayHz?: number;
    idleSwayAmp?: number;
  } | null;
  media?: Record<string, boolean>;
}

function mount(overrides: MountOverrides = {}) {
  const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true, ...overrides.media });
  const pointer = fakeHost();
  const doc = fakeHost();
  const frames = fakeFrames();
  // 默认把轻晃关掉：既有用例测的是"静止即停帧"的满速车道。
  // 怠速车道另有专门的用例（传 idleSwayAmp > 0）。
  const config = overrides.config ?? (() => ({ idleSwayAmp: 0 }));
  const charm = useCursorCharm({
    matchMedia: media.matchMedia,
    target: pointer.host,
    visibilityTarget: doc.host,
    configEnabled: overrides.configEnabled ?? (() => true),
    config,
    requestAnimationFrame: frames.requestAnimationFrame,
    cancelAnimationFrame: frames.cancelAnimationFrame,
    setTimeout: frames.setTimeout,
    clearTimeout: frames.clearTimeout,
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

  it("系统要求减少动态时不启用（整站不出现，不是放慢），轻晃配置也不留帧", () => {
    const { charm, pointer, doc, frames } = mount({
      media: { [REDUCED]: true },
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT }),
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.total()).toBe(0);
    expect(doc.total()).toBe(0);
    frames.advance(1000);
    expect(frames.pending()).toBe(0);
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
      setTimeout: frames.setTimeout,
      clearTimeout: frames.clearTimeout,
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
    expect(charm.tetherGeometry?.centerline[0]).toEqual({ x: 120, y: 240 });
    // 首次移动的链是竖直的、松弛为 0：几何上就是一条直线。
    expect(charm.tetherGeometry?.maxDeviationPx).toBeLessThan(0.01);
    // 第一次移动只是"手放上去"，物理从下一帧开始（此处是满速车道）。
    expect(frames.pending()).toBe(1);
    expect(frames.delays).toEqual([]);
  });

  it("每帧只更新模板绑定的两个值；几何与帧数是普通快照（不逐帧触发响应式失效）", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 10, clientY: 20 });
    frames.advance(20);

    // 模板绑定：路径 `d` 与站标中心（组件把它换算成一个 transform）必须是 ref。
    expect(isRef(charm.tetherPath)).toBe(true);
    expect(isRef(charm.logoCenter)).toBe(true);
    // 测试/调试用的几何与帧数：普通属性，每帧推进不会让任何订阅者失效。
    expect(isRef(charm.tetherGeometry)).toBe(false);
    expect(isRef(charm.physicsFrames)).toBe(false);
    expect(charm.tetherGeometry).not.toBeNull();
    expect(charm.physicsFrames).toBeGreaterThan(0);
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
      setTimeout: frames.setTimeout,
      clearTimeout: frames.clearTimeout,
      now: frames.now,
    });

    expect(() => charm.dispose()).not.toThrow();
    expect(charm.visible.value).toBe(false);
  });

  it("节数配置变化后链重建，几何跟着换一套且不报错", () => {
    const segments = ref(4);
    const { charm, pointer } = mount({
      config: () => ({ segments: segments.value, ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    const centerlineBefore = charm.tetherGeometry?.centerline.length ?? 0;
    expect(centerlineBefore).toBeGreaterThan(5);

    segments.value = 12;
    pointer.emit("pointermove", { clientX: 10, clientY: 0 });
    expect(charm.tetherGeometry?.centerline.length).toBeGreaterThan(centerlineBefore);
  });
});

describe("useCursorCharm：固定步长循环与停帧（满速车道）", () => {
  it("指针移动后按真实时间推进，头节点始终钉在指针上", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(16.667);
    pointer.emit("pointermove", { clientX: 60, clientY: 0 });

    for (let i = 0; i < 6; i += 1) frames.advance(16.667);

    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(60, 1);
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
    expect(frames.clearTimeout).not.toHaveBeenCalled();
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

describe("useCursorCharm：怠速车道（票 #42 裁决）", () => {
  it("指针停住后进入 ~20fps 怠速车道（每帧 50ms，明显低于满速 60fps）", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT, idleSwayHz: 0.8 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });

    // 指针刚移动过：先按满速 rAF 走；停住超过活动窗口后才切到怠速车道。
    expect(frames.requestAnimationFrame).toHaveBeenCalled();

    frames.advance(1000);
    // 1 秒内 6 帧满速 + 18 帧怠速，共约 24 帧：满速是 60 帧，怠速必须明显更低。
    expect(charm.physicsFrames).toBeGreaterThanOrEqual(18);
    expect(charm.physicsFrames).toBeLessThanOrEqual(26);
    expect(charm.physicsFrames).toBeLessThan(60);
    // 怠速车道的每一次调度用的都是同一个节流间隔，不会偷偷补帧追赶。
    expect(frames.delays.length).toBeGreaterThan(0);
    expect(frames.delays.every((delay) => delay === CHARM_IDLE_FRAME_INTERVAL_MS)).toBe(true);
  });

  it("指针持续移动时保持满速 rAF（不被怠速车道节流）", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT, idleSwayHz: 0.8 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });

    // 每帧都移动指针：头部一直在被主动驱动，整秒都该是满速帧。
    for (let index = 0; index < 60; index += 1) {
      pointer.emit("pointermove", { clientX: index * 3, clientY: 0 });
      frames.advance(16.667);
    }

    expect(charm.physicsFrames).toBeGreaterThanOrEqual(50);
    expect(charm.physicsFrames).toBeLessThanOrEqual(60);
    // 一次都没排过 50ms 的定时器：拖行时不应降频。
    expect(frames.delays).toEqual([]);
  });

  it("idle_sway_hz = 0 视为关掉轻晃：幅度 > 0 也必须静止并退回零帧", () => {
    const { pointer, frames } = mount({
      // 默认幅度 20 + 频率 0：正面撞上"文档说能关但是一直空转"的旧行为。
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT, idleSwayHz: 0 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    const framesRun = frames.runUntilIdle(1200);

    expect(framesRun).toBeGreaterThan(0);
    expect(frames.pending()).toBe(0);
    expect(frames.delays).toEqual([]);
  });

  it("轻晃期间自由端真的在动（不是整层平移，头节点钉死）", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_MAX, idleSwayHz: 0.8 }),
    });
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });

    // 沿怠速车道跑 2 秒，记录自由端的最大横向位移（0.8Hz 的周期是 1.25s，
    // 单看某一时刻可能刚好过零点，所以量峰值而不是某一帧）。
    let peakTailOffset = 0;
    let peakArc = 0;
    for (let frame = 0; frame < 40; frame += 1) {
      frames.advance(CHARM_IDLE_FRAME_INTERVAL_MS);
      peakTailOffset = Math.max(peakTailOffset, Math.abs(charm.logoCenter.value.x - 100));
      peakArc = Math.max(peakArc, charm.tetherGeometry?.maxDeviationPx ?? 0);
    }

    expect(peakTailOffset).toBeGreaterThan(1);
    expect(peakArc).toBeGreaterThan(0.5);
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(100, 1);
  });

  it("把 idle_sway_amp 调回 0 后有限时间内退回零帧（两条车道都能停）", () => {
    const amp = ref(CHARM_IDLE_SWAY_AMP_DEFAULT);
    const { pointer, frames } = mount({
      config: () => ({ idleSwayAmp: amp.value, idleSwayHz: 0.8 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(1000);
    expect(frames.pending()).toBe(1);

    amp.value = 0;
    frames.runUntilIdle(2000);

    expect(frames.pending()).toBe(0);
    // 停帧必须取消"当前车道"的待执行帧：怠速车道的定时器被 clearTimeout 了；
    // 若当帧还没收敛、已切回满速车道，则走 cancelAnimationFrame（见满速用例）。
    expect(frames.clearTimeout).toHaveBeenCalled();
  });

  it("收敛后把 idle_sway_amp 打开会重新唤醒怠速车道", () => {
    const amp = ref(0);
    const { pointer, frames } = mount({
      config: () => ({ idleSwayAmp: amp.value, idleSwayHz: 0.8 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    frames.runUntilIdle(1200);
    expect(frames.pending()).toBe(0);

    amp.value = CHARM_IDLE_SWAY_AMP_DEFAULT;
    pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    // 刚移动过 → 先满速；停住超过活动窗口后回到怠速车道并持续排帧。
    expect(frames.pending()).toBe(1);
    frames.advance(300);
    expect(frames.delays.at(-1)).toBe(CHARM_IDLE_FRAME_INTERVAL_MS);
  });
});

describe("useCursorCharm：失焦冻结与切后台暂停", () => {
  it("窗口失焦时冻结指针端而不是瞬移：头节点留在原处，链自然收敛", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    pointer.emit("pointermove", { clientX: 160, clientY: 100 });
    frames.advance(20);
    const headBefore = charm.tetherGeometry?.centerline[0];

    pointer.emit("blur");
    // 冻结期间即使收到（浏览器不该发的）pointermove，也不能动头节点。
    pointer.emit("pointermove", { clientX: 500, clientY: 500 });
    const framesRun = frames.runUntilIdle(1200);

    expect(framesRun).toBeGreaterThan(0);
    expect(charm.tetherGeometry?.centerline[0]).toEqual(headBefore);
    expect(charm.pointer.value).toEqual({ x: 160, y: 100 });
    expect(frames.pending()).toBe(0);
  });

  it("指针移出文档（pointerout 且 relatedTarget 为空）也按冻结处理；重新进入后解冻", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    frames.advance(20);
    const headBefore = charm.tetherGeometry?.centerline[0];

    pointer.emit("pointerout", { relatedTarget: null });
    pointer.emit("pointermove", { clientX: 500, clientY: 500 });
    frames.runUntilIdle(1200);

    expect(charm.tetherGeometry?.centerline[0]).toEqual(headBefore);
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
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(100, 1);
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

  it("dispose 也能清掉怠速车道的定时器", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT }),
    });
    pointer.emit("pointermove", { clientX: 10, clientY: 20 });
    // 指针停住超过活动窗口后进入怠速车道，此时待执行的是 50ms 定时器。
    frames.advance(300);
    expect(frames.delays.length).toBeGreaterThan(0);
    expect(frames.pending()).toBe(1);

    charm.dispose();

    expect(frames.pending()).toBe(0);
    expect(frames.clearTimeout).toHaveBeenCalled();
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
      setTimeout: null,
      clearTimeout: null,
    });

    expect(() => pointer.emit("pointermove", { clientX: 10, clientY: 20 })).not.toThrow();
    expect(charm.tetherPath.value).not.toBe("");
  });
});
