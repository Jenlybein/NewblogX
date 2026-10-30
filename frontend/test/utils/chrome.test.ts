import { describe, expect, it } from "vitest";

import {
  FOOTER_ICP,
  buildSearchLocation,
  footerNavItems,
  headerNavItems,
  mobileMenuLabel,
  searchKeywordFromQuery,
  themeToggleLabel,
} from "~/utils/chrome";

/**
 * 页头/页脚的纯逻辑（从模板里抽出来，便于单测）。
 *
 * 这里钉的是**可观察契约**：导航条目与 active 判定、搜索跳转的 URL 形状
 * （路由是既有契约，不能因为重排而变）、明暗切换的无障碍文案、页脚链接与备案行。
 */
describe("headerNavItems：图纸式页头的导航与 active 判定", () => {
  it("三个入口：首页 / 搜索 / 创作，顺序固定", () => {
    expect(headerNavItems("/").map((item) => [item.key, item.label, item.to])).toEqual([
      ["home", "首页", "/"],
      ["search", "搜索", "/search"],
      ["write", "创作", "/studio/write"],
    ]);
  });

  it("首页只在根路径激活（子路径不抢首页的高亮）", () => {
    const items = headerNavItems("/article/a1");

    expect(items.map((item) => [item.key, item.active])).toEqual([
      ["home", false],
      ["search", false],
      ["write", false],
    ]);
  });

  it("搜索页及其子路径激活搜索", () => {
    expect(headerNavItems("/search").find((item) => item.key === "search")?.active).toBe(true);
  });

  it("创作区激活创作（带 query 也算）", () => {
    const write = headerNavItems("/studio/write").find((item) => item.key === "write");

    expect(write?.active).toBe(true);
  });

  it("每个路径最多只有一个 active", () => {
    for (const path of ["/", "/search", "/studio/write", "/article/a1", "/users/u1"]) {
      expect(headerNavItems(path).filter((item) => item.active).length).toBeLessThanOrEqual(1);
    }
  });
});

describe("搜索：URL 形状与既有路由保持一致（不新增/不改路径）", () => {
  it("有关键词时带 ?key=", () => {
    expect(buildSearchLocation("  果冻  ")).toEqual({ path: "/search", query: { key: "果冻" } });
  });

  it("空关键词时只跳搜索页，不带空参数", () => {
    expect(buildSearchLocation("")).toEqual({ path: "/search", query: {} });
    expect(buildSearchLocation("   ")).toEqual({ path: "/search", query: {} });
  });

  it("从路由 query 回填关键词：只认字符串，其它类型当空", () => {
    expect(searchKeywordFromQuery("AI")).toBe("AI");
    expect(searchKeywordFromQuery(undefined)).toBe("");
    expect(searchKeywordFromQuery(null)).toBe("");
    expect(searchKeywordFromQuery(42)).toBe("");
    expect(searchKeywordFromQuery(["a", "b"])).toBe("");
  });
});

describe("无障碍文案", () => {
  it("明暗切换按钮的 aria-label 说明「将切换到哪种」", () => {
    expect(themeToggleLabel("light")).toBe("切换到暗色主题");
    expect(themeToggleLabel("dark")).toBe("切换到浅色主题");
  });

  it("手机端菜单按钮的 aria-label 反映展开状态", () => {
    expect(mobileMenuLabel(false)).toBe("展开导航菜单");
    expect(mobileMenuLabel(true)).toBe("收起导航菜单");
  });
});

describe("页脚：少量链接 + 备案行", () => {
  it("只放首页与搜索两个站内链接（不铺一整张地图）", () => {
    expect(footerNavItems().map((item) => [item.label, item.to])).toEqual([
      ["首页", "/"],
      ["搜索", "/search"],
    ]);
  });

  it("备案行集中在一处（组件不再各自硬编码）", () => {
    expect(FOOTER_ICP).toMatch(/^粤ICP备 \d+ 号$/u);
  });
});
