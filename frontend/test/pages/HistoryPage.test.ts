import { readFileSync } from "node:fs";
import { join } from "node:path";

import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";
import HistoryPage from "~/pages/history.vue";
import type { HistoryArticleItem } from "~/types/api";

/**
 * 浏览历史独立页 `/history`（#241）。
 *
 * 页面 SFC 的三处替身（与 StudioHistoryPage 用例同理）：
 *   1. `definePageMeta` / `useSeoMeta` —— Nuxt 编译期宏，这里以桩函数执行；
 *   2. `naive-ui` —— 只留输入框 / 按钮与消息、对话框的极小实现；
 *   3. service —— 换掉 HTTP 装配，只观测页面如何消费游标、搜索、删除与清空。
 *
 * 想看的行为：时间轴分组（今天 / 昨天 / 近一周 / 更早）、卡片只有封面 + 两行标题 +
 * 浏览时间、标题行右侧垃圾桶、搜索走服务端 keyword、清空要二次确认、批量管理可勾选。
 */

const service = vi.hoisted(() => ({
  getHistoryArticles: vi.fn(),
  deleteHistoryArticles: vi.fn(),
  clearHistoryArticles: vi.fn(),
}));

const message = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}));

const dialog = vi.hoisted(() => ({
  warning: vi.fn(),
}));

vi.mock("~/services/history", () => ({
  getHistoryArticles: service.getHistoryArticles,
  deleteHistoryArticles: service.deleteHistoryArticles,
  clearHistoryArticles: service.clearHistoryArticles,
}));

vi.mock("naive-ui", async () => {
  const { defineComponent: define, h: render } = await import("vue");
  return {
    NButton: define({
      name: "NButton",
      props: {
        disabled: { type: Boolean, default: false },
        size: { type: String, default: "" },
        type: { type: String, default: "" },
        quaternary: { type: Boolean, default: false },
        loading: { type: Boolean, default: false },
      },
      emits: ["click"],
      setup(props, { emit, slots }) {
        return () =>
          render(
            "button",
            {
              type: "button",
              disabled: props.disabled,
              "data-size": props.size,
              onClick: () => emit("click"),
            },
            slots.default?.(),
          );
      },
    }),
    NInput: define({
      name: "NInput",
      props: {
        value: { type: String, default: "" },
        placeholder: { type: String, default: "" },
      },
      emits: ["update:value", "keyup"],
      setup(props, { emit }) {
        return () =>
          render("input", {
            type: "search",
            value: props.value,
            placeholder: props.placeholder,
            onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
            onKeyup: (event: KeyboardEvent) => emit("keyup", event),
          });
      },
    }),
    useMessage: () => message,
    useDialog: () => dialog,
  };
});

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    return () => h("a", { href: props.to }, slots.default?.());
  },
});

const ArticleCoverStub = defineComponent({
  props: { src: { type: String, default: "" }, title: { type: String, default: "" } },
  setup(props) {
    return () => h("div", { class: "article-cover", "data-src": props.src, "data-title": props.title });
  },
});

const LoadingSkeletonStub = defineComponent({
  props: { label: { type: String, default: "" } },
  setup(props) {
    return () => h("div", { class: "skeleton", "data-role": "skeleton" }, props.label);
  },
});

const ProfileEmptyStateStub = defineComponent({
  props: {
    title: { type: String, default: "" },
    description: { type: String, default: "" },
  },
  setup(props) {
    return () => h("div", { class: "history-empty", "data-state": "empty" }, [props.title, props.description]);
  },
});

/** 今天的某个整点（无论现在几点，都会落在本地日历日「今天」）。 */
function todayAt(hour = 9): string {
  const date = new Date();
  date.setHours(hour, 0, 0, 0);
  return date.toISOString();
}

/** n 个自然日之前的某个整点。 */
function daysAgoAt(days: number, hour = 9): string {
  const date = new Date();
  date.setDate(date.getDate() - days);
  date.setHours(hour, 0, 0, 0);
  return date.toISOString();
}

function historyItem(overrides: Partial<HistoryArticleItem> = {}): HistoryArticleItem {
  return {
    article_id: "a1",
    user_id: "u1",
    title: "第一篇",
    cover: "",
    nickname: "浏览者甲",
    avatar: "",
    updated_at: todayAt(9),
    ...overrides,
  };
}

function mountPage() {
  return mount(HistoryPage, {
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        ArticleCover: ArticleCoverStub,
        LoadingSkeleton: LoadingSkeletonStub,
        ProfileEmptyState: ProfileEmptyStateStub,
      },
    },
  });
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll("button").find((item) => item.text().includes(text));
  if (!button) throw new Error(`找不到文案为「${text}」的按钮`);
  return button;
}

beforeEach(() => {
  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  // 与 definePageMeta / useSeoMeta 同理：这是 Nuxt 的自动导入，单测环境里没有
  // 编译期注入，只能由用例桩掉。#314 起历史页声明了 noindex（useNoIndexPage）。
  vi.stubGlobal("useNoIndexPage", () => undefined);
  service.getHistoryArticles.mockReset();
  service.deleteHistoryArticles.mockReset();
  service.clearHistoryArticles.mockReset();
  message.success.mockReset();
  message.error.mockReset();
  dialog.warning.mockReset();
});

describe("版式：左侧时间轴 + 右侧卡片栅格", () => {
  it("按今天 / 昨天 / 更早分组，组标签在时间轴上", async () => {
    service.getHistoryArticles.mockResolvedValue({
      list: [
        historyItem({ article_id: "a1", title: "今天的" }),
        historyItem({ article_id: "a2", title: "昨天的", updated_at: daysAgoAt(1, 21) }),
        historyItem({ article_id: "a3", title: "更早的", updated_at: daysAgoAt(8) }),
      ],
      has_more: false,
    });

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.findAll(".history-group__label").map((label) => label.text())).toEqual([
      "今天",
      "昨天",
      "更早",
    ]);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(3);
    expect(wrapper.findAll(".history-group__dot")).toHaveLength(3);
    expect(wrapper.find(".history-timeline").exists()).toBe(true);
  });

  it("卡片只显示封面 + 标题（两行截断）+ 浏览时间，没有任何分类/作者/时长角标", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: false });

    const wrapper = mountPage();
    await flushPromises();

    const card = wrapper.get(".article-mini-card");
    expect(card.get(".article-mini-card__cover").exists()).toBe(true);
    expect(card.get(".article-mini-card__title").text()).toBe("第一篇");
    expect(card.get(".article-mini-card__title").attributes("href")).toBe("/article/a1");
    expect(card.get(".article-mini-card__meta").text()).toContain("浏览于");
    expect(card.text()).not.toContain("浏览者甲");
    for (const banned of ["分类", "标签", "阅读时长"]) {
      expect(card.text()).not.toContain(banned);
    }
  });

  it("栅格 4–5 列自适应（样式源码门禁）", () => {
    const source = readFileSync(join(process.cwd(), "app/pages/history.vue"), "utf8");
    const columns = (block: string, count: number) =>
      new RegExp(`grid-template-columns:\\s*repeat\\(${count},`, "u").test(block);

    expect(columns(blockAfter(source, ".article-history-grid {"), 2)).toBe(true);
    expect(
      columns(blockAfter(blockAfter(source, "@media (min-width: 1024px)"), ".article-history-grid {"), 4),
    ).toBe(true);
    expect(
      columns(blockAfter(blockAfter(source, "@media (min-width: 1440px)"), ".article-history-grid {"), 5),
    ).toBe(true);
  });

  it("色值只走语义 token，不写裸色值；标题两行截断", () => {
    const source = stripComments(readFileSync(join(process.cwd(), "app/pages/history.vue"), "utf8"));

    expect(source).not.toMatch(/#[0-9a-fA-F]{3,8}\b/u);
    expect(source).not.toMatch(/\b(?:rgb|rgba|hsl|hsla)\(/u);
    for (const token of ["--color-surface-raised", "--color-line-faint", "--color-line", "--color-ink"]) {
      expect(source).toContain(`var(${token})`);
    }

    // 标题两行截断从票 327 起归共享卡片组件：历史与收藏必须是同一个形状，
    // 所以形状本身不在本页源码里，判据要读组件。
    const miniCard = stripComments(readFileSync(join(process.cwd(), "app/components/article/ArticleMiniCard.vue"), "utf8"));
    expect(blockAfter(miniCard, ".article-mini-card__title {")).toContain("-webkit-line-clamp: 2");
  });
});

describe("顶部操作：搜索标题（服务端 keyword）", () => {
  it("输入关键词后按搜索按钮，带上 keyword 重新取第一页", async () => {
    service.getHistoryArticles
      .mockResolvedValueOnce({ list: [historyItem({ title: "Rust 所有权" })], has_more: false })
      .mockResolvedValueOnce({
        list: [historyItem({ article_id: "a9", title: "Go 并发模型" })],
        has_more: false,
      });

    const wrapper = mountPage();
    await flushPromises();

    await wrapper.get(".history-search input").setValue("Go");
    await buttonByText(wrapper, "搜索").trigger("click");
    await flushPromises();

    expect(service.getHistoryArticles).toHaveBeenCalledTimes(2);
    expect(service.getHistoryArticles.mock.calls[1]![0]).toMatchObject({ keyword: "Go" });
    expect(wrapper.get(".article-mini-card__title").text()).toBe("Go 并发模型");
  });

  it("清空关键词后搜索回到全部历史（keyword 不下传）", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: false });

    const wrapper = mountPage();
    await flushPromises();

    await wrapper.get(".history-search input").setValue("Go");
    await buttonByText(wrapper, "搜索").trigger("click");
    await flushPromises();
    await wrapper.get(".history-search input").setValue("");
    await buttonByText(wrapper, "搜索").trigger("click");
    await flushPromises();

    const lastCall = service.getHistoryArticles.mock.calls.at(-1)![0] as { keyword?: string };
    expect(lastCall.keyword).toBeUndefined();
  });
});

describe("顶部操作：清空历史（二次确认）", () => {
  it("点清空先弹确认框，确认后才真的调接口并落空态", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: false });
    service.clearHistoryArticles.mockResolvedValue({});

    const wrapper = mountPage();
    await flushPromises();

    await buttonByText(wrapper, "清空历史").trigger("click");
    expect(service.clearHistoryArticles).not.toHaveBeenCalled();
    expect(dialog.warning).toHaveBeenCalledTimes(1);

    const options = dialog.warning.mock.calls[0]![0] as {
      content: string;
      onPositiveClick: () => Promise<void>;
    };
    expect(options.content).toContain("清空");
    expect(options.content).toContain("不可恢复");

    await options.onPositiveClick();
    await flushPromises();

    expect(service.clearHistoryArticles).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(0);
    expect(wrapper.get(".history-empty").text()).toContain("还没有浏览历史");
    expect(message.success).toHaveBeenCalledWith("已清空浏览历史");
  });
});

describe("单条删除：标题行右侧垃圾桶", () => {
  it("点击垃圾桶只删这一条，且按钮在标题行内", async () => {
    service.getHistoryArticles.mockResolvedValue({
      list: [historyItem({ article_id: "a1" }), historyItem({ article_id: "a2", title: "第二篇" })],
      has_more: false,
    });
    service.deleteHistoryArticles.mockResolvedValue({});

    const wrapper = mountPage();
    await flushPromises();

    const firstCard = wrapper.get(".article-mini-card");
    const remove = firstCard.get(".article-mini-card__remove");
    expect(remove.attributes("aria-label")).toBe("移出浏览历史");
    expect(firstCard.get(".article-mini-card__title-row").exists()).toBe(true);

    await remove.trigger("click");
    await flushPromises();

    expect(service.deleteHistoryArticles).toHaveBeenCalledWith(["a1"]);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
    expect(wrapper.get(".article-mini-card__title").text()).toBe("第二篇");
    expect(message.success).toHaveBeenCalledWith("已移出浏览历史");
  });
});

describe("批量管理：多选态", () => {
  it("进入后每张卡片出现勾选框，显示已选 N 条，删除所选并退出", async () => {
    service.getHistoryArticles.mockResolvedValue({
      list: [
        historyItem({ article_id: "a1" }),
        historyItem({ article_id: "a2", title: "第二篇" }),
        historyItem({ article_id: "a3", title: "第三篇" }),
      ],
      has_more: false,
    });
    service.deleteHistoryArticles.mockResolvedValue({});

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.findAll(".article-mini-card__select")).toHaveLength(0);
    await buttonByText(wrapper, "批量管理").trigger("click");

    const boxes = wrapper.findAll(".article-mini-card__select input");
    expect(boxes).toHaveLength(3);
    expect(wrapper.get(".history-selected").text()).toContain("已选 0 条");

    await boxes[0]!.setValue(true);
    await boxes[1]!.setValue(true);
    expect(wrapper.get(".history-selected").text()).toContain("已选 2 条");

    await buttonByText(wrapper, "删除").trigger("click");
    await flushPromises();

    expect(service.deleteHistoryArticles).toHaveBeenCalledWith(["a1", "a2"]);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
    expect(wrapper.find(".article-mini-card__select").exists()).toBe(false);
    expect(wrapper.find(".history-selected").exists()).toBe(false);
  });

  it("取消退出多选态，不删任何东西", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: false });

    const wrapper = mountPage();
    await flushPromises();

    await buttonByText(wrapper, "批量管理").trigger("click");
    await wrapper.get(".article-mini-card__select input").setValue(true);
    await buttonByText(wrapper, "取消").trigger("click");
    await flushPromises();

    expect(service.deleteHistoryArticles).not.toHaveBeenCalled();
    expect(wrapper.find(".article-mini-card__select").exists()).toBe(false);
    expect(wrapper.find(".history-selected").exists()).toBe(false);
  });

  it("多选态下卡片正文不再是指向文章页的链接（避免勾选时误触跳转）", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [historyItem()], has_more: false });

    const wrapper = mountPage();
    await flushPromises();
    await buttonByText(wrapper, "批量管理").trigger("click");

    expect(wrapper.find(".article-mini-card__title").attributes("href")).toBeUndefined();
    expect(wrapper.find(".article-mini-card__remove").exists()).toBe(false);
  });
});

describe("游标分页：加载更多", () => {
  it("用上一条的 updated_at + article_id 续页，追加后同一天合并进同一分组", async () => {
    const yesterdayAt = daysAgoAt(1, 21);
    service.getHistoryArticles
      .mockResolvedValueOnce({
        list: [
          historyItem({ article_id: "a1", title: "今天甲", updated_at: todayAt(9) }),
          historyItem({ article_id: "a2", title: "昨天乙", updated_at: yesterdayAt }),
        ],
        has_more: true,
      })
      .mockResolvedValueOnce({
        list: [historyItem({ article_id: "a3", title: "今天丙", updated_at: todayAt(8) })],
        has_more: false,
      });

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.find(".history-more").exists()).toBe(true);
    await buttonByText(wrapper, "加载更多").trigger("click");
    await flushPromises();

    const secondCall = service.getHistoryArticles.mock.calls[1]![0] as {
      beforeUpdatedAt: string;
      beforeId: string;
    };
    expect(secondCall.beforeUpdatedAt).toBe(yesterdayAt);
    expect(secondCall.beforeId).toBe("a2");

    expect(wrapper.findAll(".article-mini-card")).toHaveLength(3);
    const todayGroup = wrapper.findAll(".history-group")[0]!;
    expect(todayGroup.get(".history-group__label").text()).toBe("今天");
    expect(todayGroup.findAll(".article-mini-card")).toHaveLength(2);
    expect(wrapper.find(".history-more").exists()).toBe(false);
    expect(wrapper.get(".history-end").text()).toContain("没有更多了");
  });
});

describe("三态：加载中 / 空 / 失败", () => {
  it("请求未落定时显示加载态，落定后换成卡片", async () => {
    let resolveFirst: (value: unknown) => void = () => undefined;
    service.getHistoryArticles.mockReturnValue(
      new Promise((resolve) => {
        resolveFirst = resolve;
      }),
    );

    const wrapper = mountPage();
    await nextTick();

    expect(wrapper.get(".history-loading").get(".skeleton").exists()).toBe(true);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(0);

    resolveFirst({ list: [historyItem()], has_more: false });
    await flushPromises();

    expect(wrapper.find(".history-loading").exists()).toBe(false);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
  });

  it("列表为空且无搜索词时是空态，而不是失败态", async () => {
    service.getHistoryArticles.mockResolvedValue({ list: [], has_more: false });

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.get(".history-empty").text()).toContain("还没有浏览历史");
    expect(wrapper.find(".history-error").exists()).toBe(false);
  });

  it("接口失败有独立失败态与重试入口，重试成功后消失", async () => {
    service.getHistoryArticles.mockRejectedValueOnce(new Error("网络错误")).mockResolvedValueOnce({
      list: [historyItem()],
      has_more: false,
    });

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.get(".history-error").text()).toContain("浏览历史暂时打不开");
    expect(wrapper.find(".history-empty").exists()).toBe(false);

    await buttonByText(wrapper, "重新加载").trigger("click");
    await flushPromises();

    expect(service.getHistoryArticles).toHaveBeenCalledTimes(2);
    expect(wrapper.find(".history-error").exists()).toBe(false);
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
  });
});
