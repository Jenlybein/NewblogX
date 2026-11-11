import { mount } from "@vue/test-utils";
import { Suspense, defineComponent, h, ref, type Component } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CategoriesPage from "~/pages/studio/categories.vue";

/**
 * 个人中心分类管理（票 351 改版后的口径）。
 *
 * 改版要解决的是"分类也是内容容器"：一进来是分类网格（名称 + 篇数），
 * 点开一个分类在弹窗里看该分类下的**全部**文章 —— 包括草稿，每篇带状态标签。
 * 只列已发布就管不了未发布的稿子，那正是作者打开这页的目的。
 */

const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const dialog = vi.hoisted(() => ({ warning: vi.fn(), info: vi.fn(), error: vi.fn(), success: vi.fn() }));
const categoryService = vi.hoisted(() => ({ getProfileCategories: vi.fn() }));
const searchService = vi.hoisted(() => ({ searchArticles: vi.fn() }));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");
  const passthrough = (name: string, tag = "div") =>
    defineComponent({
      name,
      setup(_props, { slots }) {
        return () => h(tag, [slots.header?.(), slots.default?.(), slots.footer?.()]);
      },
    });
  return {
    NButton: defineComponent({
      name: "NButton",
      props: { disabled: { type: Boolean, default: false }, loading: { type: Boolean, default: false } },
      setup(props, { slots }) {
        return () => h("button", { disabled: props.disabled || undefined }, slots.default?.());
      },
    }),
    NModal: defineComponent({
      name: "NModal",
      props: { show: { type: Boolean, default: false } },
      setup(props, { slots }) {
        return () => h("div", { "data-show": String(props.show) }, [slots.header?.(), slots.default?.(), slots.footer?.()]);
      },
    }),
    NSkeleton: defineComponent({ name: "NSkeleton", setup: () => () => h("div", { class: "n-skeleton" }) }),
    useMessage: () => message,
    useDialog: () => dialog,
  };
});

vi.mock("~/services/category", () => categoryService);
vi.mock("~/services/search", () => searchService);

function useAsyncDataStub(...args: unknown[]) {
  let handler: (() => unknown) | undefined;
  for (const arg of args) {
    if (typeof arg === "function") handler = arg as () => unknown;
  }
  const data = ref<unknown>(null);
  const pending = ref(true);
  const run = () =>
    Promise.resolve()
      .then(() => (handler ? handler() : null))
      .then((value) => {
        data.value = value;
      })
      .finally(() => {
        pending.value = false;
      });
  const initial = run();
  return { data, pending, status: pending, refresh: run, execute: run, asyncData: initial };
}

function mountPage() {
  return mount(
    defineComponent({
      name: "CategoriesPageUnderSuspense",
      setup() {
        return () => h(Suspense, null, { default: () => h(CategoriesPage as Component) });
      },
    }),
    {
      global: {
        stubs: {
          ProfileEmptyState: defineComponent({
            name: "ProfileEmptyState",
            props: { title: { type: String, default: "" }, description: { type: String, default: "" } },
            setup(props) {
              return () => h("div", { class: "empty-state-stub", "data-empty-state": "true" }, [props.title, props.description]);
            },
          }),
          ProfileCategoryFormModal: defineComponent({
            name: "ProfileCategoryFormModal",
            props: { show: { type: Boolean, default: false }, mode: { type: String, default: "create" } },
            setup() {
              return () => h("div", { class: "form-modal-stub" });
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

function categoriesPayload() {
  return {
    list: [
      { id: "c1", title: "工程笔记", article_count: 3 },
      { id: "c2", title: "随笔", article_count: 0 },
    ],
    count: 2,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  categoryService.getProfileCategories.mockResolvedValue(categoriesPayload());
  searchService.searchArticles.mockResolvedValue({
    list: [
      { id: "a1", title: "已发布的稿子", publish_status: 3, cover: "", created_at: "2026-11-01T10:00:00+08:00" },
      { id: "a2", title: "还没发的草稿", publish_status: 1, cover: "", created_at: "2026-11-02T10:00:00+08:00" },
    ],
    pagination: { page: 1, total_pages: 1 },
    count: 2,
  });

  vi.stubGlobal("useAsyncData", useAsyncDataStub);
  vi.stubGlobal("useSeoMeta", () => {});
  vi.stubGlobal("useNoIndexPage", () => {});
  vi.stubGlobal("definePageMeta", () => {});
  vi.stubGlobal("useAuthStore", () => ({ profileId: "900" }));
});

describe("分类网格", () => {
  it("分类以卡片网格呈现：名称与篇数都在卡上", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.find('[data-role="category-grid"]').exists()).toBe(true);
    const card = wrapper.get('[data-role="category-row-c1"]');
    expect(card.text()).toContain("工程笔记");
    expect(wrapper.get('[data-role="category-count-c1"]').text()).toContain("3 篇");
  });

  it("每张卡保留改名与删除入口（写入口仍在这一页）", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.find('[data-role="category-rename-c1"]').exists()).toBe(true);
    expect(wrapper.find('[data-role="category-delete-c1"]').exists()).toBe(true);
  });
});

describe("分类弹窗：含草稿", () => {
  it("点卡片按该分类取数，且不带 status 过滤（草稿也要列出来）", async () => {
    const wrapper = mountPage();
    await settle();

    expect(searchService.searchArticles).not.toHaveBeenCalled();

    await wrapper.get('[data-role="category-row-c1"] .folder-card__open').trigger("click");
    await settle();

    const params = searchService.searchArticles.mock.calls[0]![0] as Record<string, unknown>;
    expect(params.category_id).toBe("c1");
    // 关键：没有 status 过滤，否则草稿会被后端筛掉。
    expect(params.status).toBeUndefined();
    expect(params.auth).toBe(true);
  });

  it("草稿带状态标签，已发布的同样标出，读者能分辨", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-row-c1"] .folder-card__open').trigger("click");
    await settle();

    const draft = wrapper.get('[data-role="category-article-status-a2"]');
    expect(draft.text()).toBe("草稿");
    expect(draft.attributes("data-status")).toBe("1");

    expect(wrapper.get('[data-role="category-article-status-a1"]').text()).toBe("已发布");
  });

  it("文章卡是多列卡（不是单列条目）", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-row-c1"] .folder-card__open').trigger("click");
    await settle();

    const area = wrapper.get('[data-role="category-articles"]');
    expect(area.findAll(".article-grid-card")).toHaveLength(2);
  });

  it("读不到时说明原因，不伪装成「这个分类没有文章」", async () => {
    searchService.searchArticles.mockRejectedValue(new Error("网络断了"));

    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-row-c1"] .folder-card__open').trigger("click");
    await settle();

    const empty = wrapper.get('[data-role="category-articles"] .empty-state-stub');
    expect(empty.text()).toContain("打不开");
    expect(empty.text()).toContain("网络断了");
  });
});
