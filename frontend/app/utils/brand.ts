// 品牌单点定义（读者站）。
//
// 为什么有这个文件：用户可见的品牌串曾经散落在 12 个组件 / 页面 / service 里，
// 改名要全仓 grep。这里把品牌名、字标结构、助手昵称、一句话介绍与版权行收敛到
// 一处；组件、页面、head / OG 全部从这里取，禁止再写 "JellyBin"、"果冻盒" 或
// 旧展示名。
//
// 边界：内部标识**不在这里、也不改名** —— 主题 cookie `blogx-theme`、存储键
// `blogx_access_token` / `blogx_profile_snapshot`、`BLOGX_*` 环境变量前缀、
// compose 服务名与表名都保持原样（见 GLOSSARY「JellyBin（果冻盒）」）。
//
// 资产路径相对各自前端的 `public/` 根；`brandAssetUrl` 拼上 `app.baseURL` 得到
// 站内可用地址（后台的 `/admin/` 前缀自动带上）。
//
// 本模块必须是纯 TS（不 import Nuxt / Vue）：`nuxt.config.ts` 也要 import 它来
// 生成 `app.head`，两端单测直接 import 它。

export const BRAND = {
  name: { en: "JellyBin", zh: "果冻盒" },
  wordmark: { primary: "JellyBin", secondary: "果冻盒" },
  assistant: "捏捏",
  tagline: "一个会呼吸的个人博客",
  description: "JellyBin（果冻盒）是个人博客站：长文写作、站内搜索与 AI 助手「捏捏」。",
} as const;

export type BrandAssetKey = "flat" | "logo" | "apple" | "faviconRoot" | "og";

export const BRAND_ASSETS: Record<BrandAssetKey, string> = {
  flat: "brand/jelly-flat.svg",
  logo: "brand/logo-256.png",
  apple: "brand/apple-touch-icon.png",
  faviconRoot: "favicon.ico",
  og: "brand/og-image.png",
};

/** 品牌全名：`JellyBin（果冻盒）`。 */
export function brandFullName(): string {
  return `${BRAND.name.en}（${BRAND.name.zh}）`;
}

/**
 * 页面标题。空标题回落到品牌全名；已经带英文品牌名的标题不再重复拼接，
 * 避免出现「JellyBin（果冻盒） · JellyBin」。
 */
export function brandTitle(pageTitle?: string | null): string {
  const title = pageTitle?.trim();
  if (!title) {
    return brandFullName();
  }
  return title.includes(BRAND.name.en) ? title : `${title} · ${BRAND.name.en}`;
}

/** 分享 / SEO 描述：空值回落到品牌模块里的一句话介绍。 */
export function brandDescription(description?: string | null): string {
  return description?.trim() || BRAND.description;
}

/** 助手显示名：站点配置里给了昵称就用昵称，否则用品牌模块的统一昵称「捏捏」。 */
export function brandAiName(nickname?: string | null): string {
  return nickname?.trim() || BRAND.assistant;
}

/** 站点标题：优先用运行时配置里的项目名，没有才回落到品牌英文名。 */
export function brandProjectTitle(title?: string | null): string {
  return title?.trim() || BRAND.name.en;
}

/** 版权行：`© 2026 JellyBin（果冻盒）`。 */
export function brandCopyright(year: number): string {
  return `© ${year} ${brandFullName()}`;
}

/** 相对 `public/` 的资产 → 带 baseURL 的站内路径（`/admin/` 前缀在后台自动带上）。 */
export function brandAssetUrl(baseUrl: string, asset: BrandAssetKey): string {
  const base = baseUrl.trim() || "/";
  const normalized = base.endsWith("/") ? base : `${base}/`;
  return `${normalized}${BRAND_ASSETS[asset]}`;
}

/** 用于 OG / twitter 的绝对地址：siteUrl 去掉尾部斜杠后拼资产路径。 */
export function brandAbsoluteAssetUrl(siteUrl: string, asset: BrandAssetKey): string {
  const base = siteUrl.trim().replace(/\/+$/u, "");
  return base ? `${base}/${BRAND_ASSETS[asset]}` : `/${BRAND_ASSETS[asset]}`;
}
