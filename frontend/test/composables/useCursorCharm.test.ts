import { nextTick, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useCursorCharm } from "~/composables/useCursorCharm";
import { CHARM_FINE_POINTER_QUERY } from "~/utils/decoCharm";

/**
 * 为什么值得测：这段决定挂件"到底有没有开始工作"。
 * 后台关掉开关后只要还绑着 pointermove，就等于留了一个每次移动都跑的回调；
 * 触屏上绑了它则会出现"跟着手指的绳子"。两种都是看不见的成本，只能靠断言钉住。
 */
const REDUCED = "(prefers-reduced-motion: reduce)";

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

function fakeTarget() {
  const listeners = new Set<(event: { clientX: number; clientY: number }) => void>();
  return {
    target: {
      addEventListener: vi.fn(
        (_type: "pointermove", listener: (event: { clientX: number; clientY: number }) => void) => {
          listeners.add(listener);
        },
      ),
      removeEventListener: vi.fn(
        (_type: "pointermove", listener: (event: { clientX: number; clientY: number }) => void) => {
          listeners.delete(listener);
        },
      ),
    },
    listeners,
    emit(event: { clientX: number; clientY: number }) {
      for (const listener of listeners) listener(event);
    },
  };
}

const desktop = () => fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true });

describe("useCursorCharm：启用条件", () => {
  it("精确指针 + 未减少动态 + 总开关开着时启用并绑定 pointermove", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(charm.enabled.value).toBe(true);
    expect(pointer.target.addEventListener).toHaveBeenCalledTimes(1);
    expect(pointer.listeners.size).toBe(1);
  });

  it("触屏（无 hover/精确指针）不启用，也不绑定任何指针监听", () => {
    const media = fakeMedia({});
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.target.addEventListener).not.toHaveBeenCalled();
  });

  it("系统要求减少动态时不启用（整站不出现，不是放慢）", () => {
    const media = fakeMedia({ [CHARM_FINE_POINTER_QUERY]: true, [REDUCED]: true });
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.target.addEventListener).not.toHaveBeenCalled();
  });

  it("后台总开关关闭时不启用，也不绑定指针监听（运行时成本归零）", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => false,
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.target.addEventListener).not.toHaveBeenCalled();
    expect(pointer.listeners.size).toBe(0);
    // 连"系统偏好变了"都不需要知道：关掉之后一个监听都不留。
    expect(media.listeners.size).toBe(0);
  });

  it("没有 matchMedia（SSR）时关闭且不报错", () => {
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: () => null,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(charm.enabled.value).toBe(false);
    expect(pointer.target.addEventListener).not.toHaveBeenCalled();
  });

  it("开关后来被打开时补上监听，再关掉时摘掉（后台改完刷新即生效）", async () => {
    const media = desktop();
    const pointer = fakeTarget();
    const configEnabled = ref(false);
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => configEnabled.value,
    });

    expect(pointer.listeners.size).toBe(0);
    expect(media.listeners.size).toBe(0);

    configEnabled.value = true;
    await nextTick();
    expect(charm.enabled.value).toBe(true);
    expect(pointer.listeners.size).toBe(1);
    expect(media.listeners.size).toBe(2);

    configEnabled.value = false;
    await nextTick();
    expect(pointer.listeners.size).toBe(0);
    expect(media.listeners.size).toBe(0);
  });
});

describe("useCursorCharm：指针位置与首次淡入", () => {
  it("首次移动前不可见（页面刚打开不该先冒出一个东西）", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(charm.visible.value).toBe(false);
  });

  it("首次指针移动后记录坐标并淡入", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    pointer.emit({ clientX: 120, clientY: 240 });

    expect(charm.pointer.value).toEqual({ x: 120, y: 240 });
    expect(charm.visible.value).toBe(true);
  });

  it("后续移动只更新坐标，不会来回切换可见性", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    pointer.emit({ clientX: 10, clientY: 20 });
    pointer.emit({ clientX: 30, clientY: 40 });

    expect(charm.pointer.value).toEqual({ x: 30, y: 40 });
    expect(charm.visible.value).toBe(true);
  });

  it("没有注入 window（SSR）时移动也不会炸", () => {
    const media = desktop();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: null,
      configEnabled: () => true,
    });

    expect(() => charm.dispose()).not.toThrow();
    expect(charm.visible.value).toBe(false);
  });
});

describe("useCursorCharm：清理", () => {
  it("dispose 摘掉指针监听与媒体监听", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    expect(pointer.listeners.size).toBe(1);
    expect(media.listeners.size).toBe(2);

    charm.dispose();

    expect(pointer.listeners.size).toBe(0);
    expect(media.listeners.size).toBe(0);
  });

  it("重复 dispose 不抛错（HMR / 路由重挂）", () => {
    const media = desktop();
    const pointer = fakeTarget();
    const charm = useCursorCharm({
      matchMedia: media.matchMedia,
      target: pointer.target,
      configEnabled: () => true,
    });

    charm.dispose();
    expect(() => charm.dispose()).not.toThrow();
  });
});
