import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import BannerCarousel from "~/components/article/BannerCarousel.vue";

const NuxtLinkStub = defineComponent({
  props: { to: { default: "" } },
  setup: (_props, { slots }) => () => h("a", slots.default?.()),
});

const banners = [
  { id: "b1", cover: "https://image.example.com/b1.png", abstract: "第一张" },
  { id: "b2", cover: "https://image.example.com/b2.png", abstract: "第二张" },
  { id: "b3", cover: "https://image.example.com/b3.png", abstract: "第三张" },
];

describe("BannerCarousel：首屏轮播（方角图纸 + 降级态）", () => {
  it("有轮播时渲染第一张，切换点有 44px 触控区与 aria 文案", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");
    expect(wrapper.get(".banner-stage img").attributes("alt")).toContain("首页轮播图 1");
    expect(wrapper.findAll(".banner-dot")).toHaveLength(3);
    expect(wrapper.get(".banner-dot").attributes("aria-label")).toContain("第 1 张");
    expect(wrapper.get(".banner-dot.is-active").attributes("aria-current")).toBe("true");
  });

  it("点击第 3 个点切换图片", async () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    await wrapper.findAll(".banner-dot")[2]?.trigger("click");
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b3.png");
  });

  it("空轮播：降级为统一空态（不是空白/报错）", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: [] },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.get(".empty-state").text()).toContain("暂无轮播内容");
    expect(wrapper.find(".banner-stage").exists()).toBe(false);
  });

  it("异步加载中：空态标记 pending", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: [], pending: true },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.get("[data-empty-state='true']").attributes("data-state")).toBe("pending");
  });
});
