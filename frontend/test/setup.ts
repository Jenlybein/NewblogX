import { afterEach, beforeEach, vi } from "vitest";
import { resetReaderSwrCache } from "~/utils/readerSwrCache";

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}

beforeEach(() => {
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  vi.stubGlobal("IntersectionObserver", IntersectionObserverStub);
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => window.setTimeout(callback, 0));
  vi.stubGlobal("cancelAnimationFrame", (id: number) => window.clearTimeout(id));
  // 30s SWR 缓存是模块级单例：用例之间必须清空，否则上一个用例写入的
  // "home-latest-articles#1" 会让下一个用例以为自己是缓存命中（假绿 / 假红都会发生）。
  resetReaderSwrCache();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
