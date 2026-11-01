import { describe, expect, it } from "vitest";

import {
  PRIVATE_VIEW_STATE_KEYS,
  isPrivateViewStateKey,
  privateViewStateKeysInSerialized,
  withoutPrivateViewState,
} from "~/utils/ssrAnonymity";

/**
 * SSR 匿名化判据（ADR-0019 决策 5 / 约束 2）。
 *
 * 服务端缓存的键只有 URL（约束 3），同一份 HTML 会发给所有读者；只要有一个
 * "我有没有点过赞 / 收藏过 / 关注过"的字段留在 SSR 输出里，缓存就变成串号缓存。
 * 所以这些字段必须**连字段名都不出现**（只把值写成 undefined，payload 里仍留着键）。
 */
describe("SSR 匿名化：用户私有字段清单与剔除", () => {
  it("私有字段清单覆盖点赞 / 收藏 / 关注关系（新增用户态字段必须进这张表）", () => {
    expect([...PRIVATE_VIEW_STATE_KEYS].sort()).toEqual(["is_digg", "is_favor", "relation"]);
  });

  it("剔除后连字段名都不在（不是把值改成 undefined）", () => {
    const article = { id: "a1", title: "标题", content: "正文", is_digg: true, is_favor: false };

    const anonymous = withoutPrivateViewState(article);

    expect(anonymous).toEqual({ id: "a1", title: "标题", content: "正文" });
    expect("is_digg" in anonymous).toBe(false);
    expect("is_favor" in anonymous).toBe(false);
    expect(JSON.stringify({ data: anonymous })).not.toContain("is_digg");
  });

  it("不修改入参：客户端随后还要拿同一份数据补齐点赞态", () => {
    const article = { id: "a1", is_digg: true };

    withoutPrivateViewState(article);

    expect(article.is_digg).toBe(true);
  });

  it("isPrivateViewStateKey 只认清单里的键，公开字段（digg_count / favor_count）不受影响", () => {
    expect(isPrivateViewStateKey("is_digg")).toBe(true);
    expect(isPrivateViewStateKey("relation")).toBe(true);
    expect(isPrivateViewStateKey("digg_count")).toBe(false);
    expect(isPrivateViewStateKey("title")).toBe(false);
  });
});

describe("SSR 匿名化门禁判据：序列化后的 HTML 里出现私有键就算漏", () => {
  it("检出同名字段：payload 里出现 is_digg / relation 就报出来", () => {
    const payload = `<script type="application/json" id="__NUXT_DATA__">{"article":{"id":"a1","is_digg":false,"relation":0}}</script>`;

    expect(privateViewStateKeysInSerialized(payload)).toEqual(["is_digg", "relation"]);
  });

  it("正常字段名不误报：digg_count / favor_count 是公开计数", () => {
    const payload = `{"digg_count":3,"favor_count":1,"title":"标题"}`;

    expect(privateViewStateKeysInSerialized(payload)).toEqual([]);
  });

  it("门禁不能恒绿：canary —— 未剔除的载荷必须被抓住，剔除后必须干净", () => {
    // 这条用例是判据的"人造违规"：判据被改坏（比如只认值不认键）时这里会红。
    const rawPayload = `<script id="__NUXT_DATA__">{"article":{"is_digg":true,"is_favor":true}}</script>`;
    const anonymousPayload = `<script id="__NUXT_DATA__">{"article":{"id":"a1","title":"标题"}}</script>`;

    expect(privateViewStateKeysInSerialized(rawPayload)).toEqual(["is_digg", "is_favor"]);
    expect(privateViewStateKeysInSerialized(anonymousPayload)).toEqual([]);
  });
});
