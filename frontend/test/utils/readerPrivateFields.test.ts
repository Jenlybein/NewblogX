import { describe, expect, it } from "vitest";
import {
  READER_PRIVATE_FIELDS,
  withoutPrivateFields,
  withoutPrivateFieldsFromItems,
} from "~/utils/readerPrivateFields";

describe("readerPrivateFields：写缓存前剔除用户私有字段（ADR-0019 约束 2 / 票 #85）", () => {
  it("锁定需要剔除的字段：点赞 / 收藏 / 关注关系", () => {
    expect(READER_PRIVATE_FIELDS).toEqual(["is_digg", "is_favor", "relation"]);
  });

  it("文章详情：剔除 is_digg / is_favor，公开内容原样保留，且不修改原对象", () => {
    const article = {
      id: "a1",
      title: "标题",
      content: "正文",
      digg_count: 7,
      favor_count: 3,
      is_digg: true,
      is_favor: true,
    };

    const cached = withoutPrivateFields(article);

    expect(cached).not.toHaveProperty("is_digg");
    expect(cached).not.toHaveProperty("is_favor");
    expect(cached).toMatchObject({
      id: "a1",
      title: "标题",
      digg_count: 7,
      favor_count: 3,
    });
    // 返回值不是原对象：页面本次渲染仍要拿到完整的私有字段（命中后按未登录渲染，随后补齐）。
    expect(cached).not.toBe(article);
    expect(article.is_digg).toBe(true);
  });

  it("评论列表：逐条剔除 is_digg / relation（评论点赞与作者关系同样是私有态）", () => {
    const comments = [
      { id: "c1", content: "第一条", digg_count: 2, is_digg: true, relation: 1 },
      { id: "c2", content: "第二条", digg_count: 0, is_digg: false, relation: 0 },
    ];

    const cached = withoutPrivateFieldsFromItems(comments);

    expect(cached).toHaveLength(2);
    expect(cached[0]).not.toHaveProperty("is_digg");
    expect(cached[0]).not.toHaveProperty("relation");
    expect(cached[0]).toMatchObject({ id: "c1", content: "第一条" });
    expect(comments[0]).toHaveProperty("is_digg", true);
  });

  it("没有私有字段的对象照常返回（公开载荷不需要特殊处理）", () => {
    expect(withoutPrivateFields({ list: [], count: 0 })).toEqual({ list: [], count: 0 });
  });
});
