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

const hotTopArticles = [
  {
    id: "a1",
    title: "置顶文章",
    abstract: "置顶摘要",
    cover: "",
    category_title: "工程",
    author_nickname: "作者甲",
    view_count: 120,
  },
  {
    id: "a2",
    title: "第二篇置顶",
    abstract: "",
    cover: "",
    category_title: "",
    author_nickname: "",
    view_count: 3,
  },
];

function mountPinned(props: Record<string, unknown> = {}) {
  return mount(HomePinnedSection, {
    props: { articles: [], ...props },
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
}

describe("HomePinnedSection：热门置顶运营位", () => {
  it("有数据：按后台顺序渲染编号 + 标题 + 摘要 + 出处的网格，且可点进文章", () => {
    const wrapper = mountPinned({ articles: hotTopArticles });

    const tiles = wrapper.findAll(".home-pinned-card");
    expect(tiles).toHaveLength(2);
    expect(tiles[0]?.text()).toContain("01");
    expect(tiles[0]?.text()).toContain("置顶文章");
    expect(tiles[0]?.text()).toContain("作者甲");
    expect(tiles[0]?.text()).toContain("120 阅读");
    // 顺序就是管理员排的顺序：渲染层不重排（ADR-0017 决策 5）。
    expect(tiles[1]?.text()).toContain("02");
    expect(tiles[1]?.text()).toContain("第二篇置顶");
    expect(wrapper.get('[data-role="pinned-badge"]').text()).toBe("2 篇");
    expect(wrapper.findAll(".home-pinned-card__title")[0]?.attributes("href") ?? "").toContain("/article/a1");
  });

  it("无数据：整块不渲染 —— 标题、徽标、空态文案都不出现（ADR-0017 决策 5）", () => {
    const wrapper = mountPinned({ articles: [] });

    expect(wrapper.find('[data-home-section="pinned"]').exists()).toBe(false);
    expect(wrapper.find(".home-pinned-card").exists()).toBe(false);
    expect(wrapper.find(".empty-state").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("热门置顶");
    expect(wrapper.find('[data-role="pinned-badge"]').exists()).toBe(false);
  });
});
