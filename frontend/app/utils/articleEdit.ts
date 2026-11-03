/**
 * 阅读页「编辑」入口的门控与地址（票 213 / 决定清单 C4）。
 *
 * 决定：入口位置是 hero 元信息行（作者 / 时间 / 阅读数旁）；门控**仅作者本人**，
 * 管理员仍走后台文章管理，不在读者页再开一套权限；点击跳创作页的编辑态，
 * 地址沿用既有约定 `/studio/write?article_id=<id>`（`useWriteEntry.ts` 与
 * `pages/studio/profile.vue` 用的是同一形状）。
 *
 * 收成纯函数的原因：页面模板只做"有就渲染、没有就不渲染"，而"谁能看到"是两个
 * 既有信号（本人身份 + 站点发布开关）的合取 —— 放在这里才能脱离页面单独断言。
 */

/** 屏幕阅读器文案（C4 指定的 `aria-label="编辑这篇文章"`）。 */
export const ARTICLE_EDIT_ARIA_LABEL = "编辑这篇文章";

/** 可见时给模板的渲染数据。 */
export interface ArticleEditEntry {
  /** 创作页编辑态的站内地址。 */
  href: string;
  /** 链接的 `aria-label`。 */
  ariaLabel: string;
}

export interface ArticleEditEntryOptions {
  /** 当前文章 id；缺席时不给入口（宁可不渲染，也不拼出会跳错文章的链接）。 */
  articleId: string | null | undefined;
  /** 读者是否是这篇文章的作者（页面用 `profileId === author_id` 得出）。 */
  isSelfAuthor: boolean;
  /** 站点「创作 / 写文章」入口是否可见（`useArticleWritePolicy().entryVisible`）。 */
  writeEntryVisible: boolean;
}

/**
 * 作者本人在发布开关打开时拿到入口，其余情况返回 `null`。
 *
 * 发布开关是既有 `useArticleWritePolicy` 的投影，不在这里另判站点配置：
 * 本入口与页头「创作」必须遵守同一个开关，否则会出现"页头创作没了、
 * 文章页还能进编辑器"的不一致。
 */
export function resolveArticleEditEntry(options: ArticleEditEntryOptions): ArticleEditEntry | null {
  const articleId = options.articleId?.trim();

  if (!options.isSelfAuthor || !options.writeEntryVisible || !articleId) {
    return null;
  }

  return {
    href: `/studio/write?article_id=${encodeURIComponent(articleId)}`,
    ariaLabel: ARTICLE_EDIT_ARIA_LABEL,
  };
}
