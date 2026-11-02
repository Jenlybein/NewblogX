import type { ThemeMode } from "~/types/api";

/**
 * 页头 / 页脚的纯逻辑（从模板里抽出来，便于单测与复用）。
 *
 * 为什么抽出来：导航 active 判定、搜索跳转的 URL 形状、无障碍文案、页脚链接
 * 都是"改文案就静默错"的地方，而它们又完全不依赖 DOM。模板只做渲染。
 *
 * 路由契约不许变：搜索固定去 `/search`、参数固定 `?key=`、创作入口固定
 * `/studio/write`（本票只重排外观）。
 */

export type HeaderNavKey = "home" | "search" | "write" | "collections";

export interface HeaderNavItem {
  key: HeaderNavKey;
  label: string;
  to: string;
  active: boolean;
}

const HEADER_NAV: ReadonlyArray<Omit<HeaderNavItem, "active">> = [
  { key: "home", label: "首页", to: "/" },
  { key: "search", label: "搜索", to: "/search" },
  { key: "write", label: "创作", to: "/studio/write" },
  // 合集是第四个入口：管理员编排的主题文章集合（读者侧只读）。
  // 图标不在渲染层按 key 三元选择（那样新 key 会拿到错误的图标），
  // 见 utils/navIcon.ts 的映射表。
  { key: "collections", label: "合集", to: "/collections" },
];

function isActive(key: HeaderNavKey, pathname: string): boolean {
  if (key === "home") {
    // 首页只认根路径：/article/xxx 不该把"首页"点亮。
    return pathname === "/";
  }
  if (key === "search") {
    return pathname.startsWith("/search");
  }
  if (key === "collections") {
    // 合集列表与合集详情都算"在合集里"：详情页不能让导航丢掉位置。
    return pathname.startsWith("/collections");
  }
  return pathname.startsWith("/studio/write");
}

/** 页头导航条目 + 当前路径下的 active 标记（同一路径最多一个 active）。 */
export function headerNavItems(pathname: string): HeaderNavItem[] {
  return HEADER_NAV.map((item) => ({ ...item, active: isActive(item.key, pathname) }));
}

export interface FooterNavItem {
  label: string;
  to: string;
}

/** 页脚只放两个站内链接（"少量链接"，不铺全站地图）。 */
export function footerNavItems(): FooterNavItem[] {
  return [
    { label: "首页", to: "/" },
    { label: "搜索", to: "/search" },
  ];
}

/** 备案行：读者站唯一的展示位置在页脚，集中在这里避免多处硬编码。 */
export const FOOTER_ICP = "粤ICP备 2026000000 号";

/**
 * 搜索跳转的 URL 形状（与改造前一致）：空关键词不拼 `?key=`，
 * 有关键词时去掉首尾空白。
 */
export function buildSearchLocation(keyword: string): {
  path: "/search";
  query: { key?: string };
} {
  const trimmed = keyword.trim();
  return {
    path: "/search",
    query: trimmed ? { key: trimmed } : {},
  };
}

/** 路由 query 回填输入框：只接受字符串，其它类型一律当空（避免 [object Object]）。 */
export function searchKeywordFromQuery(value: unknown): string {
  return typeof value === "string" ? value : "";
}

/** 明暗切换按钮的 aria-label：说清"点了会切到哪种"，而不是只说"切换主题"。 */
export function themeToggleLabel(theme: ThemeMode): string {
  return theme === "dark" ? "切换到浅色主题" : "切换到暗色主题";
}

/** 手机端折叠菜单按钮的 aria-label：反映当前展开状态。 */
export function mobileMenuLabel(open: boolean): string {
  return open ? "收起导航菜单" : "展开导航菜单";
}
