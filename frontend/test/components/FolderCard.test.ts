import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import FolderCard from "~/components/common/FolderCard.vue";
import FolderCardList from "~/components/common/FolderCardList.vue";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 合集/项目文件夹卡的行为（grill 记录 §三 3.6、票 293）。
 *
 * 断的都是**外部可观察**的东西：点击去哪、空封面长什么样、能不能聚焦、
 * 组件里有没有裸色值；像素级视觉不在这里（仓库不做截图回归，见 ADR-0012）。
 */
const projectDir = process.cwd();

/** NuxtLink 的替身：渲染成真 `<a href>`，链接语义（可聚焦 / 可复制 / 中键）才算真验到。 */
const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    return () => h("a", { href: typeof props.to === "string" ? props.to : "" }, slots.default?.());
  },
});

const collectionCard = {
  id: "c1",
  href: "/collections/c1",
  title: "Nuxt 部署实战",
  description: "从零把 Nuxt 应用部署到自建服务器。",
  cover: "https://cdn.example.com/c1.png",
  countLabel: "6 篇文章",
};

const projectCard = {
  ...collectionCard,
  id: "p1",
  href: "/projects/p1",
  title: "果冻盒",
  cover: "",
  countLabel: "暂无文章",
};

function mountCard(props: Record<string, unknown> = {}, attachTo?: HTMLElement) {
  return mount(FolderCard, {
    props: { href: "/collections/c1", title: "Nuxt 部署实战", ...props },
    ...(attachTo ? { attachTo } : {}),
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

function mountList(cards: unknown[] = [collectionCard, projectCard]) {
  return mount(FolderCardList, {
    props: { cards },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

describe("文件夹卡：点击就是导航（一次点击进详情，不做「先炸开后进详情」）", () => {
  it("卡片的可点区域就是指向详情页的链接", () => {
    const link = mountCard().get(".folder-card__link");

    expect(link.element.tagName).toBe("A");
    expect(link.attributes("href")).toBe("/collections/c1");
  });

  it("列表里的每张卡各是一条自己的链接（合集与项目两条路由各自正确）", () => {
    const links = mountList()
      .findAll(".folder-card__link")
      .map((link) => link.attributes("href"));

    expect(links).toEqual(["/collections/c1", "/projects/p1"]);
  });

  it("标题、两行描述与篇数留在文件夹下方（不叠到前盖上）", () => {
    const wrapper = mountCard({ description: "从零部署", meta: "6 篇文章" });

    expect(wrapper.get(".folder-card__title").text()).toBe("Nuxt 部署实战");
    expect(wrapper.get(".folder-card__description").text()).toBe("从零部署");
    expect(wrapper.get(".folder-card__meta").text()).toBe("6 篇文章");
    expect(wrapper.get(".folder-card__flap").text()).toBe("");
  });
});

describe("文件夹卡：封面", () => {
  it("有封面时把 src 原样交给共用封面组件（16:9 由它撑）", () => {
    const img = mountCard({ cover: collectionCard.cover }).get(".folder-card__sheet img");

    expect(img.attributes("src")).toBe(collectionCard.cover);
    expect(img.attributes("style")).toBeUndefined();
  });

  it("空封面不渲染破图：退到共用兜底占位（对辅助技术隐藏、无文字）", () => {
    const wrapper = mountCard({ cover: "" });
    const sheet = wrapper.get(".folder-card__sheet");

    expect(sheet.find("img").exists()).toBe(false);
    const placeholder = sheet.get(".article-cover-default");
    expect(placeholder.attributes("aria-hidden")).toBe("true");
    expect(placeholder.text()).toBe("");
  });

  it("封面地址只有空白时也按空封面处理", () => {
    const wrapper = mountCard({ cover: "   " });

    expect(wrapper.find(".folder-card__sheet img").exists()).toBe(false);
  });
});

describe("文件夹卡：可聚焦", () => {
  it("链接能真正拿到焦点（不是 tabindex 伪装出来的）", () => {
    const wrapper = mountCard({}, document.body);
    const link = wrapper.get(".folder-card__link");

    (link.element as HTMLElement).focus();

    expect(document.activeElement).toBe(link.element);
    expect(link.attributes("tabindex")).toBeUndefined();
    wrapper.unmount();
  });
});

describe("文件夹卡：配色只来自语义 token（组件源内零裸色值）", () => {
  const sources = ["app/components/common/FolderCard.vue", "app/components/common/FolderCardList.vue"];

  it.each(sources)("%s 里没有裸十六进制 / rgb() / hsl()", (relativePath) => {
    const source = readFileSync(join(projectDir, relativePath), "utf8");

    expect(findColorLiterals(source)).toEqual([]);
  });
});

/**
 * 前盖玻璃的降级写法（ADR-0012 约束 7 + 2026-11-07 后续修订：文件夹 flap 是
 * 内容面上唯一的玻璃例外）。这几条只能读源码钉——降级是「另一种浏览器里的表现」，
 * happy-dom 不做 layout、更不认 `@supports`。
 */
describe("文件夹卡：前盖玻璃的降级与窄屏", () => {
  const source = readFileSync(join(projectDir, "app/components/common/FolderCard.vue"), "utf8");

  it("前盖常开玻璃（backdrop-filter），填充取玻璃 token", () => {
    expect(source).toContain("backdrop-filter:");
    expect(source).toContain("var(--glass-thin-fill)");
  });

  it("不支持 backdrop-filter 时退化为不透明面", () => {
    expect(source).toContain("@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)))");
    expect(source).toMatch(/@supports not[\s\S]*?background: var\(--color-surface-raised\)/u);
  });

  it("系统「减少透明度」时同样退化为不透明面", () => {
    expect(source).toMatch(
      /@media \(prefers-reduced-transparency: reduce\)[\s\S]*?backdrop-filter: none/u,
    );
  });

  it("窄屏（≤768px）模糊走 --glass-blur-mobile（12px）", () => {
    expect(source).toMatch(/@media \(max-width: 768px\)[\s\S]*?blur\(var\(--glass-blur-mobile\)\)/u);
  });

  it("减少动态：位移与旋转归零，只保留静态形态（且不吃弹簧）", () => {
    expect(source).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?transform: none/u,
    );
    expect(source).not.toMatch(/stiffness|damping|type:\s*"spring"/u);
  });

  it("位移走 200ms 的 CSS 过渡 + 唯一缓动 token", () => {
    expect(source).toContain("transition:");
    expect(source).toContain("var(--motion-base) var(--motion-ease)");
  });
});

/**
 * 「两页各自的重复卡片样式被移除」是本票的一半价值（票 293）。
 * 这条读两个列表页的源码钉住收敛结果：卡片样式只剩一份，且都来自共用组件。
 */
describe("两页共用同一个文件夹卡：各自的卡片样式已删除", () => {
  const pages = ["app/pages/collections/index.vue", "app/pages/projects/index.vue"];

  it.each(pages)("%s 用共用组件渲染列表", (relativePath) => {
    const source = readFileSync(join(projectDir, relativePath), "utf8");

    expect(source).toContain('from "~/components/common/FolderCardList.vue"');
    expect(source).toContain("<FolderCardList");
  });

  it.each(pages)("%s 不再自带卡片样式（网格 / 封面 / 标题 / 描述 / 篇数）", (relativePath) => {
    const source = readFileSync(join(projectDir, relativePath), "utf8");

    expect(source).not.toMatch(/collection-card|project-card/u);
    expect(source).not.toMatch(/border-radius/u);
    expect(source).not.toMatch(/aspect-ratio/u);
    expect(source).not.toMatch(/backdrop-filter/u);
  });
});
