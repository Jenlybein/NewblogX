import { mount } from "@vue/test-utils";
import { defineComponent, h, ref, watch } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useProfileCategoryManage } from "~/composables/useProfileCategoryManage";
import UserProfilePage from "~/pages/users/[id].vue";

/**
 * 页面里用到的 useAsyncData 替身。
 *
 * 与 test/mocks/nuxt-imports.ts 的那份差别只有一处：**refresh 会重新跑取数函数**。
 * 共享替身的 refresh 只是再 await 同一个 promise（不重取），于是"写完重拉列表"
 * 这类行为在页面用例里观察不到——而它正是这一票要钉的东西之一。
 */
function useAsyncDataRefetching(...args: unknown[]) {
  let handler: (() => unknown) | undefined;
  for (const arg of args) {
    if (typeof arg === "function") {
      handler = arg as () => unknown;
    }
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

/**
 * 个人主页的分类栏写入口（票 325）。
 *
 * 页面这层的唯一职责是"是不是我自己"：新建 / 改名 / 删除只在自己的主页出现，
 * 他人主页与未登录一律只读。这条规则不属于分类栏组件（它按插槽渲染、不判断身份），
 * 也不属于写操作 composable（它不认识登录态），只能在这里锁住。
 *
 * 页面 SFC 的替身与其他页面用例同理：
 *   1. Nuxt 自动导入的标识符（useRoute / useRouter / useAsyncData / useSeoMeta /
 *      useUiStore / useProfileCategoryManage）在单测里没有编译器注入，用 stubGlobal 补；
 *   2. service 换成记录器，只观测页面怎么用它们；
 *   3. 子组件里只留分类栏是真的，其余展示组件打桩。
 */

const authStore = vi.hoisted(() => ({ profileId: "100" as string | null, isLoggedIn: true, initializeSession: vi.fn(async () => true) }));
const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const dialog = vi.hoisted(() => ({ warning: vi.fn() }));

const categoryService = vi.hoisted(() => ({
  getProfileCategories: vi.fn(),
  saveProfileCategory: vi.fn(),
  deleteProfileCategories: vi.fn(),
}));

vi.mock("~/services/category", () => categoryService);
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
  getFollowList: vi.fn(async () => ({ list: [], count: 0 })),
  getFansList: vi.fn(async () => ({ list: [], count: 0 })),
  followUser: vi.fn(),
  unfollowUser: vi.fn(),
}));
vi.mock("~/services/search", () => ({ searchArticles: vi.fn(async () => ({ list: [], count: 0 })) }));
vi.mock("~/composables/useDraftChatJump", () => ({ useDraftChatJump: () => ({ jumpToDraftChat: vi.fn() }) }));
vi.mock("naive-ui", () => ({ useMessage: () => message, useDialog: () => dialog }));

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

/**
 * 弹窗替身：既能观察页面接给了哪些参数，也能像真弹窗那样把"用户输入"抛回去。
 *
 * 之前这里只渲染 `data-*` 属性、从不 emit `submit`，于是"页面把 submit 接到了
 * 哪里、接到了什么值"这条线两侧各自绿、中间没测：composable 用例自己调
 * `submit()`（读的是它自己的 formTitle），页面用例只看到一个不会提交的替身，
 * 真实接线断了也没有用例红（P0）。现在替身保留输入、按真弹窗的规则重置、
 * 并在点保存时把当前文本 emit 出去。
 */
const CategoryFormModalStub = defineComponent({
  props: {
    show: { type: Boolean, default: false },
    mode: { type: String, default: "create" },
    initialTitle: { type: String, default: "" },
    pending: { type: Boolean, default: false },
  },
  emits: {
    "update:show": (_value: boolean) => true,
    submit: (_title: string) => true,
  },
  setup(props, { emit }) {
    const text = ref("");

    // 与真弹窗同一条规则：每次打开（或换 initialTitle）按 initialTitle 重置。
    // 提交失败时 show 不变，所以输入留在框里、不会被重置掉。
    watch(
      () => [props.show, props.initialTitle] as const,
      ([show, initial]) => {
        if (show) text.value = initial;
      },
      { immediate: true },
    );

    return () =>
      h(
        "div",
        {
          "data-role": "category-form-modal",
          "data-show": String(props.show),
          "data-mode": props.mode,
          "data-initial": props.initialTitle,
        },
        [
          h("input", {
            "data-role": "category-form-input",
            value: text.value,
            onInput: (event: Event) => {
              text.value = (event.target as HTMLInputElement).value;
            },
          }),
          h(
            "button",
            {
              "data-role": "category-form-save",
              onClick: () => emit("submit", text.value),
            },
            "保存",
          ),
        ],
      );
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
        ProfileCategoryFormModal: CategoryFormModalStub,
        ArticleFeedItem: ProfileMediaStub,
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

beforeEach(() => {
  vi.clearAllMocks();
  authStore.profileId = "100";
  categoryService.getProfileCategories.mockResolvedValue({
    list: [{ id: "c1", title: "工程笔记", article_count: 2 }],
    count: 1,
  });
  categoryService.saveProfileCategory.mockResolvedValue(undefined);
  categoryService.deleteProfileCategories.mockResolvedValue(undefined);

  vi.stubGlobal("useRoute", () => ({
    params: { id: "100" },
    query: { tab: "categories" },
    path: "/users/100",
  }));
  vi.stubGlobal("useRouter", () => ({ replace: vi.fn() }));
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useUiStore", () => ({ openAuthModal: vi.fn() }));
  vi.stubGlobal("useAsyncData", useAsyncDataRefetching);
  vi.stubGlobal("useProfileCategoryManage", useProfileCategoryManage);
  // 这几个是 Nuxt 自动导入：单测里没有编译器注入，只能补到 globalThis 上。
  vi.stubGlobal("useAuthStore", () => authStore);
  vi.stubGlobal("useArticleSearch", async () => ({ articles: ref([]), pending: ref(false) }));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("分类栏写入口：只有自己的主页才有", () => {
  it("自己的主页：有「新建分类」，每个分类有「改名」「删除」", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.get('[data-role="category-create"]').text()).toBe("新建分类");
    expect(wrapper.get('[data-role="category-rename-c1"]').text()).toBe("改名");
    expect(wrapper.get('[data-role="category-delete-c1"]').text()).toBe("删除");
  });

  it("他人主页：一个写入口都没有，分类仍是可点的只读入口", async () => {
    authStore.profileId = "999";

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="category-rename-c1"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="category-delete-c1"]').exists()).toBe(false);
    // 只读视图本身还在：分类卡片照常渲染。
    expect(wrapper.findAll(".profile-category-card")).toHaveLength(1);
  });

  it("未登录：同样没有任何写入口", async () => {
    authStore.profileId = null;
    authStore.isLoggedIn = false;

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
    expect(wrapper.findAll(".profile-category-card")).toHaveLength(1);
  });
});

describe("分类栏写操作：页面接线", () => {
  it("「新建分类」打开的是新建模式的弹窗，输入框留空", async () => {
    const wrapper = mountPage();
    await settle();

    const modal = () => wrapper.get('[data-role="category-form-modal"]');
    expect(modal().attributes("data-show")).toBe("false");

    await wrapper.get('[data-role="category-create"]').trigger("click");

    expect(modal().attributes("data-show")).toBe("true");
    expect(modal().attributes("data-mode")).toBe("create");
    expect(modal().attributes("data-initial")).toBe("");
  });

  it("「改名」打开改模式的弹窗，并把当前名字回填进去", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-rename-c1"]').trigger("click");

    const modal = wrapper.get('[data-role="category-form-modal"]');
    expect(modal.attributes("data-show")).toBe("true");
    expect(modal.attributes("data-mode")).toBe("rename");
    expect(modal.attributes("data-initial")).toBe("工程笔记");
  });

  it("点「删除」先弹二次确认，确认后按该分类发删除请求", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-delete-c1"]').trigger("click");

    expect(dialog.warning).toHaveBeenCalledTimes(1);
    const request = dialog.warning.mock.calls[0]![0] as {
      title: string;
      content: string;
      onPositiveClick: () => Promise<void>;
    };
    expect(request.title).toBe("删除分类");
    expect(request.content).toContain("2 篇文章");
    expect(categoryService.deleteProfileCategories).not.toHaveBeenCalled();

    await request.onPositiveClick();

    expect(categoryService.deleteProfileCategories).toHaveBeenCalledWith(["c1"]);
    expect(message.success).toHaveBeenCalledWith("分类已删除");
    // 写完以服务端为准：分类列表被重拉（首屏一次 + 删除后一次）。
    expect(categoryService.getProfileCategories).toHaveBeenCalledTimes(2);
  });
});

/**
 * 这条线以前是断的：弹窗 emit('submit', title)、页面 @submit="submitCategory"，
 * 但 submit() 不接参数、只读 formTitle，而 formTitle 没有任何输入回写来源。
 * 后果是新建永远提示"分类名称不能为空"，改名提交的则是旧标题（后端 UPDATE 同值，
 * 提示成功却什么都没变）。下面三条用例真的跨过这条线：
 * 用户敲的字 → 弹窗 emit → 页面接线 → composable → service 载荷。
 */
describe("分类栏写操作：弹窗提交的标题真的进到请求里（P0）", () => {
  it("新建：saveProfileCategory 收到的是弹窗里敲进去的标题", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-create"]').trigger("click");
    await wrapper.get('[data-role="category-form-input"]').setValue("工程笔记");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");
    await settle();

    expect(message.warning).not.toHaveBeenCalled();
    expect(categoryService.saveProfileCategory).toHaveBeenCalledTimes(1);
    expect(categoryService.saveProfileCategory).toHaveBeenCalledWith({ title: "工程笔记" });
    expect(wrapper.get('[data-role="category-form-modal"]').attributes("data-show")).toBe("false");
  });

  it("改名：提交的是新标题而不是回填的旧标题", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-rename-c1"]').trigger("click");
    const input = wrapper.get('[data-role="category-form-input"]');
    // 回填：打开改名时看到的是"改哪个名字"。
    expect((input.element as HTMLInputElement).value).toBe("工程笔记");

    await input.setValue("工程随笔");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");
    await settle();

    expect(categoryService.saveProfileCategory).toHaveBeenCalledWith({ id: "c1", title: "工程随笔" });
  });

  it("新建失败：弹窗保持打开，用户输入不丢", async () => {
    categoryService.saveProfileCategory.mockRejectedValueOnce(new Error("名称重复"));
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-create"]').trigger("click");
    await wrapper.get('[data-role="category-form-input"]').setValue("工程笔记");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");
    await settle();

    expect(message.error).toHaveBeenCalledWith("名称重复");
    expect(wrapper.get('[data-role="category-form-modal"]').attributes("data-show")).toBe("true");
    expect(
      (wrapper.get('[data-role="category-form-input"]').element as HTMLInputElement).value,
    ).toBe("工程笔记");
    // 失败不重拉列表（以服务端为准的前提是"服务端真写进去了"）。
    expect(categoryService.getProfileCategories).toHaveBeenCalledTimes(1);
  });
});
