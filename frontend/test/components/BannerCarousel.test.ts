import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

describe("BannerCarousel：首屏封面媒体层（底图 + 底部渐黑 + 降级态）", () => {
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

  it("≥2 张才渲染控件，且控件住在底部渐黑区里（不再是一颗玻璃胶囊）", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners.slice(0, 2) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.findAll(".banner-dot")).toHaveLength(2);
    expect(wrapper.findAll(".banner-arrow")).toHaveLength(2);
    // 控件是渐黑区的内容，不是并列的浮层。
    expect(wrapper.get(".banner-veil").find(".banner-controls").exists()).toBe(true);
    // 控件不再单独铺一块玻璃底：它就是渐黑区里的一行。
    expect(wrapper.get(".banner-controls").classes()).not.toContain("glass-thin");
  });

  it("图片加载失败切到失败态，切到下一张时恢复", async () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: banners.slice(0, 2) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    await wrapper.get(".banner-photo__img").trigger("error");
    expect(wrapper.get(".banner-photo").classes()).toContain("is-failed");

    await wrapper.findAll(".banner-dot")[1]?.trigger("click");
    expect(wrapper.get(".banner-photo").classes()).not.toContain("is-failed");
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

describe("BannerCarousel：轮播标题（纯文本、可空）", () => {
  const titled = [
    { id: "t1", cover: "https://image.example.com/t1.png", title: "春季上新" },
    { id: "t2", cover: "https://image.example.com/t2.png", title: "" },
    { id: "t3", cover: "https://image.example.com/t3.png", title: "第三张的标题" },
  ];

  it("有标题时在渐黑区里把标题渲染出来", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: titled.slice(0, 1) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    const title = wrapper.get(".banner-title");
    expect(title.text()).toBe("春季上新");
    // 标题与控件同住底部渐黑区：它是渐黑区的内容，不是叠在图上另起的一层。
    expect(wrapper.get(".banner-veil").find(".banner-title").exists()).toBe(true);
  });

  it("没有标题（空串）时不渲染文字区，不留一块空标题带", () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: titled.slice(1, 2) as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.find(".banner-title").exists()).toBe(false);
  });

  it("切换轮播时标题跟着换；切到无标题的一张就不渲染文字区", async () => {
    const wrapper = mount(BannerCarousel, {
      props: { banners: titled as never },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });
    expect(wrapper.get(".banner-title").text()).toBe("春季上新");

    await wrapper.findAll(".banner-dot")[1]?.trigger("click");
    expect(wrapper.find(".banner-title").exists()).toBe(false);

    await wrapper.findAll(".banner-dot")[2]?.trigger("click");
    expect(wrapper.get(".banner-title").text()).toBe("第三张的标题");
  });

  it("标题里含 HTML 标签时按纯文本显示（不被解析成 DOM）", () => {
    const wrapper = mount(BannerCarousel, {
      props: {
        banners: [{ id: "x", cover: "https://image.example.com/x.png", title: "<b>粗体</b> & <i>斜体</i>" }] as never,
      },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    const title = wrapper.get(".banner-title");
    expect(title.text()).toBe("<b>粗体</b> & <i>斜体</i>");
    // 标签没有被解析成元素：文字区里没有任何 b/i 子节点。
    expect(title.find("b").exists()).toBe(false);
    expect(title.find("i").exists()).toBe(false);
  });
});

/**
 * 自动轮播。
 *
 * 用假定时器把"等 6 秒"压成一次推进；判据只看"图有没有换"，
 * 不依赖实现里的具体 setInterval 句柄。
 */
describe("BannerCarousel：自动轮播", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  function mountWith(interval: number, list = banners) {
    return mount(BannerCarousel, {
      props: { banners: list as never, autoPlayInterval: interval },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });
  }

  it("到点自动切到下一张，并且循环回第一张", async () => {
    const wrapper = mountWith(6000);

    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");
    await vi.advanceTimersByTimeAsync(6000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b2.png");

    await vi.advanceTimersByTimeAsync(12000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");

    wrapper.unmount();
  });

  it("只有一张时不自动轮播", async () => {
    const wrapper = mountWith(6000, banners.slice(0, 1));

    await vi.advanceTimersByTimeAsync(30000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");

    wrapper.unmount();
  });

  it("指针停在上面时暂停，移开后恢复", async () => {
    const wrapper = mountWith(6000);
    const root = wrapper.get(".banner-stage");

    await root.trigger("mouseenter");
    await vi.advanceTimersByTimeAsync(20000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");

    await root.trigger("mouseleave");
    await vi.advanceTimersByTimeAsync(6000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b2.png");

    wrapper.unmount();
  });

  it("手动点过圆点后重新计时：不会紧接着又被自动切走", async () => {
    const wrapper = mountWith(6000);

    await vi.advanceTimersByTimeAsync(5000);
    await wrapper.findAll(".banner-dot")[2]?.trigger("click");
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b3.png");

    // 只过 2 秒（原本 5+2 会到 7 秒触发），不应被切走。
    await vi.advanceTimersByTimeAsync(2000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b3.png");

    wrapper.unmount();
  });

  it("间隔 ≤ 0 时关闭自动轮播（显式静态展示）", async () => {
    const wrapper = mountWith(0);

    await vi.advanceTimersByTimeAsync(30000);
    expect(wrapper.get(".banner-stage img").attributes("src")).toContain("b1.png");

    wrapper.unmount();
  });
});