import { getSiteAiInfo, getSiteRuntimeConfig, getSiteSeo } from "~/services/site";
import type { SiteAiInfo, SiteRuntimeConfig, SiteSeoData } from "~/types/api";

export const useSiteStore = defineStore("site", () => {
  const runtimeConfig = ref<SiteRuntimeConfig | null>(null);
  const seo = ref<SiteSeoData | null>(null);
  const aiInfo = ref<SiteAiInfo | null>(null);
  const fetched = shallowRef(false);

  async function ensurePublicBootstrap() {
    if (fetched.value && runtimeConfig.value && seo.value && aiInfo.value) {
      return;
    }

    const [runtime, seoData, ai] = await Promise.allSettled([
      getSiteRuntimeConfig(),
      getSiteSeo(),
      getSiteAiInfo(),
    ]);

    runtimeConfig.value = runtime.status === "fulfilled" ? runtime.value : runtimeConfig.value;
    seo.value = seoData.status === "fulfilled" ? seoData.value : seo.value;
    aiInfo.value = ai.status === "fulfilled" ? ai.value : aiInfo.value;
    fetched.value = true;
  }

  return {
    runtimeConfig,
    seo,
    aiInfo,
    fetched,
    ensurePublicBootstrap,
  };
});
