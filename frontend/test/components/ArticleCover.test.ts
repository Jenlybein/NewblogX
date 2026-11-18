import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticleCover from "~/components/article/ArticleCover.vue";

/**
 * 读者侧封面（票 204 / 票 440）：固定居中；**没有封面就什么都不画**。
 *
 * 缺封面 / 加载失败曾经有两种兜底（同底色空白、首字墨片），两种都被否掉了：
 * 用户要的是"没封面就不显示封面框"。所以组件只剩一个出口——有图就渲染图，
 * 没图就没有节点；怎么换排版由消费方决定（见 `useCoverVisibility`）。
 *
 * 封面的形状由 CSS 门禁锁（test/design/article-cover-shape.test.ts）。
 */

describe("ArticleCover：有图才有节点", () => {
  it("有封面时渲染图片：src/alt 正确，且不带取景行内样式", () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/cover.png", title: "文章" },
    });

    const image = wrapper.get(".article-cover-image");
    expect(image.attributes("src")).toBe("https://image.example.com/cover.png");
    expect(image.attributes("alt")).toBe("文章");
    // 固定居中由 CSS 承担（object-fit: cover / object-position: center）。
    expect(image.attributes("style")).toBeUndefined();
  });

  it("没有封面地址时不渲染任何节点（不留空白框、不留首字块）", () => {
    const wrapper = mount(ArticleCover, { props: { src: null, title: "文章" } });

    expect(wrapper.find(".article-cover-image").exists()).toBe(false);
    expect(wrapper.findAll("div, span").length).toBe(0);
    expect(wrapper.text()).toBe("");
    expect(wrapper.find("img").exists()).toBe(false);
  });

  it("封面地址只有空白时也当作没有封面", () => {
    const wrapper = mount(ArticleCover, { props: { src: "   ", title: "文章" } });

    expect(wrapper.find("img").exists()).toBe(false);
  });

  it("图片加载失败时节点退场，并把失败抛给消费方（整个封面槽跟着拿掉）", async () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/broken.png", title: "神经网络" },
    });

    await wrapper.get(".article-cover-image").trigger("error");

    expect(wrapper.find(".article-cover-image").exists()).toBe(false);
    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.emitted("fail")).toHaveLength(1);
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
