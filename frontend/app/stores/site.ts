import {
  getLoginCapabilities,
  getSiteAiInfo,
  getSiteDecoConfig,
  getSiteRuntimeConfig,
  getSiteSeo,
} from "~/services/site";
import type {
  LoginCapabilities,
  SiteAiInfo,
  SiteDecoConfig,
  SiteRuntimeConfig,
  SiteSeoData,
} from "~/types/api";

export const useSiteStore = defineStore("site", () => {
  const runtimeConfig = ref<SiteRuntimeConfig | null>(null);
  const seo = ref<SiteSeoData | null>(null);
  const aiInfo = ref<SiteAiInfo | null>(null);
  // deco 段（九段之一，装饰）：读者站必须先知道总开关，才知道要不要挂光标挂件。
  // 它和站点配置 / SEO / AI 信息同一批并发取回，不新增一轮串行往返。
  const deco = ref<SiteDecoConfig | null>(null);
  // 登录能力面（票 481）：哪些登录方式可用、哪些入口要画验证码框。
  // 它与上面四项同一批并发取回：登录弹窗与四个发码表单都要在"第一帧"就拿到答案，
  // 晚一步就会出现"表单画好了、验证码框却半天后才出现"的跳动。
  const loginCapabilities = ref<LoginCapabilities | null>(null);
  const fetched = shallowRef(false);

  // 同一批引导可能被两处同时触发（布局 + 应用根层的挂件），
  // 而 `fetched` 要等响应回来才置真 —— 没有这道单飞闸门，首屏会发两整批请求。
  let inFlight: Promise<void> | null = null;

  async function ensurePublicBootstrap() {
    if (
      fetched.value &&
      runtimeConfig.value &&
      seo.value &&
      aiInfo.value &&
      deco.value &&
      loginCapabilities.value
    ) {
      return;
    }
    if (inFlight) return inFlight;

    inFlight = (async () => {
      const [runtime, seoData, ai, decoData, capabilities] = await Promise.allSettled([
        getSiteRuntimeConfig(),
        getSiteSeo(),
        getSiteAiInfo(),
        getSiteDecoConfig(),
        getLoginCapabilities(),
      ]);

      runtimeConfig.value =
        runtime.status === "fulfilled" ? runtime.value : runtimeConfig.value;
      seo.value = seoData.status === "fulfilled" ? seoData.value : seo.value;
      aiInfo.value = ai.status === "fulfilled" ? ai.value : aiInfo.value;
      deco.value = decoData.status === "fulfilled" ? decoData.value : deco.value;
      loginCapabilities.value =
        capabilities.status === "fulfilled" ? capabilities.value : loginCapabilities.value;
      fetched.value = true;
    })().finally(() => {
      inFlight = null;
    });

    return inFlight;
  }

  return {
    runtimeConfig,
    seo,
    aiInfo,
    deco,
    loginCapabilities,
    fetched,
    ensurePublicBootstrap,
  };
});

