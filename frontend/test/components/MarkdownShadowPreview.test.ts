import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

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
});
