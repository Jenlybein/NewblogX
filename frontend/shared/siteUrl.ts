/**
 * 站点根地址归一：去空白、去结尾斜杠。
 *
 * 为什么单独一个模块：`siteUrl` 会被两处消费 —— 读者站的 canonical / JSON-LD
 * （`app/utils/seo.ts`）与 robots.txt 的 `Sitemap:` 行（`shared/robotsTxt.ts` + Nitro
 * 路由）。两处各写一遍 `trim().replace(/\/+$/)` 迟早会有一处改成"只去一个斜杠"，
 * 于是某天生产环境出现 `//article/12` 或 `Sitemap: https://x//api/sitemap.xml`。
 *
 * 必须是纯 TS（不 import Nuxt / Vue）：Nitro 路由、`app/utils` 与单测三边共用。
 */
export function siteOrigin(siteUrl?: string | null): string {
  return String(siteUrl ?? "")
    .trim()
    .replace(/\/+$/u, "");
}
