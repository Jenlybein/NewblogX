import { describe, expect, it, vi } from "vitest";

import { usePointerSpecular } from "~/composables/usePointerSpecular";

/**
 * 指针镜面高光的接线。
 *
 * 纯坐标换算在 utils/specular（单测），这里测：只在允许的场合启用（桌面 + 非减少动态）、
 * 往元素上写 `--mx/--my`、移动端与减少动态下完全不写、销毁时摘掉监听。
 */
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

function fakeElement(rect = { left: 0, top: 0, width: 200, height: 100 }) {
  const properties = new Map<string, string>();
  return {
    element: {
      getBoundingClientRect: () => rect,
      style: {
        setProperty: vi.fn((name: string, value: string) => {
          properties.set(name, value);
        }),
      },
    },
    properties,
  };
}

const REDUCED = "(prefers-reduced-motion: reduce)";
const MOBILE = "(max-width: 768px)";

describe("usePointerSpecular：启用条件", () => {
  it("桌面 + 未减少动态时启用", () => {
    const media = fakeMedia({});
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    expect(specular.enabled.value).toBe(true);
  });

  it("移动端（≤768px）关闭（省一次每帧的重绘）", () => {
    const media = fakeMedia({ [MOBILE]: true });
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    expect(specular.enabled.value).toBe(false);
  });

  it("减少动态时关闭（高光是被禁的增益效果）", () => {
    const media = fakeMedia({ [REDUCED]: true });
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    expect(specular.enabled.value).toBe(false);
  });

  it("没有 matchMedia（SSR）时关闭，不报错", () => {
    const specular = usePointerSpecular({ matchMedia: () => null });

    expect(specular.enabled.value).toBe(false);
  });

  it("系统设置变化时跟着开关（视口从手机转桌面 / 打开减少动态）", () => {
    const media = fakeMedia({ [MOBILE]: false });
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    expect(specular.enabled.value).toBe(true);

    for (const listener of media.listeners) listener({ matches: true });

    expect(specular.enabled.value).toBe(false);
  });
});

describe("usePointerSpecular：写入 --mx / --my", () => {
  it("按元素矩形换算成百分比并写 CSS 变量（不改布局，不读 scroll）", () => {
    const media = fakeMedia({});
    const { element, properties } = fakeElement();
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    specular.handlePointerMove({ clientX: 100, clientY: 50, currentTarget: element as never });

    expect(properties.get("--mx")).toBe("50%");
    expect(properties.get("--my")).toBe("50%");
  });

  it("关闭时不写任何变量（移动端 / 减少动态）", () => {
    const media = fakeMedia({ [REDUCED]: true });
    const { element, properties } = fakeElement();
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    specular.handlePointerMove({ clientX: 120, clientY: 90, currentTarget: element as never });

    expect(properties.size).toBe(0);
    expect(element.style.setProperty).not.toHaveBeenCalled();
  });

  it("没有 currentTarget 或尺寸为 0 时安全跳过（首帧 / 隐藏元素）", () => {
    const media = fakeMedia({});
    const { element, properties } = fakeElement({ left: 0, top: 0, width: 0, height: 0 });
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    specular.handlePointerMove({ clientX: 10, clientY: 10, currentTarget: null });
    expect(properties.size).toBe(0);

    specular.handlePointerMove({ clientX: 10, clientY: 10, currentTarget: element as never });
    expect(properties.size).toBe(0);
  });

  it("销毁时摘掉媒体监听", () => {
    const media = fakeMedia({});
    const specular = usePointerSpecular({ matchMedia: media.matchMedia });

    expect(media.listeners.size).toBe(2);
    specular.dispose();
    expect(media.listeners.size).toBe(0);
  });
});
