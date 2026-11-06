import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import OutlineTree from "~/components/outline/OutlineTree.vue";
import type { OutlineNode } from "~/utils/outlineTree";

/** 组件级接缝：展开/收起、带来源的链接、当前文章高亮、折叠持久化衔接、沟槽圆点。 */

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

  it("当前文章高亮且命中被收起分支时自动展开，文件夹自己不高亮", () => {
    const wrapper = mountTree({ collapsed: new Set(["f1", "f2"]), currentArticleId: "102" });

    expect(wrapper.get('[data-outline-row="a2"]').classes()).toContain("is-current");
    expect(wrapper.get('[data-outline-row="a2"]').attributes("aria-current")).toBe("page");
    expect(wrapper.get('[data-outline-row="a1"]').classes()).not.toContain("is-current");
    expect(wrapper.get('[data-outline-row="f1"]').classes()).not.toContain("is-current");
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
});

/** 挂载后等一次微任务：圆点要等 `onMounted` 里的首次测量才会出现。 */
async function mountLanded(options: Parameters<typeof mountTree>[0] = {}) {
  const wrapper = mountTree(options);
  await nextTick();
  await flushPromises();
  return wrapper;
}

/**
 * 把 `document.fonts.ready` 换成一个**由测试手动兑现**的 Promise。
 *
 * 为什么必须自己控制时机：字体晚到是真实浏览器里的常态（webfont 比首帧慢），
 * 而 happy-dom 根本没有 `document.fonts`。只有把"就绪"这一刻捏在手里，
 * 才能断言"字体晚到时圆点做了什么"（而不是只断言它最终停在哪）。
 */
function deferFontsReady() {
  let release: () => void = () => {};
  const ready = new Promise<void>((resolve) => {
    release = resolve;
  });
  const original = Object.getOwnPropertyDescriptor(document, "fonts");
  Object.defineProperty(document, "fonts", { configurable: true, value: { ready } });

  return {
    /** 兑现 `fonts.ready`，并把因此触发的重对准跑完。 */
    async land() {
      release();
      await flushPromises();
    },
    restore() {
      if (original) Object.defineProperty(document, "fonts", original);
      else Reflect.deleteProperty(document, "fonts");
    },
  };
}

describe("沟槽圆点：只跟随当前文章", () => {
  it("两个详情页没有当前文章时不渲染圆点", () => {
    const wrapper = mountTree();

    expect(wrapper.find("[data-outline-dot]").exists()).toBe(false);
  });

  it("首次进入直接落位（snap），且圆点是装饰：aria-hidden、不可聚焦、不含链接", async () => {
    const wrapper = await mountLanded({ currentArticleId: "102" });
    const dot = wrapper.get("[data-outline-dot]");

    expect(dot.attributes("aria-hidden")).toBe("true");
    expect(dot.attributes("tabindex")).toBeUndefined();
    expect(dot.findAll("a, button")).toHaveLength(0);
    expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    expect(dot.attributes("data-outline-dot-motion")).toBe("snap");
  });

  it("首次进入字体晚到：仍是落位（snap），入场不会因为字体替换而滑一段", async () => {
    const fonts = deferFontsReady();

    try {
      const wrapper = await mountLanded({ currentArticleId: "102" });
      expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-motion")).toBe("snap");

      // 字体到货 → 行高变化 → 重测；这一趟"还没移动过"，所以不许播弹簧。
      await fonts.land();

      const dot = wrapper.get("[data-outline-dot]");
      expect(dot.attributes("data-outline-dot-motion")).toBe("snap");
      expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    } finally {
      fonts.restore();
    }
  });

  it("切过文章后字体才就绪：按普通重对准走弹簧（travel），不把圆点拽回旧位置", async () => {
    const fonts = deferFontsReady();

    try {
      const wrapper = await mountLanded({ currentArticleId: "101" });
      await wrapper.setProps({ currentArticleId: "102" });
      await nextTick();
      expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-motion")).toBe("travel");

      await fonts.land();

      const dot = wrapper.get("[data-outline-dot]");
      expect(dot.attributes("data-outline-dot-motion")).toBe("travel");
      expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    } finally {
      fonts.restore();
    }
  });

  it("圆点的深度跟着当前行：跨层级时透传新的 depth（横向落在该层引导线上）", async () => {
    const wrapper = await mountLanded({ currentArticleId: "101" });
    expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-depth")).toBe("1");

    await wrapper.setProps({ currentArticleId: "102" });
    await nextTick();

    expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-depth")).toBe("2");
  });

  it("同树内切文章：圆点换目标并开始移动（travel），不重挂载", async () => {
    const wrapper = await mountLanded({ currentArticleId: "101" });

    await wrapper.setProps({ currentArticleId: "102" });
    await nextTick();

    const dot = wrapper.get("[data-outline-dot]");
    expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    expect(dot.attributes("data-outline-dot-motion")).toBe("travel");
    // aria-current 与高亮跟着换，圆点不改变这套语义。
    expect(wrapper.get('[data-outline-row="a2"]').attributes("aria-current")).toBe("page");
    expect(wrapper.get('[data-outline-row="a1"]').attributes("aria-current")).toBeUndefined();
  });

  it("折叠文件夹让目标行位移后重新对准（目标行不变，重新移动）", async () => {
    const wrapper = await mountLanded({ currentArticleId: "103" });
    expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-target")).toBe("a3");
    expect(wrapper.get("[data-outline-dot]").attributes("data-outline-dot-motion")).toBe("snap");

    // 收起上方的第一章：它整支隐藏，附录（当前文章）向上位移。
    await wrapper.setProps({ collapsed: new Set(["f1"]) });
    await nextTick();

    const dot = wrapper.get("[data-outline-dot]");
    expect(dot.attributes("data-outline-dot-target")).toBe("a3");
    expect(dot.attributes("data-outline-dot-motion")).toBe("travel");
    expect(wrapper.findAll("[data-outline-row]").map((row) => row.attributes("data-outline-row"))).toEqual([
      "f1",
      "a3",
    ]);
  });

  it("hover 与键盘聚焦不动圆点（只有当前文章换才动）", async () => {
    const wrapper = await mountLanded({ currentArticleId: "102" });

    await wrapper.get('[data-outline-row="a1"] a').trigger("mouseenter");
    await wrapper.get('[data-outline-row="a1"] a').trigger("focus");
    await wrapper.get('[data-outline-row="f2"] button').trigger("mouseenter");
    await nextTick();

    const dot = wrapper.get("[data-outline-dot]");
    expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    expect(dot.attributes("data-outline-dot-motion")).toBe("snap");
  });

  it("减少动态：切文章时圆点瞬移（snap），不保留滑动", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));

    const wrapper = await mountLanded({ currentArticleId: "101" });
    await wrapper.setProps({ currentArticleId: "102" });
    await nextTick();

    const dot = wrapper.get("[data-outline-dot]");
    expect(dot.attributes("data-outline-dot-target")).toBe("a2");
    expect(dot.attributes("data-outline-dot-motion")).toBe("snap");
  });

  it("圆点不改变键盘顺序：焦点仍然只有文件夹箭头与文章链接", async () => {
    const wrapper = await mountLanded({ currentArticleId: "102" });

    const focusables = wrapper
      .findAll("a, button")
      .map((element) => element.attributes("href") ?? element.attributes("data-outline-toggle"));

    expect(focusables).toEqual([
      "f1",
      "/article/101?from=collection:c1",
      "f2",
      "/article/102?from=collection:c1",
      "/article/103?from=collection:c1",
    ]);
    expect(wrapper.get("[data-outline-dot]").element.closest("a, button")).toBeNull();
  });
});
