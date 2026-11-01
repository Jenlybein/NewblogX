import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { READER_PRIVATE_FIELDS } from "~/utils/readerPrivateFields";

/**
 * "SSR 输出不得含用户态"门禁（ADR-0019 约束 2）。
 *
 * 服务端缓存的键只有 URL（约束 3）：同一份 HTML 会发给所有读者，登录态由客户端补齐。
 * 所以文章页与评论区的 SSR 输出里不能出现 `is_digg` / `is_favor` / `relation` 这类
 * "我有没有点过赞 / 收藏过 / 关注过"的字段 —— 出现任何一个，共享缓存就变成串号缓存，
 * 而且值是匿名值（恒 false）的时候**没有任何信号**，只能靠门禁抓。
 *
 * 判据写成纯函数 + 人造违规用例（canary），风格参照 test/gates/rendering-boundary.test.ts；
 * 另加两条接线检查，防止"剔除"在以后重构里被悄悄拆掉。
 */
const projectDir = process.cwd();

/** 用户私有字段清单（SSR 输出不得含）。 */
const PRIVATE_VIEW_STATE_KEYS = ["is_digg", "is_favor", "relation"];

/**
 * 判据：序列化后的 SSR 输出里是否残留私有字段。
 *
 * 只认 JSON 键形态（`"is_digg":`）：正文里出现同名的普通字符串不算泄漏，否则会误报。
 */
function privateViewStateKeysInSerialized(serialized: string): string[] {
  return PRIVATE_VIEW_STATE_KEYS.filter((key) => new RegExp(`"${key}"\\s*:`, "u").test(serialized));
}

describe("SSR 输出不得含用户态（ADR-0019 约束 2）", () => {
  it("私有字段清单只有一份（readerPrivateFields），且覆盖点赞 / 收藏 / 关注关系", () => {
    // 客户端 30s 缓存（#85）与服务端剔除（#86）必须共用同一张表，否则会漏掉一半。
    expect([...READER_PRIVATE_FIELDS].sort()).toEqual(["is_digg", "is_favor", "relation"]);
  });

  it("判据认出 payload 里的私有键：is_digg / relation 会报出来", () => {
    const payload = '{"article":{"id":"a1","is_digg":false,"relation":0}}';

    expect(privateViewStateKeysInSerialized(payload)).toEqual(["is_digg", "relation"]);
  });

  it("公开计数不误报：digg_count / favor_count 是所有人共享的数字", () => {
    expect(privateViewStateKeysInSerialized('{"digg_count":3,"favor_count":1}')).toEqual([]);
  });

  it("门禁不能恒绿：canary —— 未剔除的载荷必须被抓到，剔除后必须干净", () => {
    const raw = '{"is_digg":true,"is_favor":true}';
    const anonymous = '{"id":"a1","title":"标题","digg_count":3}';

    expect(privateViewStateKeysInSerialized(raw)).toEqual(["is_digg", "is_favor"]);
    expect(privateViewStateKeysInSerialized(anonymous)).toEqual([]);
  });

  it("接线：文章页服务端取数是匿名口径（auth: false），客户端才用带鉴权的详情", () => {
    // 缓存键只有 URL：SSR 取数绝不能携带登录身份，否则 HTML 内容会随请求变化。
    const serviceSource = readFileSync(join(projectDir, "app/services/article.ts"), "utf8");

    expect(serviceSource).toContain("getArticleDetailAnonymous");
    expect(serviceSource).toContain("{ auth: false }");
  });

  it("接线：服务端正文与评论都经过同一份私有字段清单剔除", () => {
    const composableSource = readFileSync(
      join(projectDir, "app/composables/useArticlePageSources.ts"),
      "utf8",
    );

    expect(composableSource).toContain("getArticleDetailAnonymous");
    expect(composableSource).toContain("withoutPrivateFields");
    expect(composableSource).toContain("withoutPrivateFieldsFromItems");
    expect(composableSource).toContain('mode === "server"');
  });
});
