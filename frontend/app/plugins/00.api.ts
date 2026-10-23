import { createApiClient } from "~/services/http/client";

export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig();
  const normalizedApiBase = String(config.public.apiBase || "/api").trim() || "/api";

  if (!normalizedApiBase.startsWith("/")) {
    throw new Error(`[AuthGuard] apiBase 必须保持同源相对路径，当前值=${normalizedApiBase}`);
  }

  return {
    provide: {
      api: createApiClient(normalizedApiBase),
    },
  };
});
