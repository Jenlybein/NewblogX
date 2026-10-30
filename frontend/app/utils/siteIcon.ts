/**
 * 站点图标（`<link rel="icon">`）的取值规则。
 *
 * 背景：读者站的品牌图标由 `nuxt.config` 的 `app.head` 统一发出（品牌 SVG + `.ico` 兜底 +
 * apple-touch-icon）。后端 `GET /site/seo` 里另有一个 `icon` 字段（管理员在后台
 * 「站点 → SEO」里配置）——它是可覆盖入口，但默认值是占位串 `/xxx.png`。如果直接照发，
 * 占位串会盖掉品牌图标并 404（线上实测过：`<link rel="icon" href="/xxx.png">` 排在品牌图标之后）。
 *
 * 规则：
 * - 空值 / 占位值 → 不发（让 `app.head` 里的品牌图标生效）；
 * - 真实值（管理员显式配置）→ 作为覆盖发出，并按扩展名补 `type`。
 */

const PLACEHOLDER_ICONS = new Set(["/xxx.png"]);

const TYPE_BY_EXTENSION: Record<string, string> = {
  svg: "image/svg+xml",
  ico: "image/x-icon",
  png: "image/png",
  webp: "image/webp",
  gif: "image/gif",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
};

export interface SiteIconLink {
  rel: string;
  type?: string;
  href: string;
}

/** 是否是没有实际意义的占位图标（空串、纯空白、已知占位串）。 */
export function isPlaceholderSiteIcon(icon?: string | null): boolean {
  const value = (icon ?? "").trim();
  if (!value) return true;
  return PLACEHOLDER_ICONS.has(value);
}

/** 按扩展名给图标补 MIME type；认不出来就不给（浏览器会自行嗅探）。 */
export function siteIconType(href: string): string | undefined {
  const withoutQuery = href.split("?")[0]?.split("#")[0] ?? "";
  const extension = withoutQuery.split(".").pop()?.toLowerCase() ?? "";
  return TYPE_BY_EXTENSION[extension];
}

/**
 * 产出应当追加到 `<head>` 的图标链接。
 * 返回空数组表示"交给 `app.head` 的品牌图标"。
 */
export function siteIconLinks(icon?: string | null): SiteIconLink[] {
  if (isPlaceholderSiteIcon(icon)) return [];

  const href = (icon as string).trim();
  const type = siteIconType(href);
  return type ? [{ rel: "icon", type, href }] : [{ rel: "icon", href }];
}
