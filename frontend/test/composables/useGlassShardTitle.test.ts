import { mount } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useGlassShardTitle, GLASS_SCRAMBLE_CHARS } from "~/composables/useGlassShardTitle";
import { GLASS_SHARDS } from "~/utils/glassShards";

/**
 * 「碎玻璃标题」的接线测试。
 *
 * 几何数据在 utils/glassShards（这里只核对形状），真正要钉住的是三件事：
 *   1. 点击后文字真的会乱码、并在碎裂结束时逐字解码回原文（不是"点了没反应"）；
 *   2. 乱码只在这个标题自己的文本节点上发生，不碰页面别处；
 *   3. 「减少动态」时点击完全不出效果（文字不动、柔光不亮）—— 这是无障碍承诺，
 *      不是"效果弱一点"。
 */
const TextHost = defineComponent({
  props: { text: { type: String, required: true } },
  setup(props) {
    const root = ref<HTMLElement | null>(null);
    useGlassShardTitle(root, { text: () => props.text });
    return () =>
      h("span", { ref: root, class: "glass-title" }, [
        h("span", { class: "glass-title__halo" }),
        h("span", { class: "glass-title__base", "data-glass-text": "" }, props.text),
        h(
          "span",
          { class: "glass-title__shards" },
          GLASS_SHARDS.map((shard) =>
            h("span", { "data-glass-shard": "", "data-shard-id": shard.id }, [
              h("span", { "data-glass-text": "" }, props.text),
            ]),
          ),
        ),
      ]);
  },
});

const TITLE = "JELLYBIN";

function mountTitle() {
  return mount(TextHost, { props: { text: TITLE }, attachTo: document.body });
}

function visibleTexts(wrapper: ReturnType<typeof mountTitle>): string[] {
  return wrapper.findAll("[data-glass-text]").map((node) => node.text());
}

/**
 * 「已乱码」= 不再是原文，且每个字符都来自乱码字符集（空格保留）。
 * 注意字符集里**有** `0` / `1`，所以不能拿"不含字母数字"当判据。
 */
function isScrambled(text: string): boolean {
  return (
    text !== TITLE &&
    [...text].every((char) => char === " " || GLASS_SCRAMBLE_CHARS.includes(char))
  );
}

describe("碎玻璃标题：点击乱码 → 解码", () => {
  beforeEach(() => {
    // 帧循环用 setTimeout 兜底的话，假时钟会被 0ms 的 rAF 反复触发；这里直接停掉帧循环，
    // 只测"文字状态机"，几何动效不在单测范围（靠真机截图验收）。
    vi.stubGlobal("requestAnimationFrame", vi.fn(() => 1));
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.useFakeTimers();
  });

  it("点击后进入乱码，碎裂结束时逐字解码回原文", async () => {
    const wrapper = mountTitle();
    const root = wrapper.find(".glass-title");

    expect(visibleTexts(wrapper)).toEqual(Array.from({ length: GLASS_SHARDS.length + 1 }, () => TITLE));

    await root.trigger("pointerup");
    // 柔光先亮起来（点击反馈的可见部分）
    expect(wrapper.find(".glass-title__halo").classes()).toContain("is-active");

    vi.advanceTimersByTime(40);
    expect(visibleTexts(wrapper).every(isScrambled)).toBe(true);

    // 1s 碎裂结束 → 开始解码；再跑够 20 步 × 20ms 之后必须回到原文
    vi.advanceTimersByTime(1000 + 20 * 21);
    expect(wrapper.find(".glass-title__halo").classes()).not.toContain("is-active");
    expect(visibleTexts(wrapper)).toEqual(Array.from({ length: GLASS_SHARDS.length + 1 }, () => TITLE));

    wrapper.unmount();
  });

  it("卸载后点击不再改文字（监听已摘掉）", () => {
    const wrapper = mountTitle();
    wrapper.unmount();

    expect(visibleTexts(wrapper).every((text) => text === TITLE)).toBe(true);
  });
});

describe("碎玻璃标题：减少动态时不出效果", () => {
  beforeEach(() => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    vi.stubGlobal("requestAnimationFrame", vi.fn(() => 1));
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.useFakeTimers();
  });

  it("点击不炸开、不乱码、不亮柔光", async () => {
    const wrapper = mountTitle();

    await wrapper.find(".glass-title").trigger("pointerup");
    vi.advanceTimersByTime(2000);

    expect(wrapper.find(".glass-title__halo").classes()).not.toContain("is-active");
    expect(visibleTexts(wrapper).every((text) => text === TITLE)).toBe(true);

    wrapper.unmount();
  });
});

describe("碎玻璃几何数据", () => {
  it("9 块碎片：6 块底片 + 3 块浮层（浮层带毛玻璃与更大位移）", () => {
    expect(GLASS_SHARDS).toHaveLength(9);
    expect(GLASS_SHARDS.filter((shard) => shard.z === 1)).toHaveLength(6);
    expect(GLASS_SHARDS.filter((shard) => shard.z === 2)).toHaveLength(3);
  });

  it("每块都是一条合法的 polygon，且 id 不重复", () => {
    const ids = new Set<number>();

    for (const shard of GLASS_SHARDS) {
      expect(shard.path).toMatch(/^polygon\([0-9%,. ]+\)$/u);
      expect(ids.has(shard.id)).toBe(false);
      ids.add(shard.id);
    }
  });
});
