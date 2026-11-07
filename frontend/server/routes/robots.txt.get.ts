import { buildRobotsTxt } from "../../shared/robotsTxt";

/**
 * `GET /robots.txt`（#314 交付 2）。
 *
 * 为什么是路由而不是 `public/robots.txt`：`Sitemap:` 行必须是绝对地址，域名来自
 * 运行时 `NUXT_PUBLIC_SITE_URL`（local / test / production 三个不同域名）。静态
 * 文件写不出"当前这台机器"的那一个，所以每次请求按运行时配置现拼。
 *
 * 缓存：nginx 对 `location /` 统一发 `no-cache`，这份十几行的文本每次现拼成本可以
 * 忽略；不加内存缓存也就不需要"改站点了记得清缓存"这种运维知识。
 */
export default defineEventHandler((event) => {
  const siteUrl = String(useRuntimeConfig(event).public.siteUrl || "");

  setHeader(event, "Content-Type", "text/plain; charset=utf-8");
  return buildRobotsTxt(siteUrl);
});
