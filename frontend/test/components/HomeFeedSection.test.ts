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
  it("有数据时渲染文章条目，且首批**零入场**（不隐藏、不延迟）", () => {
    const wrapper = mountFeed({ articles: [makeArticle(), makeArticle({ id: "a2", title: "第二篇" })] });

    const items = wrapper.findAll(".feed-motion-item");
    expect(items).toHaveLength(2);
    expect(wrapper.get('[data-role="feed-list"]').text()).toContain("文章标题");
    // 票 443：初态就是可见的，时长为 0 —— 正文不该等水合才出现。
    expect(items[0]?.attributes("data-motion-duration")).toBe("0");
    expect(items[1]?.attributes("data-motion-duration")).toBe("0");
    expect(items[0]?.attributes("data-motion-delay")).toBe("0");
    expect(items[1]?.attributes("data-motion-delay")).toBe("0");
    expect(wrapper.find(".empty-state").exists()).toBe(false);
  });

  it("减少动态时：翻页淡入同样归零（静态出现，不是慢慢动）", async () => {
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
    await wrapper.setProps({ articles: [makeArticle({ id: "a9", title: "第二页" })] });

    expect(wrapper.findAll(".feed-motion-item")[0]?.attributes("data-motion-duration")).toBe("0");
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

  it("加载中：先出骨架（页面不会先闪错误态，也不塌成空态文案）", () => {
    const wrapper = mountFeed({ articles: [], pending: true });

    const skeleton = wrapper.get('[data-role="feed-skeleton"]');
    expect(skeleton.attributes("data-state")).toBe("pending");
    expect(wrapper.findAll("[data-role='skeleton-bar']")).toHaveLength(3);
    expect(wrapper.find(".empty-state").exists()).toBe(false);
  });
});

/**
 * 翻页不重播（#433）。
 *
 * 读者反馈：点"下一页"之后整批条目又从 opacity:0 逐条错峰入场，等于每次翻页都要
 * 等半秒。规则是**首次进入该区块只播一次**，用组件内的"已入场"标记判定（不落
 * session 存储）：批次换了就是"新一批直接出现"，入场 props 归零且没有隐藏初态。
 *
 * 断言走两个可观察面：列表容器上的 `data-entrance`（stagger / instant）与 Motion
 * 收到的 duration / delay —— 不断言像素。
 */
describe("HomeFeedSection：翻页不重播", () => {
  function entrance(wrapper: ReturnType<typeof mountFeed>) {
    return wrapper.get('[data-role="feed-list"]').attributes("data-entrance");
  }

  it("首批直接可见；翻页后新一批只做一次整体淡入（不排队、不留空白帧）", async () => {
    const wrapper = mountFeed({ articles: [makeArticle(), makeArticle({ id: "a2" })] });

    expect(entrance(wrapper)).toBe("visible");
    expect(wrapper.findAll(".feed-motion-item")[1]?.attributes("data-motion-delay")).toBe("0");

    await wrapper.setProps({
      articles: [
        makeArticle({ id: "a3", title: "第二页第一篇" }),
        makeArticle({ id: "a4", title: "第二页第二篇" }),
      ],
    });

    expect(entrance(wrapper)).toBe("swap");
    expect(wrapper.get('[data-role="feed-list"]').text()).toContain("第二页第一篇");
    // 一次 120ms 的淡入（0.12s），延迟恒为 0（所有条目同时进）。
    expect(wrapper.findAll(".feed-motion-item").map((item) => item.attributes("data-motion-duration"))).toEqual([
      "0.12",
      "0.12",
    ]);
    expect(wrapper.findAll(".feed-motion-item").map((item) => item.attributes("data-motion-delay"))).toEqual([
      "0",
      "0",
    ]);
  });

  it("空批次（加载中）不消耗批次身份：数据首次到达仍算首批、直接可见", async () => {
    const wrapper = mountFeed({ articles: [], pending: true });

    await wrapper.setProps({ articles: [makeArticle()], pending: false });

    expect(entrance(wrapper)).toBe("visible");
    expect(wrapper.get(".feed-motion-item").attributes("data-motion-duration")).toBe("0");
  });
});
