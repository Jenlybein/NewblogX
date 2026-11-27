import { useNuxtApp } from "#imports";
import type {
  BannerListData,
  LoginCapabilities,
  SiteAiInfo,
  SiteDecoConfig,
  SiteRuntimeConfig,
  SiteSeoData,
  SiteSidebarCardsData,
} from "~/types/api";

/**
 * 登录能力面（票 票 481）：登录方式可用性 + 四个入口要不要画验证码框。
 *
 * 为什么要单独一个端点：登录/发码/验证码这几类开关按通道拆进了 mail / sms /
 * oauth / captcha 四段，而**那四段含凭据、只有后台能读**。读者站原来靠公开的
 * `site` 段拿这些布尔位，拆段后就没有来源了。这个端点把布尔位现读一遍、只公开
 * 布尔位，凭据一个字节都不出 —— 比"让读者站直读后台段"安全，比"继续把开关塞回
 * site 段"干净。
 */
export function getLoginCapabilities() {
  return useNuxtApp().$api.request<LoginCapabilities>("/api/site/login_capabilities", {
    auth: false,
  });
}

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
 * 装饰配置（九段里的 deco 段，ADR-0020）。
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

