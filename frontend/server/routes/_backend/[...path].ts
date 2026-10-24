import { defineEventHandler } from "h3";
import { proxyRoutePath } from "../../utils/upstreamProxy";

export default defineEventHandler((event) => {
  const runtimeConfig = useRuntimeConfig(event);
  const upstream = runtimeConfig.apiUpstream;
  return proxyRoutePath(event, upstream);
});
