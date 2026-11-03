import { readFileSync } from "node:fs";
import { join } from "node:path";

import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, shallowRef, Suspense } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { blockAfter } from "../helpers/designFiles";
import { installAwaitableAsyncData } from "../mocks/awaitableAsyncData";
import type { HistoryArticleItem } from "~/types/api";

/**
 * 浏览历史页（#194）：短卡片、悬浮移出按钮、独立失败态。
 *
 * 这是全仓第一个直接挂载页面 SFC 的用例，三处替身缺一不可：
 *   1. `useAsyncData` —— 页面里有顶层 await，`installAwaitableAsyncData` 给出与 Nuxt 同口径的 thenable；
 *      顶层 await 让 setup 变成异步，所以挂载时必须套一层 `<Suspense>`（Nuxt 页面本来就在其下）；
 *   2. `definePageMeta` / `useSeoMeta` —— Nuxt 在编译期消除的宏，这里以桩函数执行；
 *   3. `shallowRef` —— Nuxt 自动导入的 Vue API，页面源码里没有显式 import。
 *
 * service 层用 `vi.mock` 换掉：本票只关心页面如何消费 `getHistoryArticles` / `deleteHistoryArticles`
 * 的数据，不关心 HTTP 装配（那是 service 自己的职责）。
 */

const service = vi.hoisted(() => ({
  getHistoryArticles: vi.fn(),
  deleteHistoryArticles: vi.fn(),
}));

const message = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("~/services/studio", () => ({
  getHistoryArticles: service.getHistoryArticles,
  deleteHistoryArticles: service.deleteHistoryArticles,
}));

vi.mock("naive-ui", async () => {
  const { defineComponent: define, h: render } = await import("vue");
  return {
    NButton: define({
      name: "NButton",
      props: { disabled: { type: Boolean, default: false } },
      emits: ["click"],
      setup(props, { emit, slots }) {
        return () =>
          render(
            "button",
            { type: "button", disabled: props.disabled, onClick: () => emit("click") },
            slots.default?.(),
          );
      },
    }),
    useMessage: () => message,
  };
});

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    return () => h("a", { href: props.to }, slots.default?.());
  },
});

const StudioEmptyStateStub = defineComponent({
  props: { title: { type: String, default: "" }, description: { type: String, default: "" } },
  setup(props) {
    return () =>
      h("div", { class: "studio-empty-state", "data-state": "empty" }, [props.title, props.description]);
  },
});

function historyItem(overrides: Partial<HistoryArticleItem> = {}): HistoryArticleItem {
  return {
    article_id: "a1",
    user_id: "u1",
    title: "第一篇",
    cover: "",
    nickname: "浏览者甲",
    avatar: "",
    updated_at: "2026-11-01T00:00:00Z",
    ...overrides,
  };
}

async function mountHistoryPage() {
  const Page = (await import("~/pages/studio/history.vue")).default;
  const Host = defineComponent({
    setup() {
      return () => h(Suspense, null, { default: () => h(Page) });
    },
  });

  const wrapper = mount(Host, {
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        StudioEmptyState: StudioEmptyStateStub,
      },
    },
  });
  await flushPromises();
  return wrapper;
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll("button").find((item) => item.text().includes(text));
  if (!button) throw new Error(`找不到文案为「${text}」的按钮`);
  return button;
}

beforeEach(() => {
  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("shallowRef", shallowRef);
  installAwaitableAsyncData();
  service.getHistoryArticles.mockReset();
  service.deleteHistoryArticles.mockReset();
});

describe("浏览历史：短卡片栅格", () => {
  it("每篇文章一张短卡片：16:9 封面 + 标题 + 一行浏览时间，不再有摘要与昵称", async () => {
    service.getHistoryArticles.mockResolvedValue({
      list: [historyItem(), historyItem({ article_id: "a2", title: "第二篇" })],
      has_more: false,
    });

    const wrapper = await mountHistoryPage();
    const cards = wrapper.get(".article-history-grid").findAll(".article-history-card");

    expect(cards).toHaveLength(2);
    expect(cards[0]!.get(".article-history-card__cover").exists()).toBe(true);
    expect(cards[0]!.get(".article-history-card__title").text()).toBe("第一篇");
    expect(cards[0]!.get(".article-history-card__title").attributes("href")).toBe("/article/a1");

    const time = cards[0]!.get(".article-history-card__time").text();
    expect(time).toMatch(/\d{2}\/\d{2} \d{2}:\d{2}/u);

    // 短卡片只留标题与时间：摘要占位与浏览者昵称都不再渲染。
    expect(wrapper.findAll(".article-feed-abstract")).toHaveLength(0);
    expect(wrapper.text()).not.toContain("上次读到这里");
    expect(wrapper.text()).not.toContain("浏览者甲");
  });

  it("栅格随视口宽度 2 → 3 → 4 列递增（样式源码门禁）", () => {
    const source = readFileSync(join(process.cwd(), "app/pages/studio/history.vue"), "utf8");
    const columns = (block: string, count: number) =>
      new RegExp(`grid-template-columns:\\s*repeat\\(${count},`, "u").test(block);

    expect(columns(blockAfter(source, ".article-history-grid {"), 2)).toBe(true);
    expect(
      columns(blockAfter(blockAfter(source, "@media (min-width: 768px)"), ".article-history-grid {"), 3),
    ).toBe(true);
    expect(
      columns(blockAfter(blockAfter(source, "@media (min-width: 1200px)"), ".article-history-grid {"), 4),
    ).toBe(true);
  });

  it("卡片明显小于合集卡片：标题字号更小，且没有摘要段落", () => {
    const source = readFileSync(join(process.cwd(), "app/pages/studio/history.vue"), "utf8");
    const collectionSource = readFileSync(join(process.cwd(), "app/pages/collections/index.vue"), "utf8");
    const fontSizeOf = (css: string, selector: string) => {
      const found = /font-size:\s*([0-9.]+)rem/u.exec(blockAfter(css, selector));
      if (!found) throw new Error(`${selector} 没有 rem 字号`);
      return Number(found[1]);
    };

    expect(fontSizeOf(source, ".article-history-card__title {")).toBeLessThan(
      fontSizeOf(collectionSource, ".collection-card__title {"),
    );
    expect(source).not.toContain("article-history-card__abstract");
  });
});

describe("浏览历史：移出按钮收进卡片右上角", () => {
  it("按钮带无障碍文案、窄屏常显的样式，点击后移出并刷新列表", async () => {
    const remaining = [historyItem({ article_id: "a2", title: "第二篇" })];
    service.getHistoryArticles
      .mockResolvedValueOnce({ list: [historyItem(), ...remaining], has_more: false })
      .mockResolvedValueOnce({ list: remaining, has_more: false });

    const wrapper = await mountHistoryPage();
    const remove = wrapper.get(".article-history-card__remove");

    expect(remove.attributes("aria-label")).toBe("移出历史");
    expect(remove.attributes("type")).toBe("button");

    await remove.trigger("click");
    await flushPromises();

    expect(service.deleteHistoryArticles).toHaveBeenCalledWith(["a1"]);
    expect(message.success).toHaveBeenCalledWith("已移出浏览历史");
    expect(service.getHistoryArticles).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll(".article-history-card")).toHaveLength(1);
    expect(wrapper.get(".article-history-card__title").text()).toBe("第二篇");

    const source = readFileSync(join(process.cwd(), "app/pages/studio/history.vue"), "utf8");
    const narrow = blockAfter(source, "@media (max-width: 768px)");
    expect(narrow).toContain(".article-history-card__remove");
  });
});

describe("浏览历史：加载失败有独立失败态", () => {
  it("接口失败渲染失败态，而不是落进「还没有浏览历史」空态", async () => {
    service.getHistoryArticles.mockRejectedValue(new Error("网络错误"));

    const wrapper = await mountHistoryPage();

    expect(wrapper.get(".article-history-error").text()).toContain("浏览历史暂时打不开");
    expect(wrapper.find(".studio-empty-state").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("还没有浏览历史");
  });

  it("失败态提供重试入口，重试会重新请求", async () => {
    service.getHistoryArticles.mockRejectedValueOnce(new Error("网络错误")).mockResolvedValueOnce({
      list: [historyItem()],
      has_more: false,
    });

    const wrapper = await mountHistoryPage();
    await buttonByText(wrapper, "重新加载").trigger("click");
    await flushPromises();

    expect(service.getHistoryArticles).toHaveBeenCalledTimes(2);
    expect(wrapper.find(".article-history-error").exists()).toBe(false);
    expect(wrapper.findAll(".article-history-card")).toHaveLength(1);
  });
});

describe("浏览历史：空态与分页保留", () => {
  it("成功但列表为空时仍是「还没有浏览历史」空态", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [], has_more: false });

    const wrapper = await mountHistoryPage();

    expect(wrapper.get(".studio-empty-state").text()).toContain("还没有浏览历史");
    expect(wrapper.find(".article-history-error").exists()).toBe(false);
  });

  it("保留分页与刷新记录", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: true });

    const wrapper = await mountHistoryPage();
    expect(wrapper.text()).toContain("第 1 页");
    expect(buttonByText(wrapper, "下一页").attributes("disabled")).toBeUndefined();

    await buttonByText(wrapper, "下一页").trigger("click");
    expect(wrapper.text()).toContain("第 2 页");

    await buttonByText(wrapper, "刷新记录").trigger("click");
    await flushPromises();
    expect(service.getHistoryArticles).toHaveBeenCalledTimes(2);
  });
});
