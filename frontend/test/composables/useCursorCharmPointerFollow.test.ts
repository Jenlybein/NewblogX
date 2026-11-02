import { describe, expect, it } from "vitest";

import { useCursorCharm } from "~/composables/useCursorCharm";
import { REDUCED_MOTION_QUERY } from "~/composables/useMotionPreference";
import {
  CHARM_FINE_POINTER_QUERY,
  CHARM_POINTER_OFFSET_X_DEFAULT,
  CHARM_POINTER_OFFSET_Y_DEFAULT,
  type CharmPoint,
} from "~/utils/decoCharm";

/**
 * 指针端平滑跟随的**接线**用例。
 *
 * 物理本身由 `test/utils/decoCharmPointerFollow.test.ts` 断言；这里只管一件事：
 * 组合式喂给物理的指针端锚点是否真的被"平滑跟随"了 —— 指针瞬移时渲染出去的
 * 指针端节点不能一步跳到锚点上（那就是硬赋值），而必须逐帧靠近、始终不脱节、
 * 停手后精确落回「指针 + 偏移」并停帧。
 */

type Listener = (event?: unknown) => void;

const RAF_DELAY_MS = 16.667;

function fakeMedia(map: Record<string, boolean>) {
  return {
    matchMedia: (query: string) => ({
      matches: map[query] ?? false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }),
  };
}

function fakeHost(initialHidden = false) {
  const listeners = new Map<string, Set<Listener>>();
  return {
    host: {
      hidden: initialHidden,
      addEventListener: (type: string, listener: Listener) => {
        const set = listeners.get(type) ?? new Set<Listener>();
        set.add(listener);
        listeners.set(type, set);
      },
      removeEventListener: (type: string, listener: Listener) => {
        listeners.get(type)?.delete(listener);
      },
    },
    emit(type: string, event?: unknown) {
      for (const listener of listeners.get(type) ?? []) listener(event);
    },
  };
}

/** 可手动推进的动画帧队列与虚拟时钟（与主用例同构，只留需要的部分）。 */
function fakeFrames(frameDelayMs: number = RAF_DELAY_MS) {
  interface Scheduled {
    id: number;
    due: number;
    callback: Listener;
  }
  const queue = new Map<number, Scheduled>();
  let nextId = 1;
  const clock = { now: 0 };
  return {
    now: () => clock.now,
    requestAnimationFrame: (callback: Listener) => {
      const id = nextId;
      nextId += 1;
      queue.set(id, { id, due: clock.now + frameDelayMs, callback });
      return id;
    },
    cancelAnimationFrame: (id: number) => {
      queue.delete(id);
    },
    pending: () => queue.size,
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
        next.callback(clock.now);
      }
      clock.now = end;
    },
  };
}

function mount() {
  const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true });
  const pointer = fakeHost();
  const doc = fakeHost();
  const frames = fakeFrames();
  const charm = useCursorCharm({
    matchMedia: media.matchMedia,
    target: pointer.host,
    visibilityTarget: doc.host,
    configEnabled: () => true,
    // 关掉轻晃：静止后要能停帧，才能断言"落回锚点"。
    config: () => ({ idleSwayAmp: 0 }),
    requestAnimationFrame: frames.requestAnimationFrame,
    cancelAnimationFrame: frames.cancelAnimationFrame,
    now: frames.now,
  });
  return { charm, pointer, frames };
}

const OFFSET = { x: CHARM_POINTER_OFFSET_X_DEFAULT, y: CHARM_POINTER_OFFSET_Y_DEFAULT };

/** 渲染出去的指针端节点（中心线首点就是链的第 0 节）。 */
function renderedHead(charm: ReturnType<typeof useCursorCharm>): CharmPoint {
  const head = charm.tetherGeometry?.centerline[0];
  return { x: head?.x ?? Number.NaN, y: head?.y ?? Number.NaN };
}

/** 当前锚点：指针热点 + 指针端偏移（渲染层不再单独偏一次）。 */
function currentAnchor(charm: ReturnType<typeof useCursorCharm>): CharmPoint {
  return { x: charm.pointer.value.x + OFFSET.x, y: charm.pointer.value.y + OFFSET.y };
}

function distance(a: CharmPoint, b: CharmPoint): number {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

describe("useCursorCharm：指针端平滑跟随", () => {
  it("指针一帧跨 240px 时指针端不瞬移：落后一截、但仍在阈值内", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);

    pointer.emit("pointermove", { clientX: 240, clientY: 0 });
    frames.advance(RAF_DELAY_MS);

    const lag = distance(renderedHead(charm), currentAnchor(charm));
    // 硬赋值时滞后恒为 0（指针端一步到位）；平滑跟随必须"动了但没到"。
    expect(lag).toBeGreaterThan(4);
    expect(lag).toBeLessThan(24);
    // 指针端确实朝指针动了，不是原地不动。
    expect(renderedHead(charm).x).toBeGreaterThan(50);
  });

  it("连续快速拖动全程不脱节：指针端与「指针 + 偏移」的距离始终 ≤ 24px", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);

    let elapsedMs = 0;
    let worstLag = 0;
    // 60 帧、每帧两次 pointermove（≈120Hz 报点率），900px / 0.25s 的横向甩动。
    for (let frame = 0; frame < 60; frame += 1) {
      for (let event = 0; event < 2; event += 1) {
        elapsedMs += RAF_DELAY_MS / 2;
        const seconds = elapsedMs / 1000;
        pointer.emit("pointermove", {
          clientX: 900 * Math.min(1, seconds / 0.25),
          clientY: 120 * Math.sin(seconds * 20),
        });
      }
      frames.advance(RAF_DELAY_MS);
      worstLag = Math.max(worstLag, distance(renderedHead(charm), currentAnchor(charm)));
    }

    expect(worstLag).toBeLessThan(24);
  });

  it("停手后精确落回「指针 + 偏移」并停帧：静止姿态不能被跟随残留污染", () => {
    const { charm, pointer, frames } = mount();
    pointer.emit("pointermove", { clientX: 0, clientY: 0 });
    frames.advance(RAF_DELAY_MS);
    pointer.emit("pointermove", { clientX: 400, clientY: 60 });

    // 跑到环不再请求新帧（收敛停帧）。
    for (let frame = 0; frame < 900 && frames.pending() > 0; frame += 1) {
      frames.advance(RAF_DELAY_MS);
    }

    expect(frames.pending()).toBe(0);
    expect(charm.tetherGeometry?.centerline[0]?.x).toBeCloseTo(400 + OFFSET.x, 1);
    expect(charm.tetherGeometry?.centerline[0]?.y).toBeCloseTo(60 + OFFSET.y, 1);
  });

  it("减少动态 / 触屏下没有任何跟随（整层不渲染，也不建链）", () => {
    const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: false, [REDUCED_MOTION_QUERY]: true });
    const pointer = fakeHost();
    const frames = fakeFrames();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.host,
      visibilityTarget: fakeHost().host,
      configEnabled: () => true,
      config: () => ({ idleSwayAmp: 0 }),
      requestAnimationFrame: frames.requestAnimationFrame,
      cancelAnimationFrame: frames.cancelAnimationFrame,
      now: frames.now,
    });

    pointer.emit("pointermove", { clientX: 600, clientY: 0 });
    frames.advance(1000);

    expect(charm.enabled.value).toBe(false);
    expect(charm.tetherGeometry).toBeNull();
    expect(frames.pending()).toBe(0);
  });
});
