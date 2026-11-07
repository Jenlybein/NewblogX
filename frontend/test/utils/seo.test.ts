import { describe, expect, it } from "vitest";

import {
  articleJsonLd,
  breadcrumbJsonLd,
  canonicalArticleUrl,
  PRIVATE_PAGE_ROBOTS,
  webSiteJsonLd,
} from "~/utils/seo";

/**
 * 这一组用例钉的是 #314 的 ④⑤⑥ 三项（规范地址 / 分享卡片字段 / 结构化数据）。
 *
 * 为什么是纯函数：这三样全是"从一个已知事实推出一段文本"，不读浏览器状态、
 * 不打接口。页面只负责把实际值传进来，因此可以在这里的接缝上逐条摆出"对的事实
 * 进来 → 对的机器读数出去"，不必渲染整个文章页。
 *
 * 期望值一律写字面量（不拿被测函数回算期望）：这些字符串是 Google / OG 规范
 * 要求的样子，抄自规范而不是抄自实现。
 */

/** 读取 JSON-LD 的某个顶层字段：返回值是 `unknown`，比较用 `expect(...).toBe`。 */
function fieldOf(ld: unknown, key: string): unknown {
  return (ld as Record<string, unknown>)[key];
}

describe("canonicalArticleUrl：文章页的规范地址", () => {
  it("拼成 siteUrl + /article/{id}，恒不带查询串", () => {
    const canonical = canonicalArticleUrl("https://blogx.gentlybeing.cn", "12");

    expect(canonical).toBe("https://blogx.gentlybeing.cn/article/12");
    expect(canonical).not.toContain("?");
  });

  it("剥掉来路参数：从带 ?from= 的当前地址取到 id，规范地址里没有 from", () => {
    // 模拟文章页的真实输入：路由参数 id 来自地址栏，而地址栏可能带着
    // `?from=collection:5`（从合集目录点进来的痕迹）。规范地址只由 id 决定，
    // 因此同一条 URL 的多个来路变体收敛到同一个规范地址（去重才有意义）。
    const currentUrl = new URL("https://blogx.gentlybeing.cn/article/12?from=collection:5&utm_source=wechat");
    const id = currentUrl.pathname.split("/").filter(Boolean).pop() || "";
    const canonical = canonicalArticleUrl(currentUrl.origin, id);

    expect(canonical).toBe("https://blogx.gentlybeing.cn/article/12");
    expect(canonical).not.toContain("from=");
    expect(canonical).not.toContain("utm_source");
  });

  it("来自不同来路的两条变体收敛到同一个规范地址", () => {
    const fromCollection = canonicalArticleUrl("https://blogx.gentlybeing.cn", "12");
    const fromProject = canonicalArticleUrl("https://blogx.gentlybeing.cn/", "12");

    expect(fromCollection).toBe(fromProject);
  });

  it("siteUrl 带结尾斜杠或空白也拼不出双斜杠", () => {
    expect(canonicalArticleUrl(" https://blogx.gentlybeing.cn/ ", "12")).toBe(
      "https://blogx.gentlybeing.cn/article/12",
    );
  });

  it("没有配 siteUrl 时退化成站内相对地址，而不是拼出 ://", () => {
    expect(canonicalArticleUrl("", "12")).toBe("/article/12");
  });
});

describe("PRIVATE_PAGE_ROBOTS：私密页的收录指令", () => {
  it("是 noindex，不是 nofollow / none（只表达「别收录」，不替爬虫决定要不要跟链接）", () => {
    expect(PRIVATE_PAGE_ROBOTS).toBe("noindex");
  });
});

describe("articleJsonLd：文章结构化数据（Article）", () => {
  const facts = {
    url: "https://blogx.gentlybeing.cn/article/12",
    siteUrl: "https://blogx.gentlybeing.cn",
    siteName: "JellyBin（果冻盒）",
    siteLogoUrl: "https://blogx.gentlybeing.cn/brand/logo-256.png",
    title: "把阅读页做成一张纸",
    abstract: "一次只讲一件事情的版式改造。",
    cover: "/uploads/cover-12.png",
    createdAt: "2026-01-01T12:00:00Z",
    updatedAt: "2026-05-01T09:30:00Z",
    authorName: "张三",
    authorId: "901",
    tags: ["Go", "Nuxt"],
    categoryName: "技术",
  };

  it("必有字段：@type=Article、headline、datePublished、author、publisher", () => {
    expect(articleJsonLd(facts)).toMatchObject({
      "@context": "https://schema.org",
      "@type": "Article",
      headline: "把阅读页做成一张纸",
      datePublished: "2026-01-01T12:00:00Z",
      author: {
        "@type": "Person",
        name: "张三",
        url: "https://blogx.gentlybeing.cn/users/901",
      },
      publisher: {
        "@type": "Organization",
        name: "JellyBin（果冻盒）",
        logo: {
          "@type": "ImageObject",
          url: "https://blogx.gentlybeing.cn/brand/logo-256.png",
        },
      },
    });
  });

  it("封面转成绝对地址（搜索结果的缩略图必须能直接取到）", () => {
    expect(fieldOf(articleJsonLd(facts), "image")).toEqual([
      "https://blogx.gentlybeing.cn/uploads/cover-12.png",
    ]);
  });

  it("发布时间取 created_at、修改时间取 updated_at（没有 published_at 列，零 DDL）", () => {
    const ld = articleJsonLd(facts);

    expect(fieldOf(ld, "datePublished")).toBe(facts.createdAt);
    expect(fieldOf(ld, "dateModified")).toBe(facts.updatedAt);
  });

  it("标签与分类：tags 拼 keywords，分类没有独立页面所以只做 articleSection", () => {
    const ld = articleJsonLd(facts);

    expect(fieldOf(ld, "keywords")).toBe("Go, Nuxt");
    expect(fieldOf(ld, "articleSection")).toBe("技术");
    // 分类在站点里没有 URL（没有 /category 路由），所以绝不编一条路径出来：
    // 面包屑里不出现分类层，分类信息只走 articleSection。
    expect(JSON.stringify(ld)).not.toContain("/category");
  });

  it("mainEntityOfPage 就是规范地址（告诉引擎这一页的正式地址）", () => {
    expect(fieldOf(articleJsonLd(facts), "mainEntityOfPage")).toBe(
      "https://blogx.gentlybeing.cn/article/12",
    );
  });

  it("空字段不编造：没封面就不出 image，没标签就不出 keywords", () => {
    const ld = articleJsonLd({ ...facts, cover: "", tags: [], abstract: "" });

    expect(ld).not.toHaveProperty("image");
    expect(ld).not.toHaveProperty("keywords");
    expect(ld).not.toHaveProperty("description");
  });

  it("没有任何地方会漏出 undefined（序列化后必须是干净 JSON）", () => {
    const serialized = JSON.stringify(articleJsonLd(facts));

    expect(serialized).not.toContain("undefined");
    expect(JSON.parse(serialized)).toBeTypeOf("object");
  });
});

describe("breadcrumbJsonLd：面包屑（BreadcrumbList）", () => {
  it("两级：首页 → 文章标题", () => {
    expect(
      breadcrumbJsonLd({
        homeName: "首页",
        homeUrl: "https://blogx.gentlybeing.cn/",
        articleTitle: "把阅读页做成一张纸",
        articleUrl: "https://blogx.gentlybeing.cn/article/12",
      }),
    ).toMatchObject({
      "@context": "https://schema.org",
      "@type": "BreadcrumbList",
      itemListElement: [
        { "@type": "ListItem", position: 1, name: "首页", item: "https://blogx.gentlybeing.cn/" },
        {
          "@type": "ListItem",
          position: 2,
          name: "把阅读页做成一张纸",
          item: "https://blogx.gentlybeing.cn/article/12",
        },
      ],
    });
  });

  it("刻意不出现分类层：站点没有分类页，编一条 URL 等于给爬虫一条 404", () => {
    const items = fieldOf(
      breadcrumbJsonLd({
        homeName: "首页",
        homeUrl: "https://blogx.gentlybeing.cn/",
        articleTitle: "标题",
        articleUrl: "https://blogx.gentlybeing.cn/article/12",
      }),
      "itemListElement",
    ) as Record<string, unknown>[];

    expect(items).toHaveLength(2);
    expect(items.every((item) => typeof item.item === "string")).toBe(true);
  });
});

describe("webSiteJsonLd：站点声明 + 站内搜索（WebSite + SearchAction）", () => {
  const facts = {
    siteName: "JellyBin（果冻盒）",
    siteUrl: "https://blogx.gentlybeing.cn",
    searchPath: "/search",
    searchParam: "key",
  };

  it("声明站点本身", () => {
    expect(webSiteJsonLd(facts)).toMatchObject({
      "@context": "https://schema.org",
      "@type": "WebSite",
      name: "JellyBin（果冻盒）",
      url: "https://blogx.gentlybeing.cn/",
    });
  });

  it("SearchAction 的模板参数必须和搜索页真实读取的参数同名（?key=）", () => {
    expect(fieldOf(webSiteJsonLd(facts), "potentialAction")).toEqual({
      "@type": "SearchAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: "https://blogx.gentlybeing.cn/search?key={search_term_string}",
      },
      "query-input": "required name=search_term_string",
    });
  });
});
