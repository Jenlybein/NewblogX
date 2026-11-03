import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticleCover from "~/components/article/ArticleCover.vue";

/**
 * 读者侧封面（#204）：固定居中，不再消费取景参数。
 *
 * 封面形状与「不拉伸、不留白」由 CSS 门禁锁（test/design/article-cover-shape.test.ts）；
 * 这里验证组件行为：图片层不带任何取景行内样式、没图/加载失败都落到空封面兜底。
 */

describe("ArticleCover：固定居中渲染", () => {
  it("有封面时渲染图片：src/alt 正确，且不带取景行内样式", () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/cover.png", title: "文章" },
    });

    const image = wrapper.get(".article-cover-image");
    expect(image.attributes("src")).toBe("https://image.example.com/cover.png");
    expect(image.attributes("alt")).toBe("文章");
    // 固定居中由 CSS 承担（object-fit: cover / object-position: center），
    // 组件不再把焦点与缩放贴到图片层。
    expect(image.attributes("style")).toBeUndefined();
  });

  it("没有封面地址时渲染空封面兜底，不出现空白或 img", () => {
    const wrapper = mount(ArticleCover, { props: { src: null, title: "文章" } });

    expect(wrapper.find(".article-cover-image").exists()).toBe(false);
    const fallback = wrapper.get(".article-cover-default");
    expect(fallback.attributes("aria-label")).toBe("文章暂无封面");
  });

  it("图片加载失败时切到空封面兜底（不留下浏览器碎图标）", async () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/broken.png", title: "文章" },
    });

    await wrapper.get(".article-cover-image").trigger("error");

    expect(wrapper.find(".article-cover-image").exists()).toBe(false);
    expect(wrapper.get(".article-cover-default").exists()).toBe(true);
  });

  it("换一张封面时失败态复位（上一张的失败不带到下一张）", async () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/broken.png", title: "文章" },
    });
    await wrapper.get(".article-cover-image").trigger("error");

    await wrapper.setProps({ src: "https://image.example.com/fresh.png" });

    expect(wrapper.get(".article-cover-image").attributes("src")).toBe("https://image.example.com/fresh.png");
  });
});
