import { mount } from "@vue/test-utils";
import { Suspense, defineComponent, h, isRef, reactive, ref, watch, type Component, type Ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import FavoritesPage from "~/pages/studio/favorites.vue";

/**
 * 个人中心收藏夹（#327）。
 *
 * 用户报的 bug：分组内容区用一套横向三栏卡片（176px 封面 / 正文 / 操作列），
 * 在个人中心这半边的窄容器里，封面与正文会挤到一起——同一类内容（"被收进某个
 * 私人列表的文章"）在站内因此有两种观感。修法是"复用优先"：改用浏览历史那张
 * 共享短卡片 `ArticleMiniCard`（竖向：封面 + 两行标题 + 一行元信息 + 行内移出）。
 *
 * 所以这层用例要真的跨过 页面 → 共享卡片 → 服务 这条线：
 *   1. 渲染的是共享卡片的真实结构（`.article-mini-card`），旧的三栏结构已不在；
 *   2. 整张卡（封面 + 标题）都指向详情页，不再单独排"查看原文"；
 *   3. 点行内「移出」真的调 `removeFavoriteFolderArticles`（带收藏夹与文章 ID），
 *      并按服务端为准重拉内容与分组计数；
 *   4. 元信息那一行是"作者 · 阅读数 · 收藏于 …"（两页共用同一个位置）。
 *
 * 替身与既有页面用例同理：Nuxt 编译期宏与自动导入的 useAsyncData 只能桩掉；
 * naive-ui 换成极小实现；service 换成记录器。
 */

const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const favoriteService = vi.hoisted(() => ({
  getOwnFavoriteFolders: vi.fn(),
  getFavoriteFolderArticles: vi.fn(),
  removeFavoriteFolderArticles: vi.fn(),
  createFavoriteFolder: vi.fn(),
  deleteFavoriteFolders: vi.fn(),
  updateFavoriteFolder: vi.fn(),
}));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  const passthrough = (name: string, tag = "div") =>
    defineComponent({
      name,
      setup(_props, { slots }) {
        return () => h(tag, slots.default?.());
      },
    });

  return {
    NButton: defineComponent({
      name: "NButton",
      props: { disabled: { type: Boolean, default: false }, quaternary: { type: Boolean, default: false }, type: { type: String, default: undefined } },
      setup(props, { slots }) {
        return () => h("button", { disabled: props.disabled || undefined }, slots.default?.());
      },
    }),
    NInput: defineComponent({
      name: "NInput",
      props: { value: { type: String, default: "" } },
      setup(_props, { slots }) {
        return () => h("input", slots.default?.());
      },
    }),
    NModal: passthrough("NModal"),
    // 骨架单独给 class，便于断言"加载态与加载后用的是同一个栅格"。
    NSkeleton: defineComponent({
      name: "NSkeleton",
      setup() {
        return () => h("div", { class: "n-skeleton" });
      },
    }),
    useMessage: () => message,
  };
});

vi.mock("~/services/favorite", () => favoriteService);

/**
 * useAsyncData 替身：会**按 watch 源重跑取数函数**。
 *
 * 共享替身的 refresh 只是再 await 同一个 promise（不重取）、也不跟 watch 源联动，
 * 于是"移出后重拉内容与分组""切换分组按新分组取数"这两条在页面用例里观察不到，
 * 而它们正是这一票要钉的接线。
 */
function useAsyncDataWatching(...args: unknown[]) {
  let handler: (() => unknown) | undefined;
  let watchSources: unknown;

  for (const arg of args) {
    if (typeof arg === "function") {
      handler = arg as () => unknown;
      continue;
    }
    if (arg && typeof arg === "object" && "watch" in (arg as Record<string, unknown>)) {
      watchSources = (arg as { watch?: unknown }).watch;
    }
  }

  const data = ref<unknown>(null);
  const error = ref<unknown>(null);
  const pending = ref(true);

  /**
   * 代数计数：只有"最新一次取数"落定时才允许把 pending 置回 false。
   *
   * 没有它就会复现一类假失败：切换分组会立刻发起第二次取数（pending=true），
   * 而第一次（还在路上的）随后结束并把自己的 finally 写进 pending=false，
   * 于是真实页面里"第二次仍在加载"的状态在用例里被抹掉。
   */
  let generation = 0;

  const run = () => {
    const mine = (generation += 1);
    pending.value = true;
    return Promise.resolve()
      .then(() => (handler ? handler() : null))
      .then((value) => {
        if (mine !== generation) return;
        data.value = value;
      })
      .catch((reason) => {
        if (mine !== generation) return;
        error.value = reason;
      })
      .finally(() => {
        if (mine !== generation) return;
        pending.value = false;
      });
  };

  const initial = run();

  const sources = Array.isArray(watchSources) ? watchSources : watchSources ? [watchSources] : [];
  for (const source of sources) {
    if (isRef(source)) {
      watch(source as Ref, () => void run(), { flush: "sync" });
    }
  }

  return { data, error, pending, status: pending, refresh: run, execute: run, asyncData: initial };
}

function mountPage() {
  return mount(
    defineComponent({
      name: "FavoritesPageUnderSuspense",
      setup() {
        return () => h(Suspense, null, { default: () => h(FavoritesPage as Component) });
      },
    }),
    {
      global: {
        stubs: {
          NuxtLink: defineComponent({
            props: { to: { type: [String, Object], default: "" } },
            setup(props, { slots }) {
              return () =>
                h(
                  "a",
                  {
                    "data-href": typeof props.to === "string" ? props.to : JSON.stringify(props.to),
                  },
                  slots.default?.(),
                );
            },
          }),
          ArticleCover: defineComponent({
            name: "ArticleCover",
            props: { src: { type: String, default: "" }, title: { type: String, default: "" } },
            setup() {
              return () => h("div", { class: "article-cover-stub" });
            },
          }),
          StudioEmptyState: defineComponent({
            name: "StudioEmptyState",
            props: { title: { type: String, default: "" }, description: { type: String, default: "" } },
            setup(props) {
              return () => h("div", { class: "studio-empty-stub" }, [props.title, props.description]);
            },
          }),
        },
      },
    },
  );
}

async function settle() {
  for (let i = 0; i < 6; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

function folderItems() {
  return {
    list: [{ id: "f1", title: "默认收藏", abstract: "暂未填写简介", article_count: 1 }],
    count: 1,
  };
}

function contentItems() {
  return {
    list: [
      {
        article_id: "a1",
        title: "响应式系统与组合式 API",
        cover: "https://cdn.example.com/a1.png",
        abstract: "一篇讲 Vue 3 响应式原理的文章。",
        user_nickname: "admin2",
        view_count: 5,
        favorited_at: "2026-11-09T23:35:00Z",
      },
    ],
    has_more: false,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useNoIndexPage", () => undefined);
  vi.stubGlobal("shallowRef", ref);
  // 页面里的 `reactive(...)` 是 Nuxt 自动导入，单测环境没有编译期注入。
  vi.stubGlobal("reactive", reactive);
  vi.stubGlobal("useAsyncData", useAsyncDataWatching);

  favoriteService.getOwnFavoriteFolders.mockResolvedValue(folderItems());
  favoriteService.getFavoriteFolderArticles.mockResolvedValue(contentItems());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("收藏夹分组内容：与浏览历史同一个卡片形状", () => {
  it("渲染共享短卡片，旧的三栏结构已不在", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
    expect(wrapper.find(".favorite-content-card").exists()).toBe(false);
    expect(wrapper.find(".favorite-content-card__cover-wrap").exists()).toBe(false);

    // 共享卡片的两行标题 + 一行元信息。
    expect(wrapper.get(".article-mini-card__title").text()).toBe("响应式系统与组合式 API");
    const meta = wrapper.get(".article-mini-card__meta").text();
    expect(meta).toContain("admin2");
    expect(meta).toContain("5 阅读");
    expect(meta).toContain("收藏于");
  });

  it("整张卡都指向详情页，不再单独排一个「查看原文」按钮", async () => {
    const wrapper = mountPage();
    await settle();

    const card = wrapper.get(".article-mini-card");
    // 封面与标题各自都是一条指向详情页的链接（NuxtLink 在用例里替身成 <a>）。
    expect(card.get(".article-mini-card__cover").attributes("data-href")).toBe("/article/a1");
    expect(card.get(".article-mini-card__title").attributes("data-href")).toBe("/article/a1");
    expect(wrapper.text()).not.toContain("查看原文");
  });

  it("点行内「移出」调服务（带分组与文章 ID），并按服务端为准重拉内容", async () => {
    const wrapper = mountPage();
    await settle();

    favoriteService.removeFavoriteFolderArticles.mockResolvedValue({});

    await wrapper.get(".article-mini-card__remove").trigger("click");
    await settle();

    expect(favoriteService.removeFavoriteFolderArticles).toHaveBeenCalledWith({
      favoriteId: "f1",
      articleIds: ["a1"],
    });
    expect(message.success).toHaveBeenCalledWith("文章已移出收藏夹");
    // 服务端为准：内容与分组计数都要重拉（计数在左栏的徽标上）。
    expect(favoriteService.getFavoriteFolderArticles).toHaveBeenCalledTimes(2);
    expect(favoriteService.getOwnFavoriteFolders).toHaveBeenCalledTimes(2);
  });

  it("移出失败时给出可读提示，且不改动列表", async () => {
    const wrapper = mountPage();
    await settle();

    favoriteService.removeFavoriteFolderArticles.mockRejectedValue(new Error("网络中断"));

    await wrapper.get(".article-mini-card__remove").trigger("click");
    await settle();

    expect(message.error).toHaveBeenCalledWith(expect.stringContaining("网络中断"));
    expect(wrapper.findAll(".article-mini-card")).toHaveLength(1);
  });

  it("加载态用同一个栅格铺骨架，不会出现第二种版式", async () => {
    favoriteService.getFavoriteFolderArticles.mockReturnValue(new Promise(() => {}));

    const wrapper = mountPage();
    await settle();

    expect(wrapper.findAll(".favorite-content-skeleton")).toHaveLength(4);
    expect(wrapper.find(".favorite-content-skeleton__cover").exists()).toBe(true);
  });

  it("空分组给空态，而不是一片空白", async () => {
    favoriteService.getFavoriteFolderArticles.mockResolvedValue({ list: [], has_more: false });

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find(".studio-empty-stub").exists()).toBe(true);
    expect(wrapper.text()).toContain("这个收藏夹还是空的");
    expect(wrapper.find(".article-mini-card").exists()).toBe(false);
  });
});
