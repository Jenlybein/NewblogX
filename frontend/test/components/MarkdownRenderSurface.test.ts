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
        required: true,
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
        themeHref: "/themes/github.css",
      },
    });

    const preview = wrapper.get('[data-test="shadow-preview-stub"]');
    const headingEvents = wrapper.emitted("headings-change");

    expect(preview.attributes("data-theme-href")).toBe("/themes/github.css");
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

  it("proxies heading navigation methods from the preview child", () => {
    const wrapper = mount(MarkdownRenderSurface, {
      props: {
        source: "## Section A",
        themeHref: "/themes/github.css",
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
});
