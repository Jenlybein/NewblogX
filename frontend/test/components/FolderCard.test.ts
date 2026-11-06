import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import FolderCard from "~/components/common/FolderCard.vue";
import FolderCardList from "~/components/common/FolderCardList.vue";
import { FOLDER_FLAP_PATH } from "~/utils/folderCardGeometry";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 合集/项目文件夹卡的行为与外形（grill 记录 §三 3.6、票 293）。
 *
 * 断的是**外部可观察**的东西：点击去哪、空封面长什么样、能不能聚焦、组件里有没有
 * 裸色值，以及"前盖是不是那条带凸舌的轮廓"——后者是本票的核心：外形来自路径，
 * 不来自圆角。像素级视觉不在这里（仓库不做截图回归，见 ADR-0012）。
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

const componentSource = readFileSync(
  join(projectDir, "app/components/common/FolderCard.vue"),
  "utf8",
);

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

  it("只有一张封面：三张扇形方案已被否", () => {
    expect(mountCard().findAll(".folder-card__sheet")).toHaveLength(1);
  });
});

/**
 * 外形（本票的核心）：前盖不是"一块圆角玻璃"，是参考组件那条**带凸舌的路径**。
 * 玻璃用同一条路径当遮罩裁形（任意列宽都铺满元素），墨线层再用它描边 + 画内阴影。
 */
describe("文件夹卡：前盖是带凸舌的轮廓（文件夹外形来自路径，不来自圆角）", () => {
  it("墨线层用参考路径逐字描边", () => {
    const edge = mountCard().get(".folder-card__flap-edge");

    expect(edge.attributes("d")).toBe(FOLDER_FLAP_PATH);
    expect(edge.attributes("d")).toContain("L178.08 33.5343");
    expect(edge.attributes("d")).toContain("196.416 41.5404H296");
  });

  it("前盖的 SVG 用 321×241 画布、按元素盒子拉伸（列宽变了形状不变）", () => {
    const svg = mountCard().get(".folder-card__flap-ink");

    expect(svg.attributes("viewBox")).toBe("0 0 321 241");
    expect(svg.attributes("preserveAspectRatio")).toBe("none");
  });

  it("玻璃层用同一条轮廓当遮罩（mask 以自定义属性下发）", () => {
    const stage = mountCard().get(".folder-card__stage").element as HTMLElement;
    const mask = stage.style.getPropertyValue("--folder-flap-mask");

    expect(mask.startsWith("url('data:image/svg+xml,")).toBe(true);
    expect(decodeURIComponent(mask)).toContain(FOLDER_FLAP_PATH);
    expect(componentSource).toContain("mask-image: var(--folder-flap-mask)");
    expect(componentSource).toContain("-webkit-mask-image: var(--folder-flap-mask)");
  });

  it("内阴影走 SVG filter（feOffset + feComposite + feColorMatrix）", () => {
    const wrapper = mountCard();
    const shade = wrapper.get(".folder-card__flap-shade");

    expect(shade.attributes("filter")).toMatch(/^url\(#folder-flap-shade-.+\)$/u);
    expect(componentSource).toContain("feOffset");
    expect(componentSource).toContain('operator="out"');
    expect(componentSource).toContain("feColorMatrix");
  });

  it("内阴影的颜色矩阵里只有数字（换主题不用改这里，也不会撞配色门禁）", () => {
    const matrices = componentSource.match(/values="([^"]*)"/gu) ?? [];

    expect(matrices).toHaveLength(2);
    for (const matrix of matrices) {
      expect(matrix).toMatch(/^values="[0-9.\s]+"$/u);
    }
  });

  it("每张卡的内阴影过滤器各自唯一（否则浏览器只认第一个）", () => {
    const Page = defineComponent({
      render: () =>
        h("ul", [
          h(FolderCard, { href: "/collections/a", title: "A" }),
          h(FolderCard, { href: "/collections/b", title: "B" }),
        ]),
    });
    const ids = mount(Page, { global: { stubs: { NuxtLink: NuxtLinkStub } } })
      .findAll(".folder-card__flap-shade")
      .map((path) => path.attributes("filter"));

    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
  });

  it("层次齐全：背板 / 封面 / 前盖三层都在（前后关系由 CSS 的 z-index 定）", () => {
    const wrapper = mountCard();

    expect(wrapper.find(".folder-card__back").exists()).toBe(true);
    expect(wrapper.find(".folder-card__sheet").exists()).toBe(true);
    expect(wrapper.find(".folder-card__flap").exists()).toBe(true);
    expect(componentSource).toMatch(/\.folder-card__back \{[\s\S]*?z-index: 0/u);
    expect(componentSource).toMatch(/\.folder-card__sheet \{[\s\S]*?z-index: 1/u);
    expect(componentSource).toMatch(/\.folder-card__flap \{[\s\S]*?z-index: 2/u);
  });

  it("前盖以底边为铰链做透视（rotateX 由几何变量给，容器给 perspective）", () => {
    expect(componentSource).toContain("transform-origin: 50% 100%");
    expect(componentSource).toContain("transform: rotateX(var(--folder-flap-tilt))");
    expect(componentSource).toContain("perspective: 900px");
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
  it("玻璃层常开 backdrop-filter，填充取玻璃 token", () => {
    expect(componentSource).toContain("backdrop-filter:");
    expect(componentSource).toContain("var(--glass-thick-fill)");
  });

  it("不支持 backdrop-filter 时退化为不透明面", () => {
    expect(componentSource).toContain("@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)))");
    expect(componentSource).toMatch(/@supports not[\s\S]*?background: var\(--color-surface-raised\)/u);
  });

  it("系统「减少透明度」时同样退化为不透明面", () => {
    expect(componentSource).toMatch(
      /@media \(prefers-reduced-transparency: reduce\)[\s\S]*?backdrop-filter: none/u,
    );
  });

  it("窄屏（≤768px）模糊走 --glass-blur-mobile（12px）", () => {
    expect(componentSource).toMatch(
      /@media \(max-width: 768px\)[\s\S]*?blur\(var\(--glass-blur-mobile\)\)/u,
    );
  });

  it("减少动态：位移与旋转归零，只保留静态形态（且不吃弹簧）", () => {
    expect(componentSource).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?transform: none/u,
    );
    expect(componentSource).not.toMatch(/stiffness|damping|type:\s*"spring"/u);
  });

  it("位移走 200ms 的 CSS 过渡 + 唯一缓动 token", () => {
    expect(componentSource).toContain("transition:");
    expect(componentSource).toContain("var(--motion-base) var(--motion-ease)");
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

/**
 * 卡片视图只该有一份形状定义（Data Clump 的反面）。
 *
 * 合集与项目的卡片视图逐字段相同，`FolderCardList` 曾经又抄了第三份
 * `FolderCardItem`：三份字段表意味着加一个字段要改三处，且漏改不会报错
 * （结构类型只在赋值处比较）。这里钉住"复用既有类型"这件事。
 */
describe("文件夹卡列表：卡片视图复用既有类型", () => {
  const source = readFileSync(join(projectDir, "app/components/common/FolderCardList.vue"), "utf8");

  it("从 utils/collection 取 CollectionCardView（项目卡与之同形）", () => {
    expect(source).toContain('import type { CollectionCardView } from "~/utils/collection"');
    expect(source).toContain("cards: CollectionCardView[]");
  });

  it("不再本地声明同形的 FolderCardItem", () => {
    expect(source).not.toMatch(/FolderCardItem/u);
  });
});
