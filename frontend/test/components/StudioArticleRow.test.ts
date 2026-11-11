import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import StudioArticleRow from "~/components/studio/StudioArticleRow.vue";
import type { SearchArticleItem } from "~/types/api";

/**
 * 我的文章 · 压缩行卡（票 351）。
 *
 * 用户的原话是"列表项高度压缩、把多余行合并"：状态与查看/编辑/删除要和标签同列，
 * 四个计数要在同一行最右边。这层用例钉的是"这些信息都在同一张卡里、并且各就各位"，
 * 不钉具体像素。
 */

function article(overrides: Partial<SearchArticleItem> = {}): SearchArticleItem {
  return {
    id: "a1",
    title: "一篇文章",
    abstract: "这是摘要",
    cover: "https://cdn.example.com/c.jpg",
    publish_status: 3,
    visibility_status: "visible",
    comments_toggle: true,
    digg_count: 12,
    favor_count: 3,
    comment_count: 4,
    view_count: 250,
    tags: [{ id: "t1", title: "Go" }],
    ...overrides,
  } as SearchArticleItem;
}

function mountRow(overrides: Partial<SearchArticleItem> = {}, props: Record<string, unknown> = {}) {
  return mount(StudioArticleRow, {
    props: {
      article: article(overrides),
      statusLabel: "已发布",
      statusType: "success" as const,
      ...props,
    },
    global: {
      stubs: {
        NuxtLink: {
          props: { to: { type: [String, Object], default: "" } },
          template: '<a :data-href="typeof to === \'string\' ? to : JSON.stringify(to)"><slot /></a>',
        },
      },
    },
  });
}

describe("压缩行卡：结构", () => {
  it("只有一层卡片：封面、标题、摘要、标签、底部行都在同一张卡里", () => {
    const wrapper = mountRow();

    const row = wrapper.get(".studio-article-row");
    expect(row.find(".studio-article-row__cover").exists()).toBe(true);
    expect(row.find(".studio-article-row__title").exists()).toBe(true);
    expect(row.find(".studio-article-row__abstract").exists()).toBe(true);
    expect(row.find(".studio-article-row__tags").exists()).toBe(true);
    expect(row.find(".studio-article-row__foot").exists()).toBe(true);
    // 不再套第二层通用 feed 卡（那正是"多余的行"的来源）。
    expect(row.find(".feed-entry").exists()).toBe(false);
  });

  it("状态与操作在底部行左侧，四个计数在同一行右侧", () => {
    const wrapper = mountRow();

    const foot = wrapper.get(".studio-article-row__foot");
    const left = foot.get(".studio-article-row__left");
    const counts = foot.get(".studio-article-row__counts");

    // 状态在左。
    expect(left.text()).toContain("已发布");
    // 三个操作都在左。
    expect(left.find('[data-role="article-view"]').exists()).toBe(true);
    expect(left.find('[data-role="article-edit"]').exists()).toBe(true);
    expect(left.find('[data-role="article-delete"]').exists()).toBe(true);
    // 四个计数在右（顺序：赞 / 藏 / 评 / 阅）。
    expect(counts.findAll("span")).toHaveLength(4);
    expect(counts.text()).toContain("12");
    expect(counts.text()).toContain("250");
  });

  it("摘要缺失时给一句话占位，卡片高度不会塌陷", () => {
    const wrapper = mountRow({ abstract: "" });
    expect(wrapper.get(".studio-article-row__abstract").text()).toContain("还没有摘要");
  });

  it("标签最多三个，多的折成 +N", () => {
    const wrapper = mountRow({
      tags: [
        { id: "t1", title: "Go" },
        { id: "t2", title: "Vue" },
        { id: "t3", title: "MySQL" },
        { id: "t4", title: "Redis" },
        { id: "t5", title: "Docker" },
      ],
    });

    expect(wrapper.findAll(".studio-article-row__tag")).toHaveLength(3);
    expect(wrapper.get(".studio-article-row__tag-more").text()).toBe("+2");
  });
});

describe("压缩行卡：操作", () => {
  it("点编辑 / 删除抛出文章 id，由页面决定怎么处理", async () => {
    const wrapper = mountRow();

    await wrapper.get('[data-role="article-edit"]').trigger("click");
    await wrapper.get('[data-role="article-delete"]').trigger("click");

    expect(wrapper.emitted("edit")).toEqual([["a1"]]);
    expect(wrapper.emitted("delete")).toEqual([["a1"]]);
  });

  it("查看详情直接指向文章页，不额外发事件", () => {
    const wrapper = mountRow();
    const view = wrapper.get('[data-role="article-view"]');
    expect(view.attributes("data-href")).toBe("/article/a1");
  });

  it("可见性异常时显示对应标签，正常时不占位", () => {
    expect(mountRow({}, { visibilityLabel: "仅自己可见" }).text()).toContain("仅自己可见");
    expect(mountRow({}, { visibilityLabel: "" }).findAll(".studio-article-row__status")).toHaveLength(1);
  });
});
