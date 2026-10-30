import { describe, expect, it } from "vitest";

import {
  BRAND,
  BRAND_ASSETS,
  brandAbsoluteAssetUrl,
  brandAiName,
  brandAssetUrl,
  brandCopyright,
  brandDescription,
  brandFullName,
  brandProjectTitle,
  brandTitle,
} from "~/utils/brand";

/**
 * 这些用例钉的是「品牌单点定义」这件事本身：中文名 / 英文名 / 字标结构 /
 * 助手昵称 / 版权行的取值，以及空值回落的边界。组件与页面只消费这些函数，
 * 因此这里红了就说明全站品牌串漂了。
 */
describe("brand：品牌单点定义", () => {
  it("中英文名与字标结构：英文大字标 + 中文小字副标", () => {
    expect(BRAND.name.en).toBe("JellyBin");
    expect(BRAND.name.zh).toBe("果冻盒");
    expect(BRAND.wordmark.primary).toBe("JellyBin");
    expect(BRAND.wordmark.secondary).toBe("果冻盒");
    expect(brandFullName()).toBe("JellyBin（果冻盒）");
  });

  it("助手昵称统一为「捏捏」，站点配置的昵称优先", () => {
    expect(BRAND.assistant).toBe("捏捏");
    expect(brandAiName()).toBe("捏捏");
    expect(brandAiName(null)).toBe("捏捏");
    expect(brandAiName("   ")).toBe("捏捏");
    expect(brandAiName("  小果冻  ")).toBe("小果冻");
  });

  it("页面标题：空值回落品牌全名，已带品牌名不重复拼接，其余补品牌后缀", () => {
    expect(brandTitle()).toBe("JellyBin（果冻盒）");
    expect(brandTitle("   ")).toBe("JellyBin（果冻盒）");
    expect(brandTitle("JellyBin（果冻盒）")).toBe("JellyBin（果冻盒）");
    expect(brandTitle("JellyBin")).toBe("JellyBin");
    expect(brandTitle("  首页  ")).toBe("首页 · JellyBin");
  });

  it("描述回落：空值用品牌一句话介绍，有值则去除首尾空白", () => {
    expect(brandDescription()).toBe(BRAND.description);
    expect(brandDescription("  ")).toBe(BRAND.description);
    expect(brandDescription("  文章摘要  ")).toBe("文章摘要");
  });

  it("站点标题兜底：运行时有项目名用项目名，没有用英文品牌名", () => {
    expect(brandProjectTitle()).toBe("JellyBin");
    expect(brandProjectTitle(null)).toBe("JellyBin");
    expect(brandProjectTitle("  ")).toBe("JellyBin");
    expect(brandProjectTitle("  我的博客  ")).toBe("我的博客");
  });

  it("版权行带年份", () => {
    expect(brandCopyright(2026)).toBe("© 2026 JellyBin（果冻盒）");
    expect(brandCopyright(2027)).toBe("© 2027 JellyBin（果冻盒）");
  });

  it("站内资产路径跟随 app.baseURL（读者站是 /）", () => {
    expect(brandAssetUrl("/", "logo")).toBe("/brand/logo-256.png");
    expect(brandAssetUrl("", "flat")).toBe("/brand/jelly-flat.svg");
    expect(brandAssetUrl("/admin", "faviconRoot")).toBe("/admin/favicon.ico");
    expect(brandAssetUrl("/admin/", "og")).toBe("/admin/brand/og-image.png");
    expect(BRAND_ASSETS.og).toBe("brand/og-image.png");
  });

  it("绝对资产地址用于 OG / twitter（站点地址去尾部斜杠）", () => {
    expect(brandAbsoluteAssetUrl("https://blog.gentlybeing.cn", "og")).toBe(
      "https://blog.gentlybeing.cn/brand/og-image.png",
    );
    expect(brandAbsoluteAssetUrl("https://blog.gentlybeing.cn/", "og")).toBe(
      "https://blog.gentlybeing.cn/brand/og-image.png",
    );
    expect(brandAbsoluteAssetUrl("  ", "apple")).toBe("/brand/apple-touch-icon.png");
  });
});
