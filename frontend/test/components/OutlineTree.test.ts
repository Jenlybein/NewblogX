import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { nextTick } from "vue";

import OutlineTree from "~/components/outline/OutlineTree.vue";
import type { OutlineNode } from "~/utils/outlineTree";

/** 组件级接缝：展开/收起（含当前文章所在文件夹）、带来源的链接、当前文章高亮、折叠持久化衔接。 */

const nodes: OutlineNode[] = [
  {
    id: "f1",
    kind: 1,
    title: "第一章",
    children: [
      {
        id: "a1",
        kind: 2,
        article_id: "101",
        article_title: "开篇",
        article_cover: "https://img.example/first.png",
        children: [],
      },
      {
        id: "f2",
        kind: 1,
        title: "深入",
        children: [
          {
            id: "a2",
            kind: 2,
            article_id: "102",
            article_title: "第二篇",
            children: [],
          },
        ],
      },
    ],
  },
  {
    id: "a3",
    kind: 2,
    article_id: "103",
    article_title: "附录",
    children: [],
  },
];

const NuxtLinkStub = {
  props: ["to"],
  template: '<a :href="to"><slot /></a>',
};

function mountTree(options: {
  collapsed?: Set<string>;
  currentArticleId?: string;
  kind?: "project" | "collection";
  ownerId?: string;
  nodes?: OutlineNode[];
} = {}) {
  return mount(OutlineTree, {
    props: {
      nodes: options.nodes ?? nodes,
      kind: options.kind ?? "collection",
      ownerId: options.ownerId ?? "c1",
      collapsed: options.collapsed ?? new Set<string>(),
      currentArticleId: options.currentArticleId ?? "",
    },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

describe("OutlineTree：读者详情页目录树", () => {
  it("深度优先渲染嵌套行，层级写在 data-depth（缩进表达位置）", () => {
    const wrapper = mountTree();

    expect(wrapper.get("[data-outline-tree]").exists()).toBe(true);
    expect(wrapper.findAll("[data-outline-row]").map((item) => item.attributes("data-outline-row"))).toEqual([
      "f1",
      "a1",
      "f2",
      "a2",
      "a3",
    ]);
    expect(wrapper.findAll("[data-outline-row]").map((item) => item.attributes("data-depth"))).toEqual([
      "0",
      "1",
      "1",
      "2",
      "0",
    ]);
    expect(wrapper.text()).not.toMatch(/第\s*\d+\s*篇/u);
  });

  it("文件夹是折叠按钮不是链接，文章节点带 <kind>:<id> 来源", () => {
    const wrapper = mountTree({ kind: "project", ownerId: "375" });

    const toggle = wrapper.get('[data-outline-toggle="f1"]');
    expect(toggle.attributes("aria-expanded")).toBe("true");
    expect(wrapper.findAll('[data-outline-kind="folder"] a')).toHaveLength(0);
    expect(wrapper.get('[data-outline-row="a1"] a').attributes("href")).toBe(
      "/article/101?from=project:375",
    );
    expect(wrapper.get('[data-outline-row="a2"] a').attributes("href")).toBe(
      "/article/102?from=project:375",
    );
  });

  it("收起的文件夹只隐藏后代，点箭头发出 toggle 事件由父页面保存状态", async () => {
    const wrapper = mountTree({ collapsed: new Set(["f1"]) });

    expect(wrapper.findAll("[data-outline-row]").map((item) => item.attributes("data-outline-row"))).toEqual([
      "f1",
      "a3",
    ]);
    expect(wrapper.get('[data-outline-toggle="f1"]').attributes("aria-expanded")).toBe("false");

    await wrapper.get('[data-outline-toggle="f1"]').trigger("click");
    expect(wrapper.emitted("toggle")).toEqual([["f1"]]);
  });

  it("当前文章可见时高亮；它所在的文件夹被收起时保持收起，当前行跟着隐藏", () => {
    const wrapper = mountTree({ currentArticleId: "102" });

    expect(wrapper.get('[data-outline-row="a2"]').classes()).toContain("is-current");
    expect(wrapper.get('[data-outline-row="a2"]').attributes("aria-current")).toBe("page");
    expect(wrapper.get('[data-outline-row="a1"]').classes()).not.toContain("is-current");
    expect(wrapper.get('[data-outline-row="f1"]').classes()).not.toContain("is-current");

    const collapsedWrapper = mountTree({ collapsed: new Set(["f1"]), currentArticleId: "102" });

    expect(collapsedWrapper.findAll("[data-outline-row]").map((row) => row.attributes("data-outline-row"))).toEqual([
      "f1",
      "a3",
    ]);
    expect(collapsedWrapper.find('[data-outline-row="a2"]').exists()).toBe(false);
    expect(collapsedWrapper.find("[data-outline-row].is-current").exists()).toBe(false);
  });

  it("空树不渲染任何节点；空态文案由父页面的 ProfileEmptyState 负责", () => {
    const wrapper = mountTree({ nodes: [] });

    expect(wrapper.findAll("[data-outline-row]")).toHaveLength(0);
  });

  it("文章行是纯文字：不再渲染封面缩略图 / img / 封面占位", () => {
    const wrapper = mountTree();

    expect(wrapper.find(".outline-tree__cover").exists()).toBe(false);
    expect(wrapper.find(".article-cover-default").exists()).toBe(false);
    expect(wrapper.findAll("img")).toHaveLength(0);
    expect(wrapper.get('[data-outline-row="a1"]').text()).toContain("开篇");
  });

  it("每行都有固定图标列：文件夹是开合图标，文章是文件图标", () => {
    const wrapper = mountTree();

    expect(wrapper.findAll('[data-outline-kind="folder"]')).toHaveLength(2);
    expect(wrapper.findAll('[data-outline-kind="article"]')).toHaveLength(3);
    for (const row of wrapper.findAll("[data-outline-row]")) {
      expect(row.find(".outline-tree__icon").exists()).toBe(true);
    }
    expect(wrapper.findAll(".outline-tree__icon--folder")).toHaveLength(2);
    expect(wrapper.findAll(".outline-tree__icon--article")).toHaveLength(3);
  });

  it("折叠文件夹时箭头与文件夹图标一起换形（展开态由同一份 collapsed 决定）", () => {
    const wrapper = mountTree({ collapsed: new Set(["f1"]) });

    expect(wrapper.get('[data-outline-toggle="f1"]').attributes("aria-expanded")).toBe("false");
    expect(wrapper.get('[data-outline-row="f1"] .outline-tree__folder-glyph').classes()).not.toContain("is-open");

    const openWrapper = mountTree();
    expect(openWrapper.get('[data-outline-row="f1"] .outline-tree__folder-glyph').classes()).toContain("is-open");
  });

  it("展开文件夹带出的新行带入场标记：只标记新增行，原有行不重播", async () => {
    const wrapper = mountTree({ collapsed: new Set(["f1"]) });
    expect(wrapper.findAll("[data-outline-row]").map((row) => row.attributes("data-outline-row"))).toEqual([
      "f1",
      "a3",
    ]);

    await wrapper.setProps({ collapsed: new Set<string>() });
    await nextTick();

    expect(wrapper.get('[data-outline-row="a1"]').classes()).toContain("is-entering");
    expect(wrapper.get('[data-outline-row="f2"]').classes()).toContain("is-entering");
    expect(wrapper.get('[data-outline-row="a2"]').classes()).toContain("is-entering");
    expect(wrapper.get('[data-outline-row="f1"]').classes()).not.toContain("is-entering");
    expect(wrapper.get('[data-outline-row="a3"]').classes()).not.toContain("is-entering");
  });
});
