import { useNuxtApp } from "#imports";
import type {
  BannerListData,
  SiteAiInfo,
  SiteDecoConfig,
  SiteRuntimeConfig,
  SiteSeoData,
  SiteSidebarCardsData,
} from "~/types/api";

export function getSiteRuntimeConfig() {
  return useNuxtApp().$api.request<SiteRuntimeConfig>("/api/site/site", {
    auth: false,
  });
}

export function getSiteSeo() {
  return useNuxtApp().$api.request<SiteSeoData>("/api/site/seo", {
    auth: false,
  });
}

export function getSiteAiInfo() {
  return useNuxtApp().$api.request<SiteAiInfo>("/api/site/ai_info", {
    auth: false,
  });
}

/**
 * 装饰配置（第 8 段 deco）。
 *
 * 公开读、无需登录；读者站必须在渲染挂件**之前**知道总开关，
 * 所以它和站点配置 / SEO / AI 信息并进同一批引导请求。
 */
export function getSiteDecoConfig() {
  return useNuxtApp().$api.request<SiteDecoConfig>("/api/site/deco", {
    auth: false,
  });
}

/**
 * 读者侧右栏卡片列表。
 *
 * 匿名可读；卡片种类、顺序、标题与各卡数据都在这一个响应里 ——
 * 读者站不再需要知道"热门标签/推荐作者"该从哪些接口拼。
 */
export function getSidebarCards() {
  return useNuxtApp().$api.request<SiteSidebarCardsData>("/api/site/sidebar_cards", {
    auth: false,
  });
}

export function getBannerList() {
  return useNuxtApp().$api.request<BannerListData>("/api/banners", {
    query: {
      show: true,
      page: 1,
      limit: 8,
    },
    auth: false,
  });
}
