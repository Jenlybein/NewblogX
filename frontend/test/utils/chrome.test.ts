import { readFileSync } from "node:fs";
import { join } from "node:path";

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
  it("五个入口：首页 / 搜索 / 创作 / 合集 / 项目，顺序固定", () => {
    // 「创作」仍是导航数据的一项（路由契约、活动判定与发布门控都依赖它），
    // 只是渲染点从导航条移到右侧动作区（#198）。
    expect(headerNavItems("/").map((item) => [item.key, item.label, item.to])).toEqual([
      ["home", "首页", "/"],
      ["search", "搜索", "/search"],
      ["write", "创作", "/studio/write"],
      ["collections", "合集", "/collections"],
      ["project", "项目", "/projects"],
    ]);
  });

  it("首页只在根路径激活（子路径不抢首页的高亮）", () => {
    const items = headerNavItems("/article/a1");

    expect(items.map((item) => [item.key, item.active])).toEqual([
      ["home", false],
      ["search", false],
      ["write", false],
      ["collections", false],
      ["project", false],
    ]);
  });

  it("搜索页及其子路径激活搜索", () => {
    expect(headerNavItems("/search").find((item) => item.key === "search")?.active).toBe(true);
  });

  it("创作区激活创作（带 query 也算）", () => {
    const write = headerNavItems("/studio/write").find((item) => item.key === "write");

    expect(write?.active).toBe(true);
  });

  it("合集页及其子路径激活合集", () => {
    expect(headerNavItems("/collections").find((item) => item.key === "collections")?.active).toBe(
      true,
    );
    expect(
      headerNavItems("/collections/c1").find((item) => item.key === "collections")?.active,
    ).toBe(true);
  });

  it("项目页及其子路径激活项目（合集的高亮不被项目抢走）", () => {
    expect(headerNavItems("/projects").find((item) => item.key === "project")?.active).toBe(true);
    expect(headerNavItems("/projects/p1").find((item) => item.key === "project")?.active).toBe(true);
    expect(headerNavItems("/projects").find((item) => item.key === "collections")?.active).toBe(
      false,
    );
  });

  it("每个路径最多只有一个 active", () => {
    for (const path of [
      "/",
      "/search",
      "/studio/write",
      "/collections",
      "/collections/c1",
      "/projects",
      "/projects/p1",
      "/article/a1",
      "/users/u1",
    ]) {
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

  it("页脚不直接引用 FOOTER_ICP：备案号改读运行时站点配置（#191）", () => {
    const footerSource = readFileSync(
      join(process.cwd(), "app/components/layout/AppFooter.vue"),
      "utf8",
    );

    // 常量本身留给后续票删除，但页脚不能再引用它——否则「不再硬编码」无从谈起。
    expect(footerSource).not.toContain("FOOTER_ICP");
    expect(footerSource).toContain("useSiteStore");
    expect(FOOTER_ICP).toMatch(/^粤ICP备 \d+ 号$/u);
  });
});

