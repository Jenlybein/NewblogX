import { isRef, nextTick, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useCursorCharm } from "~/composables/useCursorCharm";
import {
  CHARM_FINE_POINTER_QUERY,
  CHARM_FPS_CAP_DEFAULT,
  CHARM_IDLE_SWAY_AMP_DEFAULT,
  CHARM_IDLE_SWAY_AMP_MAX,
  CHARM_POINTER_OFFSET_X_DEFAULT,
  CHARM_POINTER_OFFSET_Y_DEFAULT,
  CHARM_ROPE_LENGTH_DEFAULT,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：这段决定挂件"到底有没有开始工作、什么时候停、以多快的节奏跑"。
 * 物理模块的不变量已经单独测过；这里测的是接线 —— 首移才开跑、收敛就停帧、
 * 失焦冻结（绝不瞬移）、切后台暂停、卸载摘干净、重复初始化不叠第二份，
 * 以及票 #74 的恒定帧率上限：**只有一条 requestAnimationFrame 调度**，
 * 每帧先比较 `now - lastPublishAt`，到点才推进物理并发布；`pointermove`
 * 只写坐标、不再自行发布。
 *
 * 时钟是虚拟的：`fakeFrames` 是一条可手动推进的动画帧队列，`advance(16.667)`
 * 就是"60Hz 显示器又刷新了一帧"。于是"每秒发布了几次"可以直接数出来，
 * 而不必依赖真浏览器的合成时机。
 */

const REDUCED = "(prefers-reduced-motion: reduce)";

/** 假动画帧回调是"下一次刷新"：固定 16.667ms 后到期（= 60Hz）。 */
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
 * 可手动推进的动画帧队列与虚拟时钟。
 *
 * 只有一条队列：挂件只剩 requestAnimationFrame 一种调度（票 #74）。
 * 这里**没有** setTimeout 槽位 —— 定时器驱动的帧已经不存在了，
 * 留一个空槽只会让人以为它还在。
 */
function fakeFrames() {
  interface Scheduled {
    id: number;
    due: number;
    callback: Listener;
  }
  const queue = new Map<number, Scheduled>();
  let nextId = 1;
  const clock = { now: 0 };
  let executed = 0;

  return {
    clock,
    now: () => clock.now,
    /** 已经执行过的帧回调数（= 虚拟时钟走了多少帧）。 */
    executed: () => executed,
    requestAnimationFrame: vi.fn((callback: Listener) => {
      const id = nextId;
      nextId += 1;
      queue.set(id, { id, due: clock.now + RAF_DELAY_MS, callback });
      return id;
    }),
    cancelAnimationFrame: vi.fn((id: number) => {
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
        this.advance(RAF_DELAY_MS);
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
    fpsCap?: number;
    pointerOffsetX?: number;
    pointerOffsetY?: number;
  } | null;
  media?: Record<string, boolean>;
}

function mount(overrides: MountOverrides = {}) {
  const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true, ...overrides.media });
  const pointer = fakeHost();
  const doc = fakeHost();
  const frames = fakeFrames();
  // 默认把轻晃关掉：多数用例测的是"静止即停帧"。
  // 需要持续跑帧的用例（帧率上限、轻晃）自己传 idleSwayAmp > 0。
  const config = overrides.config ?? (() => ({ idleSwayAmp: 0 }));
  const charm = useCursorCharm({
    matchMedia: media.matchMedia,
    target: pointer.host,
    visibilityTarget: doc.host,
    configEnabled: overrides.configEnabled ?? (() => true),
    config,
    requestAnimationFrame: frames.requestAnimationFrame,
    cancelAnimationFrame: frames.cancelAnimationFrame,
    now: frames.now,
  });
  return { charm, media, pointer, doc, frames };
}

const desktop = () => fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true });

/**
 * 默认指针端偏移（票 #75）：物理头节点 = 指针热点 + 这个偏移。
 *
 * 组合式用例里凡是要断言"头节点在哪儿"的地方都带上它 —— 默认配置下绳头并不
 * 压在光标正中央，而是从右下 3px 悬挂；如果某处漏了偏移，那条断言会先红。
 */
const DEFAULT_OFFSET = {
  x: CHARM_POINTER_OFFSET_X_DEFAULT,
  y: CHARM_POINTER_OFFSET_Y_DEFAULT,
};

/** 轻晃开着的配置：循环会一直跑，才能数"每秒发布了几次"。 */
const swaying = (fpsCap: number) => () => ({
  idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT,
  idleSwayHz: 0.8,
  fpsCap,
});

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

  it("首次移动只淡入 + 写坐标，链在门控帧里才画出来（首帧不甩）", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 120, clientY: 240 });

    expect(charm.visible.value).toBe(true);
    expect(charm.pointer.value).toEqual({ x: 120, y: 240 });
    // 唯一发布口是门控帧：事件里不再自己 publish（票 #74 约束 5）。
    expect(charm.tetherPath.value).toBe("");
    expect(charm.tetherGeometry).toBeNull();
    expect(frames.pending()).toBe(1);

    frames.advance(RAF_DELAY_MS);

    expect(charm.tetherPath.value).not.toBe("");
    // 头节点 = 指针热点 + 默认偏移（3, 3）：默认配置下绳头不压在光标正中央。
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(120 + DEFAULT_OFFSET.x, 1);
    expect(charm.tetherGeometry?.centerline[0]?.y).toBeCloseTo(240 + DEFAULT_OFFSET.y, 1);
    // 首次移动的链是竖直的、松弛为 0：几何上就是一条直线。
    expect(charm.tetherGeometry?.maxDeviationPx).toBeLessThan(0.01);
    expect(charm.logoCenter.value.x).toBeCloseTo(120 + DEFAULT_OFFSET.x, 1);
    expect(charm.logoCenter.value.y).toBeCloseTo(
      240 + DEFAULT_OFFSET.y + CHARM_ROPE_LENGTH_DEFAULT,
      1,
    );
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
      now: frames.now,
    });

    expect(() => charm.dispose()).not.toThrow();
    expect(charm.visible.value).toBe(false);
  });

  it("节数配置变化后链重建，几何跟着换一套且不报错", () => {
    const segments = ref(4);
    const { charm, pointer, frames } = mount({
      config: () => ({ segments: segments.value, ropeLengthPx: CHARM_ROPE_LENGTH_DEFAULT }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);
    const centerlineBefore = charm.tetherGeometry?.centerline.length ?? 0;
    expect(centerlineBefore).toBeGreaterThan(5);

    segments.value = 12;
    pointer.emit("pointermove", { clientX: 10, clientY: 0 });
    frames.advance(RAF_DELAY_MS);
    expect(charm.tetherGeometry?.centerline.length).toBeGreaterThan(centerlineBefore);
  });
});

describe("useCursorCharm：固定步长循环与停帧", () => {
  it("指针移动后按真实时间推进，头节点始终钉在「指针 + 偏移」上", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(16.667);
    pointer.emit("pointermove", { clientX: 60, clientY: 0 });

    for (let i = 0; i < 6; i += 1) frames.advance(16.667);

    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(60 + DEFAULT_OFFSET.x, 1);
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

  it("轻晃开着时不停帧：两秒后仍在持续发布（不能冻在一个歪姿势上）", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_MAX, idleSwayHz: 0.8, fpsCap: 60 }),
    });
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });

    frames.advance(2000);

    expect(frames.pending()).toBe(1);
    expect(charm.physicsFrames).toBeGreaterThan(100);
    // 轻晃是自由端的真实形变：头节点钉死在「指针 + 偏移」上，尾端来回偏。
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(100 + DEFAULT_OFFSET.x, 1);
  });
});

/** 在 60Hz 虚拟时钟上跑 1 秒，返回"多少帧刷新"与"发布了几次"。 */
function simulateOneSecond(fpsCap: number) {
  const { charm, pointer, frames } = mount({ config: swaying(fpsCap) });
  pointer.emit("pointermove", { clientX: 0, clientY: 0 });
  frames.advance(1000);

  return { published: charm.physicsFrames, refreshed: frames.executed(), frames };
}

describe("useCursorCharm：恒定帧率上限（票 #74 裁决）", () => {
  it("上限 60 在 60Hz 屏上：每一帧都发布", () => {
    const { published, refreshed } = simulateOneSecond(CHARM_FPS_CAP_DEFAULT);

    expect(refreshed).toBeGreaterThanOrEqual(58);
    expect(published, "上限 60 时不该有帧被门控挡掉").toBe(refreshed);
  });

  it("上限 30 在 60Hz 屏上：每 2 帧发布一次（约 30 次/秒）", () => {
    const { published, refreshed } = simulateOneSecond(30);

    expect(refreshed).toBeGreaterThanOrEqual(58);
    // 33.3ms 一帧 = 每 2 个 16.667ms 刷新发布一次；差值只允许 ±1（起止边界）。
    expect(published).toBeGreaterThanOrEqual(Math.floor(refreshed / 2) - 1);
    expect(published).toBeLessThanOrEqual(Math.ceil(refreshed / 2) + 1);
  });

  it("上限高于刷新率（160）时等价于每帧：门控不会把 60Hz 屏打成 30Hz", () => {
    const { published, refreshed } = simulateOneSecond(160);

    expect(published).toBe(refreshed);
  });

  it("门控不拖慢物理：上限 30 时收敛所需的发布次数约为上限 60 的一半", () => {
    // 每次发布补进"距上一次发布真实流逝的时间"，所以 30 档用一半的发布次数
    // 走完同样长的物理时间。若门控丢掉的时间没补回来，两者次数会一样多。
    const fast = mount({ config: () => ({ idleSwayAmp: 0, fpsCap: 60 }) });
    fast.pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    fast.frames.advance(RAF_DELAY_MS); // 先在 (0,0) 把链建起来
    fast.pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    fast.frames.runUntilIdle(2000);
    expect(fast.frames.pending()).toBe(0);

    const slow = mount({ config: () => ({ idleSwayAmp: 0, fpsCap: 30 }) });
    slow.pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    slow.frames.advance(RAF_DELAY_MS);
    slow.pointer.emit("pointermove", { clientX: 90, clientY: 0 });
    slow.frames.runUntilIdle(2000);
    expect(slow.frames.pending()).toBe(0);

    expect(fast.charm.physicsFrames).toBeGreaterThan(10);
    expect(slow.charm.physicsFrames).toBeLessThan(fast.charm.physicsFrames * 0.75);
    expect(slow.charm.physicsFrames).toBeGreaterThan(fast.charm.physicsFrames * 0.25);
  });

  it("指针静止两秒内没有任何定时器驱动的帧（调度只剩动画帧）", () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    try {
      const { charm, pointer, frames } = mount({ config: swaying(CHARM_FPS_CAP_DEFAULT) });
      pointer.emit("pointermove", { clientX: 10, clientY: 20 });

      frames.advance(2000);

      expect(charm.physicsFrames).toBeGreaterThan(0);
      expect(frames.requestAnimationFrame).toHaveBeenCalled();
      // 唯一调度是动画帧：两秒里一次 setTimeout 都没有过。
      expect(setTimeoutSpy).not.toHaveBeenCalled();
    } finally {
      setTimeoutSpy.mockRestore();
    }
  });

  it("pointermove 不自行发布：拖动时只有门控帧能发布，最坏延迟不超过 1000/上限", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: CHARM_IDLE_SWAY_AMP_DEFAULT, idleSwayHz: 0.8, fpsCap: 60 }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);
    const pathBefore = charm.tetherPath.value;
    const publishedBefore = charm.physicsFrames;

    // 指针移动 1ms 后：坐标变了，但画面与物理都还没动。
    frames.advance(1);
    pointer.emit("pointermove", { clientX: 300, clientY: 200 });
    expect(charm.pointer.value).toEqual({ x: 300, y: 200 });
    expect(charm.tetherPath.value).toBe(pathBefore);
    expect(charm.physicsFrames).toBe(publishedBefore);

    // 下一次刷新（≤ 1000/60 ≈ 16.7ms 后）才发布，且头节点已经到新位置。
    frames.advance(RAF_DELAY_MS);
    expect(charm.physicsFrames).toBe(publishedBefore + 1);
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(300 + DEFAULT_OFFSET.x, 1);
  });
});

describe("useCursorCharm：指针端偏移（票 #75，物理头节点 = 指针热点 + 偏移）", () => {
  it("默认偏移：头节点在光标右下 3px，渲染拿到的就是它（没有第二次偏移）", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    frames.advance(RAF_DELAY_MS);

    // 渲染几何的首点必须**恰好**是"指针 + 偏移"。如果渲染层再单独偏一次，
    // 这里会读到 106 / 106，而不是 103 / 103。
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(
      100 + CHARM_POINTER_OFFSET_X_DEFAULT,
      1,
    );
    expect(charm.tetherGeometry?.centerline[0]?.y).toBeCloseTo(
      100 + CHARM_POINTER_OFFSET_Y_DEFAULT,
      1,
    );
    // 站标挂在链尾：链是从**头节点**竖下来的，所以尾端在"偏移 + 绳长"处，
    // 而不是"指针 + 绳长"处。
    expect(charm.logoCenter.value.y).toBeCloseTo(
      100 + CHARM_POINTER_OFFSET_Y_DEFAULT + CHARM_ROPE_LENGTH_DEFAULT,
      0,
    );
  });

  it("负偏移：头节点落到光标左上（-6 / -9），指针坐标本身不被污染", () => {
    const { charm, pointer, frames } = mount({
      config: () => ({ idleSwayAmp: 0, pointerOffsetX: -6, pointerOffsetY: -9 }),
    });
    pointer.emit("pointermove", { clientX: 100, clientY: 200 });
    frames.advance(RAF_DELAY_MS);

    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(94, 1);
    expect(charm.tetherGeometry?.centerline[0]?.y).toBeCloseTo(191, 1);
    // 事件里写的坐标是原始热点：偏移只在换算头节点时加一次。
    expect(charm.pointer.value).toEqual({ x: 100, y: 200 });
  });

  it("改偏移不重建链：下一次发布就生效，链保持原姿被拖过去", () => {
    const offsets = ref({ x: 0, y: 0 });
    const { charm, pointer, frames } = mount({
      config: () => ({
        idleSwayAmp: 0,
        pointerOffsetX: offsets.value.x,
        pointerOffsetY: offsets.value.y,
      }),
    });
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);
    // 先让链在 (0, 0) 收敛：尾端稳定在正下方一个绳长处。
    frames.runUntilIdle(2000);
    expect(charm.logoCenter.value.y).toBeCloseTo(CHARM_ROPE_LENGTH_DEFAULT, 0);

    // 只改偏移（节点数与绳长不动）：下一次发布必须就跟着走。
    offsets.value = { x: 10, y: 10 };
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);

    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(10, 1);
    expect(charm.tetherGeometry?.centerline[0]?.y).toBeCloseTo(10, 1);
    // 重建会把整条链按新头节点重新竖下来（尾端 y ≈ 130）；这里尾端仍挂在原处
    // 附近、只被距离约束拖了一点，说明链没有被重建。
    expect(charm.logoCenter.value.y).toBeLessThan(CHARM_ROPE_LENGTH_DEFAULT + 5);
  });

  it("偏移不影响启用门控：触屏 / 减少动态下仍然零帧、零监听", () => {
    const offsetConfig = () => ({ idleSwayAmp: 0, pointerOffsetX: -6, pointerOffsetY: -6 });

    const touch = mount({ media: { [CHARM_FINE_POINTER_QUERY]: false }, config: offsetConfig });
    expect(touch.charm.enabled.value).toBe(false);
    expect(touch.pointer.total()).toBe(0);
    touch.frames.advance(1000);
    expect(touch.frames.pending()).toBe(0);

    const reduced = mount({ media: { [REDUCED]: true }, config: offsetConfig });
    expect(reduced.charm.enabled.value).toBe(false);
    expect(reduced.pointer.total()).toBe(0);
    reduced.frames.advance(1000);
    expect(reduced.frames.pending()).toBe(0);
  });
});

describe("useCursorCharm：失焦冻结与切后台暂停", () => {
  it("窗口失焦时冻结指针端而不是瞬移：头节点留在原处，链自然收敛", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    // 先让它跑一帧把链建起来，再跳一次指针：链被拖在后面，带着能量。
    frames.advance(RAF_DELAY_MS);
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
    frames.advance(RAF_DELAY_MS);
    pointer.emit("pointermove", { clientX: 160, clientY: 100 });
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
    pointer.emit("pointermove", { clientX: 100, clientY: 100 });
    frames.advance(RAF_DELAY_MS);
    pointer.emit("pointermove", { clientX: 160, clientY: 100 });
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
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(160 + DEFAULT_OFFSET.x, 1);
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

  it("没有注入动画帧（SSR）时移动不请求帧、也不发布（唯一发布口是门控帧）", () => {
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
    expect(charm.pointer.value).toEqual({ x: 10, y: 20 });
    expect(charm.tetherPath.value).toBe("");
  });
});
