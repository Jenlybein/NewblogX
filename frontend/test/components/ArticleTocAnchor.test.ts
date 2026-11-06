import { readFileSync } from "node:fs";
import { join } from "node:path";

import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";

import ArticleTocAnchor from "~/components/article/ArticleTocAnchor.vue";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 标题大纲的「轨道 + 飞行器」（票 296）。
 *
 * 这一层测的是**组件对外的协议**，不是画得像不像：
 *   - naive `NAnchor` 仍是行为层（`bound=96` 的滚动绑定、aria、键盘）；
 *   - 视觉层叠在上面：每个标题一个节点、当前节点实心、飞机停在当前节点；
 *   - 点击标题 → 抛 jump + 钉住；读者自己滚 → 恢复跟随；
 *   - 进度条与百分比仍在；<1024 整块不渲染；减少动态下没有滑动过渡。
 *
 * naive-ui 与 motion-v 在这里用桩替换：动画框架的插值不是这里要测的东西，
 * 弹簧参数来自 `utils/motion.ts` 的预设（由 motion-spring-gates 门禁守着）。
 */

vi.mock("naive-ui", () => {
  const NAnchor = defineComponent({
    name: "NAnchor",
    inheritAttrs: false,
    props: {
      bound: { type: Number, default: 0 },
      showRail: { type: Boolean, default: true },
      type: { type: String, default: "default" },
    },
    setup(props, { slots }) {
      return () =>
        h("div", { class: "n-anchor", "data-bound": String(props.bound) }, slots.default?.());
    },
  });

  const NAnchorLink = defineComponent({
    name: "NAnchorLink",
    inheritAttrs: false,
    props: {
      href: { type: String, default: "" },
      title: { type: String, default: "" },
    },
    emits: ["click"],
    setup(props, { slots, emit, attrs }) {
      return () =>
        h(
          "a",
          {
            ...attrs,
            href: props.href,
            onClick: (event: Event) => {
              event.preventDefault();
              emit("click", event);
            },
          },
          props.title || slots.default?.(),
        );
    },
  });

  return { __esModule: true, NAnchor, NAnchorLink };
});

vi.mock("motion-v", () => {
  const Passthrough = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: true,
      props: {
        as: { type: String, default: "div" },
        animate: { type: [Object, String, Boolean, Array], default: undefined },
        initial: { type: [Object, String, Boolean, Array], default: undefined },
        transition: { type: [Object, String, Array], default: undefined },
        ignoreStrict: { type: Boolean, default: false },
        forwardMotionProps: { type: Boolean, default: false },
      },
      setup(_, { slots }) {
        // 轨道里只用到 SVG 元素，渲染成 <g> 让 DOM 结构合法。
        return () => h("g", slots.default?.());
      },
    });

  return { __esModule: true, Motion: Passthrough("Motion") };
});

const headings: ArticleHeadingAnchor[] = [
  { id: "h-a", href: "#h-a", title: "第一节", level: 2 },
  { id: "h-b", href: "#h-b", title: "第二节", level: 3 },
  { id: "h-c", href: "#h-c", title: "第三节", level: 2 },
];

function mountToc(
  options: {
    headings?: ArticleHeadingAnchor[];
    activeHeadingId?: string;
    progressPercent?: number;
  } = {},
) {
  return mount(ArticleTocAnchor, {
    props: {
      headings: options.headings ?? headings,
      activeHeadingId: options.activeHeadingId ?? "h-b",
      progressPercent: options.progressPercent ?? 42,
    },
  });
}

describe("ArticleTocAnchor：轨道叠在 naive 锚点之上", () => {
  it("每个标题一个节点，当前标题实心（is-current），其余不是", () => {
    const wrapper = mountToc();

    const nodes = wrapper.findAll("[data-node-id]");
    expect(nodes.map((node) => node.attributes("data-node-id"))).toEqual(["h-a", "h-b", "h-c"]);

    const current = wrapper.findAll("[data-node-id].is-current");
    expect(current).toHaveLength(1);
    expect(current[0].attributes("data-node-id")).toBe("h-b");
  });

  it("行为层仍是 naive 锚点：bound=96、show-rail 关闭、链接仍是可点的锚", () => {
    const wrapper = mountToc();

    expect(wrapper.get(".n-anchor").attributes("data-bound")).toBe("96");
    expect(wrapper.findAll("a[data-heading-id]")).toHaveLength(3);
    expect(wrapper.get("nav[aria-label='文章目录']").exists()).toBe(true);
  });

  it("轨道是虚线描边 + 已读段实线填充：两条同一条 d 的路径，填充按比例截断", () => {
    const wrapper = mountToc();

    const track = wrapper.get(".article-toc-rail__track");
    const fill = wrapper.get(".article-toc-rail__fill");

    expect(track.attributes("d")).toBeTruthy();
    expect(fill.attributes("d")).toBe(track.attributes("d"));
    expect(track.attributes("stroke-dasharray")).toBe("2 5");
  });

  it("飞机停当前节点上，朝向贴轨道切线", () => {
    const wrapper = mountToc();

    const plane = wrapper.get("[data-plane-for]");
    expect(plane.attributes("data-plane-for")).toBe("h-b");

    const node = wrapper.get('[data-node-id="h-b"]');
    expect(plane.attributes("transform")).toContain(
      `translate(${node.attributes("cx")} ${node.attributes("cy")})`,
    );
    expect(plane.attributes("transform")).toMatch(/rotate\([-\d.]+\)/u);
  });

  it("进度条与百分比都还在（与轨道填充并存的刻意双表达）", () => {
    const wrapper = mountToc({ progressPercent: 42 });

    expect(wrapper.get("[role='progressbar']").attributes("aria-valuenow")).toBe("42");
    expect(wrapper.get(".article-reading-progress__fill").attributes("style")).toContain("42%");
    expect(wrapper.text()).toContain("42%");
  });

  it("<1024px 整块目录不渲染（沿用既有 tocVisible 判定）", async () => {
    const originalWidth = window.innerWidth;
    window.innerWidth = 800;

    try {
      const wrapper = mountToc();
      // 目录可见性是挂载后按视口重算的，等一次渲染。
      await flushPromises();

      expect(wrapper.find("[data-toc-rail]").exists()).toBe(false);
      expect(wrapper.find("nav[aria-label='文章目录']").exists()).toBe(false);
    } finally {
      window.innerWidth = originalWidth;
    }
  });
});

describe("ArticleTocAnchor：点击钉住，读者自己滚才恢复跟随", () => {
  it("点标题抛 jump 并钉住：高亮跳到别处，飞机也不动", async () => {
    const wrapper = mountToc({ activeHeadingId: "h-a" });

    await wrapper.get('[data-heading-id="h-c"]').trigger("click");

    expect(wrapper.emitted("jump")?.at(-1)).toEqual(["h-c"]);
    expect(wrapper.get("section").attributes("data-toc-pinned")).toBe("true");
    expect(wrapper.get("[data-plane-for]").attributes("data-plane-for")).toBe("h-c");

    // 平滑滚动的途中高亮会一路变，但钉住期间飞机不该跟着跑。
    await wrapper.setProps({ activeHeadingId: "h-b" });
    await flushPromises();
    expect(wrapper.get("[data-plane-for]").attributes("data-plane-for")).toBe("h-c");
  });

  it("读者自己滚（wheel）后解除钉住，飞机回到滚动高亮的节点", async () => {
    const wrapper = mountToc({ activeHeadingId: "h-a" });
    await wrapper.get('[data-heading-id="h-c"]').trigger("click");

    window.dispatchEvent(new Event("wheel"));
    await flushPromises();

    expect(wrapper.get("section").attributes("data-toc-pinned")).toBe("false");
    expect(wrapper.get("[data-plane-for]").attributes("data-plane-for")).toBe("h-a");
  });

  it("键盘滚（PageDown）也算读者自己滚", async () => {
    const wrapper = mountToc({ activeHeadingId: "h-a" });
    await wrapper.get('[data-heading-id="h-b"]').trigger("click");

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "PageDown" }));
    await flushPromises();

    expect(wrapper.get("section").attributes("data-toc-pinned")).toBe("false");
    expect(wrapper.get("[data-plane-for]").attributes("data-plane-for")).toBe("h-a");
  });

  it("钉住目标不在标题表里（换文章）时不会把飞机钉在虚空里", async () => {
    const wrapper = mountToc({ activeHeadingId: "h-a" });
    await wrapper.get('[data-heading-id="h-c"]').trigger("click");

    await wrapper.setProps({
      headings: [
        { id: "x-1", href: "#x-1", title: "新文章第一节", level: 2 },
        { id: "x-2", href: "#x-2", title: "新文章第二节", level: 2 },
      ],
      activeHeadingId: "x-1",
    });
    await flushPromises();

    expect(wrapper.get("[data-plane-for]").attributes("data-plane-for")).toBe("x-1");
    expect(wrapper.findAll("[data-node-id]")).toHaveLength(2);
  });
});

describe("ArticleTocAnchor：减少动态", () => {
  it("正常偏好下轨道走弹簧（data-rail-motion=spring）", () => {
    const wrapper = mountToc();

    expect(wrapper.get("[data-toc-rail]").attributes("data-rail-motion")).toBe("spring");
  });

  it("减少动态下飞机与填充直接落位（data-rail-motion=jump）", () => {
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = ((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia;

    try {
      const wrapper = mountToc();

      expect(wrapper.get("[data-toc-rail]").attributes("data-rail-motion")).toBe("jump");
    } finally {
      window.matchMedia = originalMatchMedia;
    }
  });
});

/**
 * 视觉层只叠不改（票 296 的行为层边界）。
 *
 * 曾经的写法是 `:deep(.article-toc-link) { border-left-color: transparent }`：
 * 轨道一上来就把锚点「当前项左竖条」抹掉了，等于视觉层改写了行为层的既有视觉。
 * 这里读源码钉住——组件里不得再有针对 `.article-toc-link` 的 border-left 覆盖，
 * 而那条竖条本身仍在样式表里（删的是覆盖，不是竖条）。
 */
describe("ArticleTocAnchor：视觉层不覆盖行为层的既有视觉", () => {
  const projectDir = process.cwd();
  const source = stripComments(
    readFileSync(join(projectDir, "app/components/article/ArticleTocAnchor.vue"), "utf8"),
  );
  const css = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");

  it("组件源码里不再出现针对 .article-toc-link 的 border-left 覆盖", () => {
    const rules = source.match(/\.article-toc-link[^{}]*\{[^{}]*\}/gu) ?? [];

    expect(rules.filter((rule) => /border-left/u.test(rule))).toEqual([]);
    expect(source).not.toMatch(/\.article-toc-link[^{]*\{[^}]*border-left/u);
  });

  it("锚点自己的当前项左竖条仍在样式表里（让位共存，而不是抹掉）", () => {
    expect(blockAfter(css, ".article-toc-link.is-active {")).toContain("border-left-color");
  });

  it("轨道视觉层还在：靠右移锚点让位，轨道本身不缩水", () => {
    expect(source).toContain(".article-toc__rail");
    expect(blockAfter(source, ".article-toc__plot :deep(.n-anchor) {")).toContain("margin-left");
  });
});
