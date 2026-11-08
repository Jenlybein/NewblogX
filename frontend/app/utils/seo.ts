/**
 * 文章页 / 首页的机器读数（SEO 技术件，#314 交付 4–6）的**纯函数**落点。
 *
 * 为什么收成一个纯 TS 模块：canonical、OG 字段、JSON-LD 三样都是"已知事实 → 一段
 * 文本 / 一个对象"的转换，不读浏览器状态、不打接口、不碰路由。把它们从页面里拎出来
 * 有两个好处：
 *   1. 页面只负责提供事实（标题、时间、作者、标签），很容易看出"页面有没有喂对值"；
 *   2. 期望值是**规范**要求的字面量（schema.org / OG），可以在单测里逐条核对，
 *      不必渲染整个文章页才能验证 head 里写了什么。
 *
 * 刻意不做的事：不在这里读 `useRequestURL()`。规范地址必须由配置里的 `siteUrl`
 * 决定 —— 同一篇文章的 `?from=collection:5` / `?utm_source=…` 变体分别命中不同的
 * SSR 缓存键，但它们**必须**输出同一条 canonical，否则"去重"这件事本身会去重失败。
 * 配置给的地址是唯一稳定的那一份。
 */

import { siteOrigin } from "#shared/siteUrl";

/** 私密页的统一 robots 指令：可以不登录先看到空壳，但别收录（docs §6.2③）。 */
export const PRIVATE_PAGE_ROBOTS = "noindex";

/** JSON-LD 对象的最小形态：`@context` + `@type`，其余字段由各类型自己长。 */
export type JsonLd = Record<string, unknown>;

/**
 * 站点根地址归一。实现放在 `shared/siteUrl.ts`：robots.txt 的 `Sitemap:` 行要用
 * 同一份归一（否则两处对"尾斜杠"的理解迟早分叉），这里转出手给页面侧用。
 */
export { siteOrigin };
/** 站内相对路径（或已经是绝对地址的字符串）→ 绝对地址。空值返回空串。 */
export function absoluteUrl(siteUrl: string | null | undefined, path?: string | null): string {
  const value = String(path ?? "").trim();
  if (!value) return "";
  if (/^https?:\/\//iu.test(value)) return value;

  const origin = siteOrigin(siteUrl);
  const suffix = value.replace(/^\/+/u, "");
  return origin ? `${origin}/${suffix}` : `/${suffix}`;
}

/**
 * 文章页的规范地址：`<siteUrl>/article/<id>`。
 *
 * **恒不带查询串**是这条函数存在的全部理由：地址栏里可能挂着 `?from=collection:5`
 * （从合集目录点进来的来路参数），它只影响**页面行为**（左侧抽屉显示哪棵树），
 * 不影响"这一页的正式地址"。因此规范地址由 id 拼出来，来路参数根本没有机会进入
 * 字符串 —— 而不是拼完再删，删漏一个变体就有一篇文章变成"重复内容"。
 *
 * id 为空时（正文还没回来）退回站点首页：宁可暂时给一个**真实存在**的地址，
 * 也不在 head 里写一条 404 的 canonical。
 */
export function canonicalArticleUrl(siteUrl: string | null | undefined, articleId?: string | null): string {
  const origin = siteOrigin(siteUrl);
  const id = String(articleId ?? "").trim();
  if (!id) return origin ? `${origin}/` : "/";
  return `${origin}/article/${encodeURIComponent(id)}`;
}

/** 文章结构化数据（schema.org Article）需要的事实。全部来自详情接口，缺就是缺。 */
export interface ArticleJsonLdFacts {
  /** 规范地址（canonicalArticleUrl 的产物）。 */
  url: string;
  /** 站点根地址，用于把作者主页 / 封面拼成绝对地址。 */
  siteUrl?: string | null;
  siteName: string;
  siteLogoUrl?: string | null;
  title: string;
  abstract?: string | null;
  cover?: string | null;
  /** 取 `created_at`：库里没有 `published_at` 列（零 DDL 硬约束）。 */
  createdAt?: string | null;
  updatedAt?: string | null;
  authorName?: string | null;
  authorId?: string | null;
  tags?: readonly string[] | null;
  categoryName?: string | null;
}

/**
 * 文章结构化数据。
 *
 * 只写我们**真的有**的字段：封面为空就不出 `image`，标签为空就不出 `keywords`。
 * 编造一个空字符串或占位图会让 Google 的富摘要校验器直接报错，而被拒的富摘要
 * 连普通蓝链的待遇都不如。
 *
 * 分类只进 `articleSection`、不进面包屑：站点没有分类页（没有 `/category/*` 路由），
 * 给面包屑编一条分类 URL 等于把爬虫送去一页 404。
 */
export function articleJsonLd(facts: ArticleJsonLdFacts): JsonLd {
  const ld: JsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: facts.title,
    mainEntityOfPage: facts.url,
    url: facts.url,
    publisher: {
      "@type": "Organization",
      name: facts.siteName,
      ...(facts.siteLogoUrl
        ? { logo: { "@type": "ImageObject", url: facts.siteLogoUrl } }
        : {}),
    },
  };

  const description = String(facts.abstract ?? "").trim();
  if (description) ld.description = description;

  const image = absoluteUrl(facts.siteUrl, facts.cover);
  if (image) ld.image = [image];

  if (facts.createdAt) ld.datePublished = facts.createdAt;
  if (facts.updatedAt) ld.dateModified = facts.updatedAt;

  const authorName = String(facts.authorName ?? "").trim();
  if (authorName) {
    const authorId = String(facts.authorId ?? "").trim();
    ld.author = {
      "@type": "Person",
      name: authorName,
      ...(authorId ? { url: `${siteOrigin(facts.siteUrl)}/users/${encodeURIComponent(authorId)}` } : {}),
    };
  }

  const keywords = (facts.tags ?? []).map((tag) => String(tag).trim()).filter(Boolean);
  if (keywords.length > 0) ld.keywords = keywords.join(", ");

  const category = String(facts.categoryName ?? "").trim();
  if (category) ld.articleSection = category;

  return ld;
}

/** 面包屑需要的事实。 */
export interface BreadcrumbJsonLdFacts {
  homeName: string;
  homeUrl: string;
  articleTitle: string;
  articleUrl: string;
}

/**
 * 面包屑。**只有两级**：首页 → 文章标题。
 *
 * 设计方案里画的是"站点 › 分类 › 标题"，但站点没有分类页。Google 要求面包屑的
 * 每一层都有一个可访问的 `item`；为了凑出三层而编一条分类 URL，结果只会是一页
 * 404 —— 校验器报错、爬虫白跑。所以这里如实给两级，分类信息改由
 * {@link articleJsonLd} 的 `articleSection` 表达。
 */
export function breadcrumbJsonLd(facts: BreadcrumbJsonLdFacts): JsonLd {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: facts.homeName, item: facts.homeUrl },
      { "@type": "ListItem", position: 2, name: facts.articleTitle, item: facts.articleUrl },
    ],
  };
}

/** 站点声明需要的事实。 */
export interface WebSiteJsonLdFacts {
  siteName: string;
  siteUrl?: string | null;
  /** 搜索页路径，默认 `/search`。 */
  searchPath?: string;
  /** 搜索页真实读取的查询参数名，默认 `key`（见 app/pages/search.vue）。 */
  searchParam?: string;
}

/**
 * 站点声明 + 站内搜索。
 *
 * `urlTemplate` 里的参数名必须是搜索页**真的读**的那一个（`?key=`）。写错名字不会
 * 报错，只会让"在结果里直接搜索"这个动作跳到一页空结果。
 */
export function webSiteJsonLd(facts: WebSiteJsonLdFacts): JsonLd {
  const origin = siteOrigin(facts.siteUrl);
  const searchPath = facts.searchPath ?? "/search";
  const searchParam = facts.searchParam ?? "key";

  return {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: facts.siteName,
    url: `${origin}/`,
    potentialAction: {
      "@type": "SearchAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: `${origin}${searchPath}?${searchParam}={search_term_string}`,
      },
      "query-input": "required name=search_term_string",
    },
  };
}
