import { mount } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import UserProfilePage from "~/pages/users/[id].vue";
import { getUserBaseInfo } from "~/services/user";

/**
 * 公开主页的分类栏是**只读**的（票 327 口径）。
 *
 * 历史：分类的写入口（新建 / 改名 / 删除）曾在 `/users/[id]` 上按 `isSelf` 出现（票 325）。
 * 用户纠正后，管理只在个人中心 `/studio/categories`，公开主页对**任何身份**（包括作者本人）
 * 都只展示：分类列表、文章数、点分类就地展开该分类下的文章、空态与错误态。
 *
 * 这条用例以前是"分类栏写入口：只有自己的主页才有"（test/pages/UserProfileCategoryManage.test.ts）。
 * 现在反过来钉：作者本人看自己的主页时，一个写入口都不能有；同时证明展示链路没被顺手删掉
 * ——点分类仍会去取该分类下的文章并渲染出来。
 *
 * 页面 SFC 的替身与其他页面用例同理：
 *   1. Nuxt 自动导入的标识符（useRoute / useRouter / useAsyncData / useSeoMeta /
 *      useUiStore / useAuthStore / useArticleSearch）在单测里没有编译器注入，用 stubGlobal 补；
 *   2. service 换成记录器，只观测页面怎么用它们；
 *   3. 只把 `ProfileCategoryExplorer` 与文章条目留成真/可观测的组件，其余展示组件打桩。
 *      **不 stub 任何写入口**：写入口在页面源码里已经不存在了，stub 掉反而会把
 *      "又加回来了"这种回归藏起来。
 */

const authStore = vi.hoisted(() => ({ profileId: "100" as string | null, isLoggedIn: true, initializeSession: vi.fn(async () => true) }));
const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const routerReplace = vi.hoisted(() => vi.fn());

const categoryService = vi.hoisted(() => ({ getProfileCategories: vi.fn() }));
const searchService = vi.hoisted(() => ({ searchArticles: vi.fn() }));

vi.mock("~/services/category", () => categoryService);
vi.mock("~/services/search", () => searchService);
vi.mock("~/services/user", () => ({
  getUserBaseInfo: vi.fn(async () => ({
    id: "100",
    code_age: 1,
    avatar: "",
    nickname: "作者甲",
    abstract: "",
    view_count: 0,
    article_visited_count: 0,
    article_count: 0,
    fans_count: 0,
    follow_count: 0,
    favor_count: 0,
    digg_count: 0,
    comment_count: 0,
    favorites_visibility: true,
    followers_visibility: true,
    fans_visibility: true,
    home_style_id: "",
    relation: 0,
    place: "",
  })),
  getSelfUserDetail: vi.fn(async () => ({ abstract: "" })),
}));
vi.mock("~/services/favorite", () => ({
  getPublicFavoriteFolders: vi.fn(async () => ({ list: [], count: 0 })),
  getOwnFavoriteFolders: vi.fn(async () => ({ list: [], count: 0 })),
  getFavoriteFolderArticles: vi.fn(async () => ({ list: [], count: 0 })),
}));
vi.mock("~/services/follow", () => ({
  // 票 327 起关注 / 粉丝共用一个读端点（type 区分视图），条目形状也统一了。
  getRelations: vi.fn(async () => ({ list: [], has_more: false })),
  getRelationCounts: vi.fn(async () => ({ follow: 0, fans: 0, mutual: 0 })),
  followUser: vi.fn(),
  unfollowUser: vi.fn(),
  removeFan: vi.fn(),
}));
vi.mock("~/composables/useDraftChatJump", () => ({ useDraftChatJump: () => ({ jumpToDraftChat: vi.fn() }) }));
vi.mock("naive-ui", () => ({ useMessage: () => message }));

const ProfileMediaStub = defineComponent({
  props: {
    title: { type: String, default: "" },
    description: { type: String, default: "" },
    label: { type: String, default: "" },
  },
  setup(props, { slots }) {
    return () => h("div", { class: "media-stub" }, [props.title, props.description, props.label, slots.default?.()]);
  },
});

/** 文章条目替身：只需要把"哪一篇被渲染出来"暴露给断言。 */
const ArticleFeedItemStub = defineComponent({
  props: { article: { type: Object, default: () => ({}) } },
  setup(props) {
    return () => h("div", { class: "article-stub" }, (props.article as { title?: string }).title ?? "");
  },
});

function mountPage() {
  return mount(UserProfilePage, {
    global: {
      stubs: {
        NuxtLink: defineComponent({
          props: { to: { type: [String, Object], default: "" } },
          setup(_props, { slots }) {
            return () => h("a", slots.default?.());
          },
        }),
        LoadingSkeleton: ProfileMediaStub,
        ProfileHeroCard: ProfileMediaStub,
        ProfileCollectionShelf: ProfileMediaStub,
        ProfileRelationList: ProfileMediaStub,
        ProfileEmptyState: ProfileMediaStub,
        ArticleFeedItem: ArticleFeedItemStub,
      },
    },
  });
}

/** useAsyncData 的 mock 在微任务链里落数据，等到分类列表真的渲染出来。 */
async function settle() {
  for (let i = 0; i < 6; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

function useAsyncDataRefetching(...args: unknown[]) {
  let handler: (() => unknown) | undefined;
  for (const arg of args) {
    if (typeof arg === "function") handler = arg as () => unknown;
  }

  const data = ref<unknown>(null);
  const error = ref<unknown>(null);
  const pending = ref(true);

  const run = () =>
    Promise.resolve()
      .then(() => (handler ? handler() : null))
      .then((value) => {
        data.value = value;
      })
      .catch((reason) => {
        error.value = reason;
      })
      .finally(() => {
        pending.value = false;
      });

  void run();

  return { data, error, pending, status: pending, refresh: run, execute: run };
}

beforeEach(() => {
  vi.clearAllMocks();
  authStore.profileId = "100";

  categoryService.getProfileCategories.mockResolvedValue({
    list: [{ id: "c1", title: "工程笔记", article_count: 2 }],
    count: 1,
  });
  searchService.searchArticles.mockResolvedValue({
    list: [
      {
        id: "art-1",
        title: "按分类展开的文章",
        abstract: "摘要。",
        created_at: "2026-10-05T12:30:00Z",
      },
    ],
    count: 1,
  });

  vi.stubGlobal("useRoute", () => ({
    params: { id: "100" },
    query: { tab: "categories" },
    path: "/users/100",
  }));
  vi.stubGlobal("useRouter", () => ({ replace: routerReplace }));
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useUiStore", () => ({ openAuthModal: vi.fn() }));
  vi.stubGlobal("useAsyncData", useAsyncDataRefetching);
  vi.stubGlobal("useAuthStore", () => authStore);
  vi.stubGlobal("useArticleSearch", async () => ({ articles: ref([]), pending: ref(false) }));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("公开主页分类栏：对任何身份都只读", () => {
  it("作者看自己的主页：没有新建 / 改名 / 删除，也没有分类表单弹窗", async () => {
    const wrapper = mountPage();
    await settle();

    // 分类本身照常展示（只读视图没被误删）。
    expect(wrapper.findAll(".profile-category-card")).toHaveLength(1);
    expect(wrapper.text()).toContain("工程笔记");

    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
    expect(wrapper.find('[data-role^="category-rename-"]').exists()).toBe(false);
    expect(wrapper.find('[data-role^="category-delete-"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="category-form-modal"]').exists()).toBe(false);
    // 文案层面再兜一层：管理入口不能以别的写法回来。
    for (const banned of ["新建分类", "改名", "删除分类"]) {
      expect(wrapper.text()).not.toContain(banned);
    }
  });

  it("他人主页：同样没有写入口", async () => {
    authStore.profileId = "999";

    const wrapper = mountPage();
    await settle();

    expect(wrapper.findAll(".profile-category-card")).toHaveLength(1);
    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
    expect(wrapper.findAll("button").map((button) => button.text())).not.toContain("改名");
    expect(wrapper.findAll("button").map((button) => button.text())).not.toContain("删除");
  });

  it("未登录：同样没有写入口", async () => {
    authStore.profileId = null;
    authStore.isLoggedIn = false;

    const wrapper = mountPage();
    await settle();

    expect(wrapper.findAll(".profile-category-card")).toHaveLength(1);
    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
  });
});

describe("公开主页分类栏：展示链路仍然完整", () => {
  it("点一个分类就地展开该分类下的文章（取数参数与渲染都要对）", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get(".profile-category-card").trigger("click");
    await settle();

    // 就地展开：按 author_id + category_id 取公开文章，不跳页。
    expect(searchService.searchArticles).toHaveBeenCalledWith({
      type: 3,
      author_id: "100",
      category_id: "c1",
      page: 1,
      limit: 12,
      page_mode: "count",
      sort: 2,
    });
    // 右栏顶栏已删除；文章卡直接出现，选中态由左侧分类卡承担。
    expect(wrapper.find(".profile-category-heading").exists()).toBe(false);
    expect(wrapper.text()).toContain("按分类展开的文章");
    expect(routerReplace).not.toHaveBeenCalled();
  });

  it("分类读取失败时给错误态，而不是伪装成「没有分类」", async () => {
    categoryService.getProfileCategories.mockRejectedValueOnce(new Error("boom"));

    const wrapper = mountPage();
    await settle();

    // 页面把失败标记传给分类栏；分类栏自己渲染「打不开」的空态（其用例单独覆盖）。
    expect(wrapper.find(".profile-category-layout").exists()).toBe(true);
    expect(categoryService.getProfileCategories).toHaveBeenCalledTimes(1);
  });
});

describe("个人成就：累计互动口径", () => {
  it("渲染累计被赞 / 被收藏 / 累计被评论（只算已发布文章，后端聚合）", async () => {
    vi.mocked(getUserBaseInfo).mockResolvedValueOnce({
      id: "100",
      code_age: 1,
      avatar: "",
      nickname: "作者甲",
      abstract: "",
      view_count: 10,
      article_visited_count: 2000,
      article_count: 5,
      fans_count: 0,
      follow_count: 0,
      favor_count: 56,
      digg_count: 1234,
      comment_count: 7,
      favorites_visibility: true,
      followers_visibility: true,
      fans_visibility: true,
      relation: 0,
      place: "",
    } as never);

    const wrapper = mountPage();
    await settle();

    expect(wrapper.text()).toContain("累计被赞");
    expect(wrapper.text()).toContain("1.2k");
    expect(wrapper.text()).toContain("累计被收藏");
    expect(wrapper.text()).toContain("56");
    expect(wrapper.text()).toContain("累计被评论");
    expect(wrapper.text()).toContain("7");
  });
});