import { siteOrigin } from "./siteUrl";

/**
 * robots.txt 的策略（#314 交付 2）：纯函数，由 `server/routes/robots.txt.get.ts`
 * 按运行时 `siteUrl` 现拼成文本。
 *
 * 为什么不是 `public/robots.txt` 静态文件：`Sitemap:` 行必须是**绝对地址**
 * （协议要求），而本站域名来自 `NUXT_PUBLIC_SITE_URL` —— local / test / production
 * 是三个不同域名。静态文件只能写死一个，于是测试环境会把爬虫指向生产站的
 * sitemap。放在 `shared/` 而不是 `server/` 是为了让门禁单测直接 import 它。
 *
 * 只 Disallow `/api/`：那一面没有"页面"、没有 head 可读，爬它纯属浪费爬取预算。
 * **私密页面（/studio、/messages、/history、/search）刻意不在这里 Disallow** ——
 * 被 Disallow 的页面爬虫进不去，也就读不到页面里的 `noindex`；若别处有链接指向
 * 它，它仍可能以"裸 URL 无标题"出现在结果里。那几页改由页面级 meta 声明，
 * 理由见发布闸门与机审设计口径 §6.2③。
 */

/** 站点地图的路径（后端新端点，挂 /api 下）。 */
export const ROBOTS_SITEMAP_PATH = "/api/sitemap.xml";

/** 允许抓取的除外项：只有没有页面形态的接口面。 */
export const ROBOTS_DISALLOW_PATHS = ["/api/"] as const;

/**
 * 在 Disallow 之外**显式放行**的路径。
 *
 * 站点地图本身就在 `/api/` 下面：只写 `Disallow: /api/` 会把它一起关掉，
 * 等于告诉爬虫"去取一份你不许取的清单"。robots 的匹配按**最长前缀**取胜，
 * 因此这里写完整路径就能让它重新可抓——不必为 sitemap 挪位置，也就不会和
 * "nginx 只转发 /api/ 与 /uploads/"这条部署约束打架。
 */
export const ROBOTS_ALLOW_PATHS = [ROBOTS_SITEMAP_PATH] as const;

/**
 * 拼出 robots.txt 全文。
 *
 * 行序按通行写法：User-agent 分组 → Allow → Disallow → 空行 → Sitemap。
 * 结尾必须留换行：robots.txt 是逐行协议，最后一行不闭合时部分爬虫会忽略它。
 */
export function buildRobotsTxt(siteUrl?: string | null): string {
  const origin = siteOrigin(siteUrl);
  const sitemap = `${origin}${ROBOTS_SITEMAP_PATH}`;

  const lines = [
    "User-agent: *",
    ...ROBOTS_ALLOW_PATHS.map((path) => `Allow: ${path}`),
    ...ROBOTS_DISALLOW_PATHS.map((path) => `Disallow: ${path}`),
    "",
    `Sitemap: ${sitemap}`,
    "",
  ];

  return lines.join("\n");
}
