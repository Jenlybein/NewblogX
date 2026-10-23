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

function getShadowArticle(wrapper: ReturnType<typeof mount>) {
  const host = wrapper.element as HTMLDivElement;
  return host.shadowRoot?.querySelector("article");
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
        themeHref: "/themes/github.css",
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
        themeHref: "/themes/github.css",
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
});
