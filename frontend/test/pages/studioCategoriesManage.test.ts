import { mount } from "@vue/test-utils";
import { Suspense, defineComponent, h, ref, watch } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import StudioCategoriesPage from "~/pages/studio/categories.vue";

/**
 * 个人中心 - 分类管理（#327）。
 *
 * 用户纠正过定位：分类的管理属于个人主页 / 个人中心，不该散落在"写作页"或只留在
 * 公开主页里。这一页是分类的**唯一集中管理入口**，写流程复用
 * `useProfileCategoryManage`（新建 / 改名 / 删除 + 二次确认 + 写完重拉列表都在那里，
 * 它有自己的用例），页面只负责：列出来、把按钮接上、把服务端结果画出来。
 *
 * 所以这里的用例要跨过页面这条缝：
 *   1. 列表渲染：标题 + 文章数（空态 / 加载态也要有）；
 *   2. 新建：弹窗里敲的字真的进到 `saveProfileCategory` 的载荷里（不是只断言弹窗打开）；
 *   3. 删除：先二次确认（并告知"该分类下有 N 篇文章，删除后变成未分类"），
 *      确认后才发删除请求，成功后按服务端为准重拉列表。
 *
 * 替身理由与既有页面用例一致：Nuxt 编译期宏（definePageMeta / useSeoMeta /
 * useNoIndexPage）与自动导入（useAsyncData / useAuthStore）只能桩掉；两个 service
 * 换成记录器；弹窗换成"真的能输入、真的 emit submit"的替身（只渲染属性的替身
 * 会让"按钮接没接上"两侧各自绿、中间没测）。
 */

const authStore = vi.hoisted(() => ({ profileId: "100" as string | null, isLoggedIn: true }));
const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const dialog = vi.hoisted(() => ({ warning: vi.fn() }));

const categoryService = vi.hoisted(() => ({
  getProfileCategories: vi.fn(),
  saveProfileCategory: vi.fn(),
  deleteProfileCategories: vi.fn(),
}));

vi.mock("~/services/category", () => categoryService);
vi.mock("naive-ui", () => ({ useMessage: () => message, useDialog: () => dialog }));

/**
 * 弹窗替身：保留输入，按真弹窗的规则在每次打开时用 initialTitle 重置，
 * 点保存把当前文本 emit 出去 —— 与 ProfileCategoryFormModal 的行为一致。
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

    watch(
      () => [props.show, props.initialTitle] as const,
      ([show, initial]) => {
        if (show) text.value = initial;
      },
      { immediate: true },
    );

    return () =>
      h("div", { "data-role": "category-form-modal", "data-show": String(props.show), "data-mode": props.mode }, [
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
      ]);
  },
});

function mountPage() {
  return mount(
    defineComponent({
      name: "PageUnderSuspense",
      setup() {
        return () => h(Suspense, null, { default: () => h(StudioCategoriesPage) });
      },
    }),
    {
      global: {
        stubs: {
          NuxtLink: defineComponent({
            props: { to: { type: [String, Object], default: "" } },
            setup(_props, { slots }) {
              return () => h("a", slots.default?.());
            },
          }),
          ProfileCategoryFormModal: CategoryFormModalStub,
        },
      },
    },
  );
}

/** useAsyncData：refresh 会重新跑取数函数（"写完重拉列表"要靠它才观察得到）。 */
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

async function settle() {
  for (let i = 0; i < 6; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

beforeEach(() => {
  vi.clearAllMocks();

  authStore.profileId = "100";
  categoryService.getProfileCategories.mockResolvedValue({
    list: [
      { id: "c1", title: "工程笔记", article_count: 2 },
      { id: "c2", title: "随笔", article_count: 0 },
    ],
    count: 2,
  });
  categoryService.saveProfileCategory.mockResolvedValue(undefined);
  categoryService.deleteProfileCategories.mockResolvedValue(undefined);

  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useNoIndexPage", () => undefined);
  vi.stubGlobal("useAsyncData", useAsyncDataRefetching);
  vi.stubGlobal("useAuthStore", () => authStore);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("分类管理页：列表", () => {
  it("列出自己的分类：标题 + 文章数", async () => {
    const wrapper = mountPage();
    await settle();

    expect(categoryService.getProfileCategories).toHaveBeenCalledWith("100");
    expect(wrapper.get('[data-role="category-title-c1"]').text()).toBe("工程笔记");
    expect(wrapper.get('[data-role="category-count-c1"]').text()).toContain("2");
    expect(wrapper.get('[data-role="category-count-c1"]').text()).toContain("篇文章");
    expect(wrapper.get('[data-role="category-title-c2"]').text()).toBe("随笔");
    expect(wrapper.find('[data-role="category-row-c1"]').exists()).toBe(true);
    expect(wrapper.find('[data-role="category-row-c2"]').exists()).toBe(true);
  });

  it("加载中显示加载态，没有分类时显示空态与新建入口", async () => {
    let resolveList!: (value: unknown) => void;
    categoryService.getProfileCategories.mockReturnValue(
      new Promise((resolve) => {
        resolveList = resolve;
      }),
    );

    const wrapper = mountPage();
    // 页面是 `await useAsyncData(...)` 的异步 setup：先等 Suspense 解析、组件登场，
    // 此时取数 promise 还挂着，应当看到加载态。
    await settle();

    expect(wrapper.get('[data-state="pending"]').text()).toContain("正在读取分类");

    resolveList({ list: [], count: 0 });
    await settle();

    const empty = wrapper.get('[data-state="empty"]');
    expect(empty.text()).toContain("还没有分类");
    // 空态里也要能开始：新建入口一直可用。
    expect(wrapper.get('[data-role="category-create"]').text()).toContain("新建分类");
  });
});

describe("分类管理页：新建", () => {
  it("新建走 saveProfileCategory，弹窗里敲的标题真的进到载荷里，写完重拉列表", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.get('[data-role="category-create"]').text()).toContain("新建分类");
    await wrapper.get('[data-role="category-create"]').trigger("click");
    expect(wrapper.get('[data-role="category-form-modal"]').attributes("data-show")).toBe("true");

    await wrapper.get('[data-role="category-form-input"]').setValue("  新分类  ");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");
    await settle();

    expect(message.warning).not.toHaveBeenCalled();
    expect(categoryService.saveProfileCategory).toHaveBeenCalledTimes(1);
    // 首尾空白在 composable 里去掉。
    expect(categoryService.saveProfileCategory).toHaveBeenCalledWith({ title: "新分类" });
    // 写完以服务端为准：首屏一次 + 新建后一次。
    expect(categoryService.getProfileCategories).toHaveBeenCalledTimes(2);
  });

  it("改名提交的是新标题（带分类 id）", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="category-rename-c1"]').trigger("click");
    expect(wrapper.get('[data-role="category-form-modal"]').attributes("data-mode")).toBe("rename");

    await wrapper.get('[data-role="category-form-input"]').setValue("工程随笔");
    await wrapper.get('[data-role="category-form-save"]').trigger("click");
    await settle();

    expect(categoryService.saveProfileCategory).toHaveBeenCalledWith({ id: "c1", title: "工程随笔" });
  });
});

describe("分类管理页：删除", () => {
  it("先二次确认并告知影响篇数；确认后才删，删完重拉列表", async () => {
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
    expect(request.content).toContain("未分类");
    expect(categoryService.deleteProfileCategories).not.toHaveBeenCalled();

    await request.onPositiveClick();

    expect(categoryService.deleteProfileCategories).toHaveBeenCalledWith(["c1"]);
    expect(message.success).toHaveBeenCalledWith("分类已删除");
    expect(categoryService.getProfileCategories).toHaveBeenCalledTimes(2);
  });
});
