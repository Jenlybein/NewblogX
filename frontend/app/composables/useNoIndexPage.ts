import { PRIVATE_PAGE_ROBOTS } from "~/utils/seo";

/**
 * 私密页的「可以爬、别收录」声明（#314 交付 3）。
 *
 * 为什么是页面级 meta 而不是 robots.txt 的 Disallow：被 Disallow 的页面爬虫根本
 * 进不去，也就读不到页面里的 `noindex`；若别处有链接指向它，它仍可能以"裸 URL
 * 无标题"出现在结果里。`noindex` 是"我允许你看，但别放进搜索结果"，
 * 两份指令的分工见发布闸门与机审设计口径 §6.2③。
 *
 * 为什么值得抽成 composable：这份清单会随页面增加而变长，而"新加一个 /studio 子页
 * 时记得加 robots"是最容易漏的一步。一条 `useNoIndexPage()` 比每个页面各写一遍
 * `useSeoMeta({ robots: "noindex" })` 更难写错，也让
 * `test/gates/seo-foundation.test.ts` 可以按路由清单逐页检查有没有漏。
 *
 * 注意：这些页面都是 `ssr: false`（ADR-0016 决策 1），标签在客户端水合后才由
 * unhead 写进 head —— 会渲染 JS 的爬虫能看到，不渲染 JS 的爬虫本来也只看到空壳。
 */
export function useNoIndexPage() {
  useSeoMeta({ robots: PRIVATE_PAGE_ROBOTS });
}
