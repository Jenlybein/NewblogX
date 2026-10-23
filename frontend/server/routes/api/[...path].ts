import { defineEventHandler } from "h3";
import { proxyRoutePath } from "../../utils/upstreamProxy";

export default defineEventHandler((event) => {
  const runtimeConfig = useRuntimeConfig(event);
  return proxyRoutePath(event, runtimeConfig.apiUpstream, "api");
});
