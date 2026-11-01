import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";
import MarkdownRenderSurface from "~/components/common/MarkdownRenderSurface.vue";

const exposedHeadingElement = document.createElement("h2");
exposedHeadingElement.id = "section-a";

vi.mock("~/components/studio/MarkdownShadowPreview.vue", () => ({
  default: defineComponent({
    name: "StudioMarkdownShadowPreviewStub",
    props: {
      html: {
        type: String,
        default: "",
      },
      themeHref: {
        type: String,
        default: "",
      },
      articleClass: {
        type: String,
        default: "",
      },
    },
    setup(props, { expose }) {
      expose({
        scrollToHeading: (id: string) => id === "section-a",
        getHeadingElement: (id: string) => (id === "section-a" ? exposedHeadingElement : null),
      });

      return () =>
        h("div", {
          "data-test": "shadow-preview-stub",
          "data-html": props.html,
          "data-theme-href": props.themeHref,
          "data-article-class": props.articleClass,
        });
    },
  }),
}));

describe("MarkdownRenderSurface", () => {
  it("renders markdown into the preview child and emits heading updates", async () => {
    const wrapper = mount(MarkdownRenderSurface, {
      props: {
        source: "# 标题\n\n正文内容",
      },
    });

    const preview = wrapper.get('[data-test="shadow-preview-stub"]');
    const headingEvents = wrapper.emitted("headings-change");

    // 阅读页不传外部主题：自有 token 主题由 shadow 组件内置。
    expect(preview.attributes("data-theme-href")).toBe("");
    expect(preview.attributes("data-article-class")).toBe("markdown-body");
    expect(preview.attributes("data-html")).toContain("<h1");
    expect(preview.attributes("data-html")).toContain("正文内容");
    expect(headingEvents?.[0]?.[0]).toEqual([
      {
        id: "标题",
        href: "#标题",
        title: "标题",
        level: 1,
      },
    ]);
  });

  it("passes an optional external theme through for legacy previews", () => {
    const wrapper = mount(MarkdownRenderSurface, {
      props: {
        source: "## Section A",
        themeHref: "/themes/github.css",
      },
    });

    expect(wrapper.get('[data-test="shadow-preview-stub"]').attributes("data-theme-href")).toBe(
      "/themes/github.css",
    );
  });

  it("proxies heading navigation methods from the preview child", () => {
    const wrapper = mount(MarkdownRenderSurface, {
      props: {
        source: "## Section A",
      },
    });

    const viewModel = wrapper.vm as unknown as {
      scrollToHeading: (id: string) => boolean;
      getHeadingElement: (id: string) => HTMLElement | null;
    };

    expect(viewModel.scrollToHeading("section-a")).toBe(true);
    expect(viewModel.scrollToHeading("missing")).toBe(false);
    expect(viewModel.getHeadingElement("section-a")?.id).toBe("section-a");
    expect(viewModel.getHeadingElement("missing")).toBeNull();
  });

  // 读者站文章页与后台编辑器预览都经这个组件，所以这里的断言同时代表两端（#137）。
  it("passes inline SVG through as real markup, not escaped text", () => {
    const figure = [
      '<div class="svg-block">',
      '<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>',
      "<center>图 1 说明</center>",
      "</div>",
    ].join("\n");

    const wrapper = mount(MarkdownRenderSurface, {
      props: { source: figure },
    });

    const html = wrapper.get('[data-test="shadow-preview-stub"]').attributes("data-html");

    expect(html).toContain("<svg");
    expect(html).not.toContain("&lt;svg");
    expect(html).toContain("<rect");
    expect(html).toContain("<center>");
    expect(html).toContain('class="svg-block"');
  });

  it("keeps width-relative sizing so figures shrink on narrow screens", () => {
    // 窄屏自适应依赖两项：viewBox（保持纵横比）+ 相对宽度。
    // 两者都在放行清单里，不能被消毒顺手清掉。
    const figure =
      '<svg viewBox="0 0 820 502" style="width:100%;max-width:760px;height:auto;display:block;margin:18px auto">' +
      '<rect width="820" height="502" fill="#fbfcfd"/></svg>';

    const wrapper = mount(MarkdownRenderSurface, {
      props: { source: figure },
    });

    const html = wrapper.get('[data-test="shadow-preview-stub"]').attributes("data-html");

    // DOMPurify 会把 SVG 属性名规范化回 camelCase，所以这里看到的是 viewBox。
    expect(html).toContain("viewBox");
    expect(html).toContain("width");
  });
});
