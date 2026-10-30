import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import HomePinnedSection from "~/components/home/HomePinnedSection.vue";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots, attrs }) {
    return () =>
      h("a", { ...attrs, href: typeof props.to === "string" ? props.to : undefined }, slots.default?.());
  },
});

const topArticles = [
  {
    id: "a1",
    title: "置顶文章",
    abstract: "置顶摘要",
    category_title: "工程",
    author_nickname: "作者甲",
    view_count: 120,
  },
  { id: "a2", title: "第二篇置顶", abstract: "", view_count: 3 },
];

function mountPinned(props: Record<string, unknown> = {}) {
  return mount(HomePinnedSection, {
    props: { articles: [], pending: false, failed: false, ...props },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

describe("HomePinnedSection：首屏置顶网格", () => {
  it("渲染编号 + 标题 + 摘要 + 出处的三档网格，且可点进文章", () => {
    const wrapper = mountPinned({ articles: topArticles });

    const tiles = wrapper.findAll(".home-pinned-card");
    expect(tiles).toHaveLength(2);
    expect(tiles[0]?.text()).toContain("01");
    expect(tiles[0]?.text()).toContain("置顶文章");
    expect(tiles[0]?.text()).toContain("作者甲");
    expect(tiles[0]?.text()).toContain("120 阅读");
    expect(wrapper.get('[data-role="pinned-badge"]').text()).toBe("2 篇");
    expect(wrapper.findAll(".home-pinned-card__title")[0]?.attributes("href") ?? "").toContain("/article/a1");
  });

  it("空数据：渲染置顶空态并指向搜索页", () => {
    const wrapper = mountPinned({ articles: [] });

    expect(wrapper.find(".home-pinned-card").exists()).toBe(false);
    expect(wrapper.get(".empty-state").text()).toContain("还没有公开内容");
    expect(wrapper.get(".empty-state__actions").text()).toContain("去搜索文章");
  });

  it("接口失败：错误态 + 重试按钮触发 retry", async () => {
    const wrapper = mountPinned({ articles: [], failed: true });

    expect(wrapper.get(".empty-state").text()).toContain("暂时取不到");
    await wrapper.get(".empty-state__actions button").trigger("click");
    expect(wrapper.emitted("retry")).toHaveLength(1);
  });

  it("加载中：pending 空态标记，不显示空/错误文案", () => {
    const wrapper = mountPinned({ articles: [], pending: true });
    expect(wrapper.get("[data-empty-state='true']").attributes("data-state")).toBe("pending");
    expect(wrapper.get("[data-empty-state='true']").text()).toContain("正在读取");
  });
});
