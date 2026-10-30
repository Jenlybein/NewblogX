import { readFileSync } from "node:fs";
import { join } from "node:path";
import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { resolveMermaidTheme } from "~/utils/markdownTheme";

const tokensCss = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

const runtimeInitialize = vi.fn();
const runtimeRender = vi.fn();

vi.mock("~/services/mermaid-runtime.mjs", () => ({
  default: {
    initialize: runtimeInitialize,
    render: runtimeRender,
  },
}));

async function loadComponent() {
  vi.resetModules();
  const module = await import("~/components/studio/MarkdownShadowPreview.vue");
  return module.default;
}

function getShadowRoot(wrapper: ReturnType<typeof mount>) {
  return (wrapper.element as HTMLDivElement).shadowRoot;
}

function getShadowArticle(wrapper: ReturnType<typeof mount>) {
  return getShadowRoot(wrapper)?.querySelector("article");
}

function getThemeStyle(wrapper: ReturnType<typeof mount>) {
  return getShadowRoot(wrapper)?.querySelector<HTMLStyleElement>('style[data-jelly-markdown="theme"]');
}

describe("MarkdownShadowPreview", () => {
  beforeEach(() => {
    runtimeInitialize.mockReset();
    runtimeRender.mockReset();
  });

  it("renders plain html without loading the mermaid runtime", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, {
      props: {
        html: "<p>纯文本内容</p>",
      },
    });

    await flushPromises();

    expect(runtimeRender).not.toHaveBeenCalled();
    expect(getShadowArticle(wrapper)?.innerHTML).toContain("纯文本内容");
  });

  it("loads the mermaid runtime only for mermaid blocks and reuses cached svg output", async () => {
    const bindFunctions = vi.fn();
    runtimeRender.mockResolvedValue({
      svg: '<svg data-test="mermaid-diagram"><text>Flow</text></svg>',
      bindFunctions,
    });

    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, {
      props: {
        html: '<div class="mermaid">flowchart TD\nA[开始] --> B[结束]</div>',
      },
    });

    await flushPromises();

    expect(runtimeInitialize).toHaveBeenCalledTimes(1);
    expect(runtimeRender).toHaveBeenCalledTimes(1);
    expect(runtimeRender.mock.calls[0]?.[1]).toContain("flowchart TD");
    expect(bindFunctions).toHaveBeenCalledTimes(1);
    expect(getShadowArticle(wrapper)?.innerHTML).toContain('data-test="mermaid-diagram"');

    await wrapper.setProps({
      html: '<div class="mermaid">flowchart TD\nA[开始] --> B[结束]</div>',
    });
    await flushPromises();

    expect(runtimeRender).toHaveBeenCalledTimes(1);
  });

  it("注入自有 markdown 主题（token 驱动），不再依赖 github-markdown-css", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, { props: { html: "<p>正文</p>" } });

    await flushPromises();

    const style = getThemeStyle(wrapper);
    expect(style).toBeTruthy();
    expect(style?.textContent).toContain("markdown 主题（JellyBin 自有）");
    expect(style?.textContent).toContain(".markdown-body pre");
    expect(style?.textContent).toContain("var(--color-ink)");
    expect(style?.textContent).toContain("var(--type-leading-prose)");
  });

  it("默认不加载任何外部主题 <link>（明暗切换不换链接、不闪白）", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, { props: { html: "<p>正文</p>" } });

    await flushPromises();

    expect(getShadowRoot(wrapper)?.querySelectorAll('link[rel="stylesheet"]').length).toBe(0);
  });

  it("显式传入 themeHref 时才注入外部主题（兼容创作区多主题预览）", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, {
      props: { html: "<p>正文</p>", themeHref: "/themes/github.css" },
    });

    await flushPromises();

    const link = getShadowRoot(wrapper)?.querySelector<HTMLLinkElement>('link[data-jelly-markdown="external-theme"]');
    expect(link?.getAttribute("href")).toBe("/themes/github.css");

    await wrapper.setProps({ themeHref: "" });
    await flushPromises();

    expect(getShadowRoot(wrapper)?.querySelector('link[data-jelly-markdown="external-theme"]')).toBeNull();
  });

  it("样式不泄漏：宿主 light DOM 为空，主题只在 shadow root 里", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, { props: { html: "<p>正文</p>", articleClass: "markdown-body" } });

    await flushPromises();

    expect((wrapper.element as HTMLDivElement).innerHTML).toBe("");
    expect(document.querySelector("style[data-jelly-markdown]")).toBeNull();
    expect(getShadowRoot(wrapper)?.querySelectorAll("article.markdown-body").length).toBe(1);
  });

  // ADR-0012 决策 #11：图表配色也走 token。这条链路上最容易断的环是"组件有没有真的
  // 把 themeVariables 传下去" —— 上面的单测只覆盖 buildMermaidConfig，这里补连线。
  it("mermaid 初始化带 token 派生的 themeVariables（不是第三方默认配色）", async () => {
    const MarkdownShadowPreview = await loadComponent();
    mount(MarkdownShadowPreview, {
      props: {
        html: '<div class="mermaid">flowchart TD\nA --> B</div>',
        tokensCss,
        themeMode: "dark",
      },
    });

    await flushPromises();

    expect(runtimeInitialize).toHaveBeenCalledTimes(1);
    const config = runtimeInitialize.mock.calls[0]?.[0];
    expect(config.theme).toBe("base");
    expect(config.themeVariables).toEqual(resolveMermaidTheme(tokensCss, "dark"));
    // 第三方默认主色（浅紫）—— 留着就说明没接上。
    expect(JSON.stringify(config.themeVariables)).not.toContain("#ECECFF");
  });

  it("明暗切换时重新 initialize 并清掉旧 SVG 缓存（图表配色跟着翻）", async () => {
    const MarkdownShadowPreview = await loadComponent();
    const wrapper = mount(MarkdownShadowPreview, {
      props: {
        html: '<div class="mermaid">flowchart TD\nA --> B</div>',
        tokensCss,
        themeMode: "light",
      },
    });

    await flushPromises();
    const lightConfig = runtimeInitialize.mock.calls[0]?.[0];
    expect(lightConfig.themeVariables).toEqual(resolveMermaidTheme(tokensCss, "light"));
    expect(runtimeRender).toHaveBeenCalledTimes(1);

    await wrapper.setProps({ themeMode: "dark" });
    await flushPromises();

    expect(runtimeInitialize).toHaveBeenCalledTimes(2);
    const darkConfig = runtimeInitialize.mock.calls[1]?.[0];
    expect(darkConfig.themeVariables).toEqual(resolveMermaidTheme(tokensCss, "dark"));
    expect(darkConfig.themeVariables).not.toEqual(lightConfig.themeVariables);
    // 缓存清了才会重渲染：否则图表会一直停在上一个模式的配色上。
    expect(runtimeRender).toHaveBeenCalledTimes(2);
  });

  it("没传 tokensCss 时不假装染色（SSR / 拿不到 CSS 源的调用方）", async () => {
    const MarkdownShadowPreview = await loadComponent();
    mount(MarkdownShadowPreview, {
      props: { html: '<div class="mermaid">flowchart TD\nA --> B</div>' },
    });

    await flushPromises();

    const config = runtimeInitialize.mock.calls[0]?.[0];
    expect(config.startOnLoad).toBe(false);
    expect(config.securityLevel).toBe("strict");
    expect(config.themeVariables).toBeUndefined();
  });
});
