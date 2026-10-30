import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import HomeFeedSection from "~/components/home/HomeFeedSection.vue";
import type { SearchArticleItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(_props, { slots }) {
    return () => h("a", slots.default?.());
  },
});

/** 用 stub 把 Motion 的 transition 值落成 DOM 属性，便于断言"减少动态时归零"。 */
const MotionStub = defineComponent({
  props: {
    initial: { type: Object, default: () => ({}) },
    animate: { type: Object, default: () => ({}) },
    transition: { type: Object, default: () => ({}) },
  },
  setup(props, { slots }) {
    return () =>
      h(
        "div",
        {
          "data-motion-duration": String(props.transition?.duration ?? ""),
          "data-motion-delay": String(props.transition?.delay ?? ""),
        },
        slots.default?.(),
      );
  },
});

function makeArticle(overrides: Record<string, unknown> = {}): SearchArticleItem {
  return {
    id: "a1",
    title: "文章标题",
    abstract: "摘要",
    cover: "",
    created_at: "2026-10-01T00:00:00Z",
    category: { title: "工程" },
    author: { id: "u1", nickname: "作者甲", avatar: "" },
    tags: [{ id: "t1", title: "Nuxt" }],
    digg_count: 1,
    favor_count: 2,
    comment_count: 3,
    view_count: 4,
    ...overrides,
  } as unknown as SearchArticleItem;
}

function mountFeed(props: Record<string, unknown> = {}) {
  return mount(HomeFeedSection, {
    props: { articles: [], pending: false, failed: false, ...props },
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        Motion: MotionStub,
        IconThumbUp: true,
        IconHeart: true,
        IconMessageCircle2: true,
        IconEye: true,
      },
    },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("HomeFeedSection：最新文章 + 分级淡入", () => {
  it("有数据时渲染文章条目，条目带 motion-stagger-item 与递进 delay", () => {
    const wrapper = mountFeed({ articles: [makeArticle(), makeArticle({ id: "a2", title: "第二篇" })] });

    const items = wrapper.findAll(".motion-stagger-item");
    expect(items).toHaveLength(2);
    expect(wrapper.get('[data-role="feed-list"]').text()).toContain("文章标题");
    expect(items[0]?.attributes("data-motion-delay")).toBe("0");
    expect(items[1]?.attributes("data-motion-delay")).toBe("0.04");
    expect(wrapper.find(".empty-state").exists()).toBe(false);
  });

  it("减少动态时：stagger 时长/延迟归零（静态出现，不是慢慢动）", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      onchange: null,
      dispatchEvent: () => false,
    }));

    const wrapper = mountFeed({ articles: [makeArticle(), makeArticle({ id: "a2" })] });
    const items = wrapper.findAll(".motion-stagger-item");
    expect(items[0]?.attributes("data-motion-duration")).toBe("0");
    expect(items[1]?.attributes("data-motion-duration")).toBe("0");
    expect(items[1]?.attributes("data-motion-delay")).toBe("0");
  });

  it("空数据：渲染统一空态，不塌成一行字，并给出「去搜索文章」入口", () => {
    const wrapper = mountFeed({ articles: [] });

    const empty = wrapper.get("[data-home-section='feed'] .empty-state");
    expect(empty.text()).toContain("还没有公开内容");
    expect(wrapper.get(".empty-state__actions").text()).toContain("去搜索文章");
    expect(wrapper.find('[data-role="feed-list"]').exists()).toBe(false);
  });

  it("接口失败：错误态渲染 + 「重试本页」按钮，点击触发 retry 事件", async () => {
    const wrapper = mountFeed({ articles: [], failed: true });

    expect(wrapper.get(".empty-state").text()).toContain("暂时取不到");
    const retry = wrapper.get(".empty-state__actions button");
    expect(retry.text()).toBe("重试本页");
    await retry.trigger("click");
    expect(wrapper.emitted("retry")).toHaveLength(1);
  });

  it("加载中：pending 空态标记（页面不会先闪错误态）", () => {
    const wrapper = mountFeed({ articles: [], pending: true });
    expect(wrapper.get("[data-empty-state='true']").attributes("data-state")).toBe("pending");
  });
});
