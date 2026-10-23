import type { BannerItem, SiteAiInfo, SiteRuntimeConfig, SiteSeoData } from "~/types/api";

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
  return useNuxtApp().$api.request<{ list: BannerItem[]; has_more: boolean }>("/api/banners", {
    query: {
      show: true,
      page: 1,
      limit: 8,
    },
    auth: false,
  });
}
