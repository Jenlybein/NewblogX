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

describe("BannerCarousel：首屏封面媒体层（底图 + 罩层 + 降级态）", () => {
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

  it("只有 1 张时不渲染控件（没有可轮播的内容就不该出现箭头/圆点）", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners.slice(0, 1) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.findAll(".banner-dot")).toHaveLength(0);
    expect(wrapper.find(".banner-controls").exists()).toBe(false);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");
  });

  it("≥2 张才渲染玻璃胶囊控件（箭头 + 圆点在同一层薄玻璃里）", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners.slice(0, 2) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.findAll(".banner-dot")).toHaveLength(2);
    expect(wrapper.findAll(".banner-arrow")).toHaveLength(2);
    expect(wrapper.get(".banner-controls").classes()).toContain("glass-thin");
  });

  it("取景与视差分属两层：视差在外层容器，缩放与焦点在图片（两个 transform 不互相覆盖）", () => {
    const wrapper = mount(BannerCarousel, {
      props: {
        banners: [
          { ...banners[0], crop: { focus_x: 0.3, focus_y: 0.4, zoom: 1.5 } },
        ] as never,
        photoStyle: { transform: "translateY(6px)" },
      },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    const parallaxLayer = wrapper.get(".banner-photo").attributes("style") ?? "";
    const framingLayer = wrapper.get(".banner-photo__img").attributes("style") ?? "";

    // 视差（首屏编排传进来的 photoStyle）只在外层，缩放/焦点只在图片层。
    expect(parallaxLayer).toContain("translateY(6px)");
    expect(parallaxLayer).not.toContain("scale(1.5)");
    expect(framingLayer).toContain("scale(1.5)");
    expect(framingLayer).toContain("transform-origin");
    expect(framingLayer).not.toContain("translateY(6px)");
  });

  it("没有取景记录的历史图：图片层走居中、缩放 1（与引入取景前一致）", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners.slice(0, 1) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    const framingLayer = wrapper.get(".banner-photo__img").attributes("style") ?? "";

    expect(framingLayer).toContain("object-fit: cover");
    expect(framingLayer).toContain("transform-origin: 50% 50%");
    expect(framingLayer).toContain("transform: scale(1)");
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
