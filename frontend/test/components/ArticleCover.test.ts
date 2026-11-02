import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticleCover from "~/components/article/ArticleCover.vue";

// 读者侧封面的取景复现：
// 封面不裁第二份图，靠「焦点百分比 + 缩放」用 CSS 复现作者确认过的那一块。
// 这里验证组件真的把取景样式贴到了图片层，而不是只在纯函数测试里正确。

describe("ArticleCover：按取景参数复现同一块封面", () => {
  it("有取景参数时 object-position / transform-origin 取焦点、transform 取缩放", () => {
    const wrapper = mount(ArticleCover, {
      props: {
        src: "https://image.example.com/cover.png",
        title: "文章",
        crop: { focus_x: 0.2, focus_y: 0.8, zoom: 1.5 },
      },
    });

    const style = wrapper.get(".article-cover-image").attributes("style") ?? "";
    expect(style).toContain("object-position: 20% 80%");
    expect(style).toContain("transform-origin: 20% 80%");
    expect(style).toContain("transform: scale(1.5)");
    expect(style).toContain("object-fit: cover");
  });

  it("存量文章没有取景参数时按居中、缩放 1 渲染，不报错", () => {
    const wrapper = mount(ArticleCover, {
      props: { src: "https://image.example.com/cover.png", title: "文章" },
    });

    const style = wrapper.get(".article-cover-image").attributes("style") ?? "";
    expect(style).toContain("object-position: 50% 50%");
    expect(style).toContain("transform: scale(1)");
  });
});
