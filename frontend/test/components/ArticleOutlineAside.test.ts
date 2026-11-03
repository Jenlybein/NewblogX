import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticleOutlineAside from "~/components/article/ArticleOutlineAside.vue";
import type { ArticleOutlineContext } from "~/utils/outlineContext";

/**
 * 阅读页左栏（组件接缝）：有树出现、无归属不出现、当前节点高亮、上一篇/下一篇、
 * 窄屏「目录」抽屉与首屏骨架占位。版式的三栏结构由页面负责（这里只断言行内结构）。
 */

const NuxtLinkStub = {
  props: ["to"],
  template: '<a :href="to"><slot /></a>',
};

function makeContext(overrides: Partial<ArticleOutlineContext> = {}): ArticleOutlineContext {
  return {
    has_tree: true,
    owner_kind: 2,
    owner_id: "375",
    nodes: [
      {
        id: "f1",
        kind: 1,
        title: "第一章",
        children: [
          { id: "n1", kind: 2, article_id: "101", article_title: "第一篇", article_cover: "" },
          { id: "n2", kind: 2, article_id: "102", article_title: "第二篇", article_cover: "" },
        ],
      },
    ],
    current: { id: "n1", article_id: "101", article_title: "第一篇" },
    previous: null,
    next: { id: "n2", article_id: "102", article_title: "第二篇" },
    ...overrides,
  };
}

function mountAside(options: {
  context?: ArticleOutlineContext | null;
  pending?: boolean;
  currentArticleId?: string;
} = {}) {
  return mount(ArticleOutlineAside, {
    props: {
      context: options.context === undefined ? makeContext() : options.context,
      pending: options.pending ?? false,
      currentArticleId: options.currentArticleId ?? "101",
    },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

describe("ArticleOutlineAside：有归属树时左栏出现", () => {
  it("渲染整棵树并高亮当前文章节点", () => {
    const wrapper = mountAside();

    expect(wrapper.get("[data-article-outline]").exists()).toBe(true);
    expect(wrapper.findAll("[data-outline-row]").map((row) => row.attributes("data-outline-row"))).toEqual([
      "f1",
      "n1",
      "n2",
    ]);
    const current = wrapper.get('[data-outline-row="n1"]');
    expect(current.classes()).toContain("is-current");
    expect(current.attributes("aria-current")).toBe("page");
    expect(wrapper.text()).toContain("目录");
  });

  it("上一篇 / 下一篇沿用同一棵树的 from（翻页不串专题）", () => {
    const wrapper = mountAside();

    expect(wrapper.get('[data-outline-neighbour="next"]').attributes("href")).toBe(
      "/article/102?from=project:375",
    );
    expect(wrapper.get('[data-outline-neighbour="next"]').text()).toContain("第二篇");
    expect(wrapper.find('[data-outline-neighbour="previous"]').exists()).toBe(false);
  });

  it("最后一篇没有下一篇时只渲染上一篇", () => {
    const wrapper = mountAside({
      currentArticleId: "102",
      context: makeContext({
        current: { id: "n2", article_id: "102", article_title: "第二篇" },
        previous: { id: "n1", article_id: "101", article_title: "第一篇" },
        next: null,
      }),
    });

    expect(wrapper.get('[data-outline-neighbour="previous"]').attributes("href")).toBe(
      "/article/101?from=project:375",
    );
    expect(wrapper.find('[data-outline-neighbour="next"]').exists()).toBe(false);
  });
});

describe("ArticleOutlineAside：无归属或取数失败时不出现", () => {
  it("has_tree=false 时整栏不渲染（右栏标题大纲保留在页面侧）", () => {
    const wrapper = mountAside({ context: makeContext({ has_tree: false }) });

    expect(wrapper.find("[data-article-outline]").exists()).toBe(false);
  });

  it("既不在取数中、也没有树时同样不渲染空栏", () => {
    const wrapper = mountAside({ context: null, pending: false });

    expect(wrapper.find("[data-article-outline]").exists()).toBe(false);
  });
});

describe("ArticleOutlineAside：首屏骨架占位", () => {
  it("pending 时先出与侧栏同构的骨架，并给读屏一句加载提示", () => {
    const wrapper = mountAside({ context: null, pending: true });

    expect(wrapper.get("[data-article-outline]").attributes("data-state")).toBe("pending");
    expect(wrapper.find('[data-role="article-outline-skeleton"]').exists()).toBe(true);
    expect(wrapper.get('[data-role="article-outline-skeleton"]').findAll("[data-role='skeleton-bar']").length).toBeGreaterThan(0);
    expect(wrapper.text()).toContain("文章目录正在加载");
  });

  it("树到达后原地替换骨架（同一栏位，不新增/移除列）", async () => {
    const wrapper = mountAside({ context: null, pending: true, currentArticleId: "101" });

    await wrapper.setProps({ context: makeContext(), pending: false });

    expect(wrapper.get("[data-article-outline]").attributes("data-state")).toBe("ready");
    expect(wrapper.find('[data-role="article-outline-skeleton"]').exists()).toBe(false);
    expect(wrapper.find("[data-outline-tree]").exists()).toBe(true);
  });
});

describe("ArticleOutlineAside：<1024px 的「目录」抽屉", () => {
  it("有「目录」展开按钮，aria-expanded 反映抽屉状态", async () => {
    const wrapper = mountAside();
    const toggle = wrapper.get("[data-outline-drawer-toggle]");

    expect(toggle.attributes("aria-expanded")).toBe("false");
    expect(toggle.text()).toContain("目录");
    expect(toggle.attributes("aria-controls")).toBe("article-outline-panel");

    await toggle.trigger("click");
    expect(toggle.attributes("aria-expanded")).toBe("true");
    expect(wrapper.get("[data-outline-panel]").classes()).toContain("is-drawer-open");
  });

  it("点击遮罩关闭抽屉（窄屏正文不被侧栏长期挤占）", async () => {
    const wrapper = mountAside();
    await wrapper.get("[data-outline-drawer-toggle]").trigger("click");

    const backdrop = wrapper.get("[data-outline-drawer-backdrop]");
    expect(backdrop.attributes("aria-label")).toBe("关闭目录");
    await backdrop.trigger("click");

    expect(wrapper.get("[data-outline-drawer-toggle]").attributes("aria-expanded")).toBe("false");
    expect(wrapper.get("[data-outline-panel]").classes()).not.toContain("is-drawer-open");
  });
});
