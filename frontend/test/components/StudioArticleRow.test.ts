import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import StudioArticleRow from "~/components/studio/StudioArticleRow.vue";
import type { SearchArticleItem } from "~/types/api";

/**
 * 我的文章 · 压缩行卡（票 351，2026-11-12 重排）。
 *
 * 用户的原话是"丑到没边、状态单独占一行"：上一版把状态与操作塞在封面下方，
 * 卡片被撑成三行。现在钉的是新结构 —— 状态与标题、操作同处顶行，标签与四个
 * 计数同一条基线，整张卡只有两列。
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
    expect(row.find(".studio-article-row__baseline").exists()).toBe(true);
    // 不再套第二层通用 feed 卡（那正是"多余的行"的来源）。
    expect(row.find(".feed-entry").exists()).toBe(false);
  });

  it("状态与操作在标题同一行，不再单独占一行", () => {
    const wrapper = mountRow();

    // 顶行 = 状态 + 标题 + 操作，三者同属一个容器。
    const head = wrapper.get(".studio-article-row__head");
    expect(head.get(".studio-article-row__status").text()).toContain("已发布");
    expect(head.find(".studio-article-row__title").exists()).toBe(true);
    expect(head.find('[data-role="article-view"]').exists()).toBe(true);
    expect(head.find('[data-role="article-edit"]').exists()).toBe(true);
    expect(head.find('[data-role="article-delete"]').exists()).toBe(true);

    // 封面不再是"带状态列"的 aside：它只是卡片左列的一张图。
    expect(wrapper.find(".studio-article-row__aside").exists()).toBe(false);
    expect(wrapper.get(".studio-article-row").find(".studio-article-row__cover").exists()).toBe(true);
  });

  it("标签与四个计数在同一条基线上：标签在左、计数贴最右", () => {
    const wrapper = mountRow();

    const baseline = wrapper.get(".studio-article-row__baseline");
    // 同一容器里既有标签又有计数 —— 这就是"同一条 baseline"。
    expect(baseline.find(".studio-article-row__tags").exists()).toBe(true);
    const counts = baseline.get(".studio-article-row__counts");
    expect(counts.findAll("span")).toHaveLength(4);
    expect(counts.text()).toContain("12");
    expect(counts.text()).toContain("250");
    // 基线在正文列里（不是左列）。
    expect(wrapper.get(".studio-article-row__body").find(".studio-article-row__baseline").exists()).toBe(true);
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
