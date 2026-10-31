import type { SiteRuntimeConfig } from "~/types/api";

/**
 * 「发布与展示」两个写作开关在读者站的投影（#64 / ADR-0017 决策 2）。
 *
 * 服务端判定是唯一边界（`internal/article/app/create.go` / `update.go`），
 * 这里只负责"让界面说与服务端一致的话"：
 *   - 发布开关关闭 → 隐藏「写文章」入口；
 *   - 草稿开关关闭 → 编辑器只读，保存/发布按钮禁用并说明原因。
 *
 * 三个开关的业务默认值都是 `true`（`site_default_settings.yaml`），所以配置缺席
 * （首屏还没取回、老后台没发这几个键、请求失败）一律按全开处理：
 * 不能因为一次请求失败就把全站作者锁成只读。
 */

/** ArticleWritePolicy 是读者站真正消费的两个开关（已按默认值归一）。 */
export interface ArticleWritePolicy {
  /** 是否允许发布文章（非管理员）。 */
  allowPublish: boolean;
  /** 是否允许创建/保存草稿（非管理员）。 */
  allowDraft: boolean;
}

/**
 * 发布关闭时编辑器顶部的说明。
 *
 * 文案必须点明是"站点策略"：用户看到后应知道去找管理员改站点开关，
 * 而不是以为自己账号被限制了（与后端 `article.publish_disabled_by_site` 同一口径）。
 */
export const PUBLISH_DISABLED_NOTICE =
  "站点当前不允许发布文章（这是站点策略，与你的账号无关）。发布与提交审核已关闭，请联系管理员开启「允许非管理员发布文章」。";

/** 草稿关闭时的说明：编辑器只读，存量草稿仍可查看与删除。 */
export const DRAFT_DISABLED_NOTICE =
  "站点当前不允许保存草稿（这是站点策略，与你的账号无关）。编辑器已切换为只读，已有草稿仍可查看与删除。";

/** 把站点运行时配置归一成两个写作开关。 */
export function articleWritePolicy(
  config: SiteRuntimeConfig | null | undefined,
): ArticleWritePolicy {
  const article = config?.article;
  return {
    // 只有显式 false 才算关闭：undefined 是"还没取到 / 老后台没这个键"。
    allowPublish: article?.allow_non_admin_publish !== false,
    allowDraft: article?.allow_non_admin_draft !== false,
  };
}

/** 「写文章」入口是否展示：发布开关关闭时隐藏（ADR-0017 决策 2）。 */
export function writeEntryVisible(policy: ArticleWritePolicy): boolean {
  return policy.allowPublish;
}

/** 编辑器是否只读：草稿开关关闭时只读。 */
export function editorReadOnly(policy: ArticleWritePolicy): boolean {
  return !policy.allowDraft;
}

/** 编辑器顶部要展示的站点策略说明；空数组表示不渲染任何提示。 */
export function writePolicyNotices(policy: ArticleWritePolicy): string[] {
  const notices: string[] = [];
  // 发布在前：先解释"为什么入口/发布按钮没了"，再说编辑器为什么只读。
  if (!policy.allowPublish) notices.push(PUBLISH_DISABLED_NOTICE);
  if (!policy.allowDraft) notices.push(DRAFT_DISABLED_NOTICE);
  return notices;
}
