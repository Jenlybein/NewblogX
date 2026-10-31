import type {
  BannerListData,
  SiteAiInfo,
  SiteDecoConfig,
  SiteRuntimeConfig,
  SiteSeoData,
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
