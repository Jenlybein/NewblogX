// 品牌单点定义（管理后台）。
//
// 为什么有这个文件：后台的 4 处品牌串（后台壳、登录页、setup 页、文章封面兜底）
// 曾经各自写死旧展示名，改名要全仓 grep。这里把品牌名、字标结构、助手昵称、
// 一句话介绍与版权行收敛到一处；后台组件、页面与 head / OG 全部从这里取。
//
// 边界：内部标识**不在这里、也不改名** —— `BLOGX_ADMIN_*` 环境变量前缀、
// `blogx_admin` 服务名、后台登录存储键都保持原样（见 GLOSSARY「JellyBin（果冻盒）」）。
//
// 与读者站那份刻意不共享代码：两个前端是独立构建的应用，重复一份纯数据比
// 造一个跨应用包更简单。**两份的取值必须一致**（改名时两边一起改）。
//
// 本模块必须是纯 TS（不 import Nuxt / Vue）：`nuxt.config.ts` 也要 import 它来
// 生成 `app.head`，后台单测直接 import 它。

export const BRAND = {
  name: { en: "JellyBin", zh: "果冻盒" },
  wordmark: { primary: "JellyBin", secondary: "果冻盒" },
  assistant: "捏捏",
  adminLabel: "管理后台",
  tagline: "内容运营与系统管理",
  description: "JellyBin（果冻盒）管理后台：内容审核、站点配置与运行观测。",
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

/** 后台默认标题：`JellyBin 管理后台`。 */
export function brandAdminTitle(): string {
  return `${BRAND.name.en} ${BRAND.adminLabel}`;
}

/**
 * 页面标题。空标题回落到后台默认标题；已经带英文品牌名的标题不再重复拼接。
 */
export function brandTitle(pageTitle?: string | null): string {
  const title = pageTitle?.trim();
  if (!title) {
    return brandAdminTitle();
  }
  return title.includes(BRAND.name.en) ? title : `${title} · ${BRAND.name.en} ${BRAND.adminLabel}`;
}

/** 分享 / SEO 描述：空值回落到品牌模块里的一句话介绍。 */
export function brandDescription(description?: string | null): string {
  return description?.trim() || BRAND.description;
}

/** 助手显示名：站点配置里给了昵称就用昵称，否则用品牌模块的统一昵称「捏捏」。 */
export function brandAiName(nickname?: string | null): string {
  return nickname?.trim() || BRAND.assistant;
}

/** 版权行：`© 2026 JellyBin（果冻盒）`。 */
export function brandCopyright(year: number): string {
  return `© ${year} ${brandFullName()}`;
}

/** 相对 `public/` 的资产 → 带 `app.baseURL`（后台是 `/admin/`）的站内路径。 */
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
