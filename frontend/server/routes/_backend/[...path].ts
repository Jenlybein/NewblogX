import { defineEventHandler, getRouterParam } from "h3";
import { proxyRoutePath } from "../../utils/upstreamProxy";

export default defineEventHandler((event) => {
  const runtimeConfig = useRuntimeConfig(event);
  const requestPath = getRouterParam(event, "path") || "";
  const upstream = runtimeConfig.apiUpstream;
  return proxyRoutePath(event, upstream);
});
