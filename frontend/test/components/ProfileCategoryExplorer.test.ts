import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import ProfileCategoryExplorer from "~/components/profile/ProfileCategoryExplorer.vue";
import type { ProfileCategoryItem, SearchArticleItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(_props, { slots }) {
    return () => h("a", slots.default?.());
  },
});

function category(overrides: Partial<ProfileCategoryItem> = {}): ProfileCategoryItem {
  return {
    id: "c1",
    title: "工程笔记",
    article_count: 3,
    ...overrides,
  };
}

function article(overrides: Partial<SearchArticleItem> = {}): SearchArticleItem {
  return {
    id: "art-1",
    title: "按分类展开的文章",
    abstract: "摘要。",
    cover: "",
    created_at: "2026-10-05T12:30:00Z",
    ...overrides,
  } as unknown as SearchArticleItem;
}

type ExplorerProps = {
  categories?: ProfileCategoryItem[];
  activeCategoryId?: string;
  articles?: SearchArticleItem[];
  pending?: boolean;
  articlesPending?: boolean;
  articlesFailed?: boolean;
  failed?: boolean;
};

function mountExplorer(props: ExplorerProps = {}) {
  return mount(ProfileCategoryExplorer, {
    props: {
      categories: props.categories ?? [],
      activeCategoryId: props.activeCategoryId ?? "",
      articles: props.articles ?? [],
      pending: props.pending ?? false,
      articlesPending: props.articlesPending ?? false,
      articlesFailed: props.articlesFailed ?? false,
      failed: props.failed ?? false,
    },
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        IconThumbUp: true,
        IconHeart: true,
        IconMessageCircle2: true,
        IconEye: true,
      },
    },
  });
}

describe("分类栏：列表渲染", () => {
  it("每个分类显示标题与文章数，且分类是可点的一等入口", () => {
    const wrapper = mountExplorer({
      categories: [category(), category({ id: "c2", title: "读书", article_count: 0 })],
    });

    const cards = wrapper.findAll(".profile-category-card");
    expect(cards).toHaveLength(2);
    expect(cards[0]?.get(".profile-category-card__title").text()).toBe("工程笔记");
    expect(cards[0]?.text()).toContain("3");
    expect(cards[1]?.get(".profile-category-card__title").text()).toBe("读书");
    expect(cards[1]?.text()).toContain("0");
    expect(cards[0]?.attributes("aria-pressed")).toBe("false");
  });

  it("加载中：分类列表区是 pending 态，不是空白", () => {
    const wrapper = mountExplorer({ pending: true });

    const empty = wrapper.get(".profile-category-layout__folders [data-empty-state='true']");
    expect(empty.attributes("data-state")).toBe("pending");
    expect(empty.text()).toContain("正在读取分类");
  });

  it("没有分类时给明确空状态，不是一片空白", () => {
    const wrapper = mountExplorer({ categories: [] });

    const empty = wrapper.get(".profile-category-layout__folders [data-empty-state='true']");
    expect(empty.text()).toContain("这位作者还没有分类");
    // 内容区同样有占位结构（不是塌成一条）。
    expect(wrapper.find(".profile-category-layout__content [data-empty-state='true']").exists()).toBe(true);
  });

  it("分类列表读取失败时说明原因，不伪装成「没有分类」", () => {
    const wrapper = mountExplorer({ failed: true });

    const empty = wrapper.get(".profile-category-layout__folders [data-empty-state='true']");
    expect(empty.text()).toContain("分类暂时打不开");
    expect(empty.text()).not.toContain("还没有分类");
  });
});

describe("分类栏：点分类就地展开文章", () => {
  it("点一个分类就地选中它（交给页面取该作者该分类下的文章），不跳页", async () => {
    const wrapper = mountExplorer({
      categories: [category(), category({ id: "c2", title: "读书" })],
    });

    const cards = wrapper.findAll(".profile-category-card");
    await cards[0]!.trigger("click");

    expect(wrapper.emitted("update:activeCategoryId")).toEqual([["c1"]]);
    // 不跳页：组件里没有任何指向搜索/分类页的跳转入口。
    expect(wrapper.find("a").exists()).toBe(false);
  });

  it("再点同一个分类就收起（选中态可以取消）", async () => {
    const wrapper = mountExplorer({
      categories: [category()],
      activeCategoryId: "c1",
      articles: [article()],
    });

    await wrapper.get(".profile-category-card").trigger("click");

    expect(wrapper.emitted("update:activeCategoryId")).toEqual([[""]]);
  });

  it("选中分类后在该栏内列出这位作者在这个分类下的文章，并标出分类名与篇数", () => {
    const wrapper = mountExplorer({
      categories: [category()],
      activeCategoryId: "c1",
      articles: [article(), article({ id: "art-2", title: "同分类第二篇" })],
    });

    // 票 351 起文章区是多列卡（不再是单列 feed-entry 条目）。
    const entries = wrapper.findAll(".article-grid-card");
    expect(entries).toHaveLength(2);
    expect(entries[0]?.text()).toContain("按分类展开的文章");
    expect(entries[1]?.text()).toContain("同分类第二篇");

    const heading = wrapper.get(".profile-category-heading");
    expect(heading.text()).toContain("工程笔记");
    expect(heading.text()).toContain("2");
    expect(wrapper.get(".profile-category-card").classes()).toContain("is-active");
  });

  it("还没选分类时，内容区给「选一个分类」的指引", () => {
    const wrapper = mountExplorer({ categories: [category()] });

    const empty = wrapper.get(".profile-category-layout__content [data-empty-state='true']");
    expect(empty.text()).toContain("选一个分类");
  });

  it("选中的分类下没有公开文章时是空态，不是空白", () => {
    const wrapper = mountExplorer({ categories: [category()], activeCategoryId: "c1", articles: [] });

    const empty = wrapper.get(".profile-category-layout__content [data-empty-state='true']");
    expect(empty.text()).toContain("这个分类下还没有公开文章");
  });

  it("文章还在路上时内容区是 pending 态占位", () => {
    const wrapper = mountExplorer({
      categories: [category()],
      activeCategoryId: "c1",
      articlesPending: true,
    });

    const empty = wrapper.get(".profile-category-layout__content [data-empty-state='true']");
    expect(empty.attributes("data-state")).toBe("pending");
    expect(empty.text()).toContain("正在读取分类文章");
  });

  it("展开失败时说明读不到，不伪装成「这个分类没有文章」", () => {
    const wrapper = mountExplorer({
      categories: [category()],
      activeCategoryId: "c1",
      articlesFailed: true,
    });

    const empty = wrapper.get(".profile-category-layout__content [data-empty-state='true']");
    expect(empty.text()).toContain("分类文章暂时打不开");
    expect(empty.text()).not.toContain("还没有公开文章");
  });
});

describe("分类栏：他人主页是只读视图", () => {
  it("默认不渲染新建 / 改名 / 删除等写入口（页面只在「自己主页」时填插槽）", () => {
    const wrapper = mountExplorer({ categories: [category()], activeCategoryId: "c1", articles: [article()] });

    const text = wrapper.text();
    for (const banned of ["新建分类", "改名", "重命名", "删除分类", "编辑分类"]) {
      expect(text).not.toContain(banned);
    }
    expect(wrapper.findAll("button").map((item) => item.text())).toEqual(["工程笔记3"]);
    // 连工具栏容器都不该出现：页面只在"自己主页"时填插槽。
    expect(wrapper.find(".profile-category-toolbar").exists()).toBe(false);
  });
});

describe("分类栏：写入口不在这层，也没有留插槽（票 327 口径）", () => {
  /**
   * 票 325 时这里验证过 `toolbar` / `category-actions` 两个插槽能把写入口接进来。
   * 用户随后纠正了定位：分类的管理只在个人中心 `/studio/categories`，公开主页对
   * 任何身份都只读。插槽如果没人填，就是死代码 —— 所以整组删掉，这里反过来钉住
   * "谁都塞不进来"：即便调用方硬传插槽，组件也不渲染它。
   */
  it("即便调用方传入 toolbar / category-actions 插槽，也不会渲染出写入口", () => {
    const wrapper = mount(ProfileCategoryExplorer, {
      props: {
        categories: [category(), category({ id: "c2", title: "读书", article_count: 0 })],
        activeCategoryId: "",
        articles: [],
        pending: false,
        articlesPending: false,
        articlesFailed: false,
        failed: false,
      },
      slots: {
        toolbar: () => h("button", { type: "button", "data-role": "category-create" }, "新建分类"),
        "category-actions": (scope: { category: ProfileCategoryItem }) =>
          h("button", { type: "button", "data-role": `rename-${scope.category.id}` }, `改名 ${scope.category.title}`),
      },
      global: {
        stubs: {
          NuxtLink: NuxtLinkStub,
          IconThumbUp: true,
          IconHeart: true,
          IconMessageCircle2: true,
          IconEye: true,
        },
      },
    });

    expect(wrapper.find(".profile-category-toolbar").exists()).toBe(false);
    expect(wrapper.find('[data-role="category-create"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="rename-c1"]').exists()).toBe(false);
    // 只读视图本身照旧：分类仍是可点的一等入口。
    expect(wrapper.findAll(".profile-category-card")).toHaveLength(2);
  });
});
