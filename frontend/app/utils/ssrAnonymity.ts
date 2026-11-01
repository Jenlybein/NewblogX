import type { ArticleDetail, CommentRootItem } from "~/types/api";

/**
 * SSR 匿名化（ADR-0019 决策 5 / 约束 2）。
 *
 * 服务端缓存的键**只有 URL**（约束 3）：同一份 HTML 会发给所有读者，登录态靠
 * 客户端在自己发请求时补齐。所以 SSR 输出里不能出现任何"我有没有点过赞 / 收藏过 /
 * 关注过"的字段 —— 只要一个字段进了 HTML（哪怕值是 false），缓存就会在读者之间串号，
 * 而且没人能从这个恒为 false 的字段上看出问题。
 *
 * 这里集中定义"用户私有状态字段"清单与剔除函数，供页面 composable 在服务端取数后调用；
 * 判据（{@link privateViewStateKeysInSerialized}）给门禁用，见
 * test/utils/ssrAnonymity.test.ts 与 test/gates/ssr-anonymous-cache.test.ts。
 */

/** 用户私有状态字段：SSR 输出里一律不得出现（新增用户态字段必须加进这张表）。 */
export const PRIVATE_VIEW_STATE_KEYS = ["is_digg", "is_favor", "relation"] as const;

export type PrivateViewStateKey = (typeof PRIVATE_VIEW_STATE_KEYS)[number];

/** 一个键是否属于用户私有状态。 */
export function isPrivateViewStateKey(key: string): key is PrivateViewStateKey {
  return (PRIVATE_VIEW_STATE_KEYS as readonly string[]).includes(key);
}

/**
 * 剔除数据对象上的用户私有状态字段，返回**新对象**。
 *
 * 为什么不是"把值改成 undefined"：`undefined` 在 devalue payload 里仍可能留下键，
 * 而且 HTML 里出现字段名本身就会让"SSR 不含用户态"这条约束失去可判性。直接把键
 * 不拷贝，序列化后的 HTML 里连名字都没有。
 *
 * 只做浅层剔除：用户态只出现在文章详情与评论条目这两种数据的顶层，嵌套里的公开
 * 计数（digg_count / favor_count）必须保留。
 *
 * @param value 服务端取回的数据对象（不会被修改）
 */
export function withoutPrivateViewState<T extends object>(value: T): Omit<T, PrivateViewStateKey> {
  const anonymous: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (isPrivateViewStateKey(key)) {
      continue;
    }
    anonymous[key] = item;
  }
  return anonymous as Omit<T, PrivateViewStateKey>;
}

/** 文章详情：服务端渲染用的匿名版本（客户端后续自己补 is_digg / is_favor）。 */
export function anonymousArticleView(article: ArticleDetail): ArticleDetail {
  return withoutPrivateViewState(article);
}

/** 评论条目：服务端渲染用的匿名版本（客户端后续自己补 is_digg / relation）。 */
export function anonymousCommentView<T extends CommentRootItem>(comment: T): T {
  return withoutPrivateViewState(comment) as T;
}

/**
 * 门禁判据：一段序列化后的 SSR 输出里是否还残留用户私有状态字段。
 *
 * 只认 JSON 键形态（`"is_digg":`），不认正文里出现的同名字符串 —— 文章内容里可能
 * 就有 `is_digg` 这个词，那把正文当违规会误报。返回命中的字段名（空数组 = 干净）。
 *
 * @param serialized 序列化后的 HTML / payload 片段
 */
export function privateViewStateKeysInSerialized(serialized: string): PrivateViewStateKey[] {
  return PRIVATE_VIEW_STATE_KEYS.filter((key) => new RegExp(`"${key}"\\s*:`, "u").test(serialized));
}
