import { describe, expect, it, vi } from "vitest";

import { whenFontsReady } from "~/utils/fontsReady";

/**
 * 字体就绪重测的唯一入口（目录树圆点与标题大纲轨道共用）。
 *
 * 这里钉三件事：**就绪后跑一次**、**没有 document（SSR）时安静返回**、
 * **加载失败时静默吞掉**。第三件尤其重要：如果失败没人接，`onMounted` 里那条
 * Promise 链会变成未处理的 rejection，整块目录表现成"莫名不测量"。
 */

/** 把 `document.fonts.ready` 换成测试给的 Promise；返回还原函数。 */
function stubFontsReady(ready: Promise<void>): () => void {
  const original = Object.getOwnPropertyDescriptor(document, "fonts");
  Object.defineProperty(document, "fonts", { configurable: true, value: { ready } });

  return () => {
    if (original) {
      Object.defineProperty(document, "fonts", original);
    } else {
      Reflect.deleteProperty(document, "fonts");
    }
  };
}

describe("whenFontsReady：字体就绪后的那一次重测", () => {
  it("就绪之前不跑，兑现之后跑且只跑一次", async () => {
    let release: () => void = () => {};
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    const restore = stubFontsReady(ready);
    const run = vi.fn();

    try {
      whenFontsReady(run);
      expect(run).not.toHaveBeenCalled();

      release();
      await ready;
      await Promise.resolve();

      expect(run).toHaveBeenCalledTimes(1);
    } finally {
      restore();
    }
  });

  it("没有 document（SSR / 老环境）时安静返回：不抛错、不跑回调", () => {
    const run = vi.fn();

    vi.stubGlobal("document", undefined);
    try {
      expect(() => whenFontsReady(run)).not.toThrow();
      expect(run).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("字体加载失败时静默吞掉：没有未处理的 rejection，也不跑回调", async () => {
    const ready = Promise.reject(new Error("font load failed"));
    const restore = stubFontsReady(ready);
    const run = vi.fn();

    try {
      expect(() => whenFontsReady(run)).not.toThrow();
      await ready.catch(() => undefined);
      await Promise.resolve();

      expect(run).not.toHaveBeenCalled();
    } finally {
      restore();
    }
  });
});
