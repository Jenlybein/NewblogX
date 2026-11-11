import { mount } from "@vue/test-utils";
import { Suspense, defineComponent, h, ref, type Component } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import FavoritesPage from "~/pages/studio/favorites.vue";

/**
 * 个人中心收藏夹（票 345 改版后的口径）。
 *
 * 版式从"左栏分组 + 右栏内容"换成"收藏夹网格 + 大弹窗"，所以这层用例钉的是
 * 新交互，而不是旧的三栏结构：
 *   1. 一进来是收藏夹网格：名称、篇数、简介都在卡上；
 *   2. 点卡片才打开内容弹窗（并按该分组取数）；
 *   3. 新建收藏夹复用同一个资料弹窗，**必须填名称**，提交的是用户输入而不是占位文案；
 *   4. 卡片 hover 的删除走二次确认，只删分组；
 *   5. 弹窗里的文章卡是共享的多列卡，hover 的「移出收藏夹」真的调服务。
 *
 * 替身：Nuxt 宏与 useAsyncData 桩掉；naive-ui 换成极小实现（NInput 要能双向）；
 * service 换成记录器。
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

  return {
    NButton: defineComponent({
      name: "NButton",
      props: {
        disabled: { type: Boolean, default: false },
        quaternary: { type: Boolean, default: false },
        tertiary: { type: Boolean, default: false },
        loading: { type: Boolean, default: false },
        type: { type: String, default: undefined },
        size: { type: String, default: undefined },
      },
      setup(props, { slots }) {
        return () => h("button", { disabled: props.disabled || undefined }, slots.default?.());
      },
    }),
    // NInput 必须真的双向：新建收藏夹的用例要输入名称后断言提交值。
    NInput: defineComponent({
      name: "NInput",
      props: { value: { type: String, default: "" }, placeholder: { type: String, default: "" } },
      emits: ["update:value"],
      setup(props, { emit }) {
        return () =>
          h("input", {
            value: props.value,
            placeholder: props.placeholder,
            onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
          });
      },
    }),
    // NModal 用 v-show 语义：内容始终在树里，用 data-show 表达开关状态。
    NModal: defineComponent({
      name: "NModal",
      props: { show: { type: Boolean, default: false } },
      setup(props, { slots }) {
        return () =>
          h("div", { "data-show": String(props.show) }, [slots.header?.(), slots.default?.(), slots.footer?.()]);
      },
    }),
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

/** useAsyncData 替身：只跑一次（改版后页面不再靠 watch 切分组取数）。 */
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
      name: "FavoritesPageUnderSuspense",
      setup() {
        return () => h(Suspense, null, { default: () => h(FavoritesPage as Component) });
      },
    }),
    {
      global: {
        stubs: {
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
    list: [
      { id: "f1", title: "默认收藏", abstract: "稍后阅读", article_count: 2 },
      { id: "f2", title: "Vue3 精读", abstract: "组合式 API", article_count: 0 },
    ],
    count: 2,
  };
}

function contentItems() {
  return {
    list: [
      {
        article_id: "a1",
        title: "文章一",
        cover: "",
        updated_at: "2026-11-11T08:00:00+08:00",
        favorited_at: "2026-11-11T09:00:00+08:00",
        view_count: 12,
        digg_count: 3,
        comment_count: 1,
        favor_count: 2,
      },
    ],
    has_more: false,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  favoriteService.getOwnFavoriteFolders.mockResolvedValue(folderItems());
  favoriteService.getFavoriteFolderArticles.mockResolvedValue(contentItems());
  favoriteService.removeFavoriteFolderArticles.mockResolvedValue({});
  favoriteService.createFavoriteFolder.mockResolvedValue(undefined);
  favoriteService.deleteFavoriteFolders.mockResolvedValue(undefined);
  favoriteService.updateFavoriteFolder.mockResolvedValue(undefined);

  vi.stubGlobal("useAsyncData", useAsyncDataStub);
  vi.stubGlobal("useSeoMeta", () => {});
  vi.stubGlobal("useNoIndexPage", () => {});
  vi.stubGlobal("definePageMeta", () => {});
  vi.stubGlobal("useRoute", () => ({ path: "/studio/favorites" }));
});

describe("收藏夹网格", () => {
  it("一进来就是收藏夹网格：名称、篇数、简介都在卡上", async () => {
    const wrapper = mountPage();
    await settle();

    const cards = wrapper.findAll(".studio-folder-card");
    expect(cards).toHaveLength(2);
    expect(wrapper.get('[data-role="folder-card-f1"]').text()).toContain("默认收藏");
    expect(wrapper.get('[data-role="folder-card-f1"]').text()).toContain("2 篇");
    expect(wrapper.get('[data-role="folder-card-f1"]').text()).toContain("稍后阅读");
  });

  it("点卡片才按该分组取数，并把文章渲染成多列卡", async () => {
    const wrapper = mountPage();
    await settle();

    // 没点之前不取内容。
    expect(favoriteService.getFavoriteFolderArticles).not.toHaveBeenCalled();

    await wrapper.get('[data-role="folder-card-f1"] .studio-folder-card__open').trigger("click");
    await settle();

    expect(favoriteService.getFavoriteFolderArticles).toHaveBeenCalledWith(
      expect.objectContaining({ favoriteId: "f1", auth: true }),
    );
    const cards = wrapper.findAll(".article-grid-card");
    expect(cards).toHaveLength(1);
    expect(cards[0]!.text()).toContain("文章一");
    // 日期只写日期，不带"发布于"前缀。
    expect(cards[0]!.text()).toContain("2026/11/11");
    expect(cards[0]!.text()).not.toContain("发布于");
  });

  it("弹窗里的「移出收藏夹」调服务并重拉内容与分组", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="folder-card-f1"] .studio-folder-card__open').trigger("click");
    await settle();

    await wrapper.get(".article-grid-card__action").trigger("click");
    await settle();

    expect(favoriteService.removeFavoriteFolderArticles).toHaveBeenCalledWith({
      favoriteId: "f1",
      articleIds: ["a1"],
    });
    expect(favoriteService.getOwnFavoriteFolders).toHaveBeenCalledTimes(2);
  });
});

describe("新建收藏夹", () => {
  it("复用资料弹窗：没有标题条，提交的是用户输入而不是占位文案", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="folder-create"]').trigger("click");
    await settle();

    // 弹窗里不该出现"修改收藏夹资料"这类标题条。
    expect(wrapper.text()).not.toContain("修改收藏夹资料");

    await wrapper.get('[data-role="folder-editor-title"]').setValue("我的新夹子");
    await wrapper.get('[data-role="folder-editor-abstract"]').setValue("收一些 Vue 文章");
    await wrapper.get('[data-role="folder-editor-submit"]').trigger("click");
    await settle();

    expect(favoriteService.createFavoriteFolder).toHaveBeenCalledWith({
      title: "我的新夹子",
      abstract: "收一些 Vue 文章",
    });
  });

  it("名称为空时「确定」不可点，也不会发请求", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="folder-create"]').trigger("click");
    await settle();

    const submit = wrapper.get('[data-role="folder-editor-submit"]');
    expect(submit.attributes("disabled")).toBeDefined();
    await submit.trigger("click");
    await settle();

    expect(favoriteService.createFavoriteFolder).not.toHaveBeenCalled();
  });
});

describe("删除收藏夹", () => {
  it("卡片上的删除走二次确认，确认后只删分组", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="folder-delete"]').trigger("click");
    await settle();

    // 二次确认里要说清"文章本身不会被删除"。
    expect(wrapper.text()).toContain("文章本身不会被删除");

    await wrapper.get('[data-role="folder-delete-confirm"]').trigger("click");
    await settle();

    expect(favoriteService.deleteFavoriteFolders).toHaveBeenCalledWith(["f1"]);
  });
});

describe("空态", () => {
  it("没有收藏夹时给一句能照着做的话", async () => {
    favoriteService.getOwnFavoriteFolders.mockResolvedValue({ list: [], count: 0 });

    const wrapper = mountPage();
    await settle();

    expect(wrapper.get(".studio-empty-stub").text()).toContain("还没有收藏夹");
  });
});
