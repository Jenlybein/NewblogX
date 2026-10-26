import type { BannerListData, SiteAiInfo, SiteRuntimeConfig, SiteSeoData } from "~/types/api";

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
