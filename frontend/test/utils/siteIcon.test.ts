import { describe, expect, it } from "vitest";

import {
  isPlaceholderSiteIcon,
  siteIconLinks,
  siteIconType,
} from "~/utils/siteIcon";

describe("isPlaceholderSiteIcon", () => {
  it("空值与纯空白视为占位", () => {
    expect(isPlaceholderSiteIcon(undefined)).toBe(true);
    expect(isPlaceholderSiteIcon(null)).toBe(true);
    expect(isPlaceholderSiteIcon("")).toBe(true);
    expect(isPlaceholderSiteIcon("   ")).toBe(true);
  });

  it("后端 SEO 的占位串视为占位（线上曾因此让 favicon 404）", () => {
    expect(isPlaceholderSiteIcon("/xxx.png")).toBe(true);
  });

  it("真实路径不算占位", () => {
    expect(isPlaceholderSiteIcon("/uploads/icon.png")).toBe(false);
    expect(isPlaceholderSiteIcon("https://cdn.example.com/icon.svg")).toBe(false);
  });
});

describe("siteIconType", () => {
  it("按扩展名给出 type", () => {
    expect(siteIconType("/a.svg")).toBe("image/svg+xml");
    expect(siteIconType("/a.ico")).toBe("image/x-icon");
    expect(siteIconType("/a.png")).toBe("image/png");
    expect(siteIconType("/a.JPEG")).toBe("image/jpeg");
  });

  it("忽略 query 与 hash", () => {
    expect(siteIconType("/a.svg?v=2")).toBe("image/svg+xml");
    expect(siteIconType("/a.png#frag")).toBe("image/png");
  });

  it("认不出的扩展名返回 undefined（交给浏览器嗅探）", () => {
    expect(siteIconType("/icon")).toBeUndefined();
    expect(siteIconType("/icon.avif")).toBeUndefined();
  });
});

describe("siteIconLinks", () => {
  it("占位或空值时不发链接，让品牌图标生效", () => {
    expect(siteIconLinks(undefined)).toEqual([]);
    expect(siteIconLinks("")).toEqual([]);
    expect(siteIconLinks("/xxx.png")).toEqual([]);
  });

  it("真实 svg 覆盖会带上正确的 type", () => {
    expect(siteIconLinks("/uploads/icon.svg")).toEqual([
      { rel: "icon", type: "image/svg+xml", href: "/uploads/icon.svg" },
    ]);
  });

  it("绝对地址同样支持", () => {
    expect(siteIconLinks("https://cdn.example.com/favicon.ico")).toEqual([
      { rel: "icon", type: "image/x-icon", href: "https://cdn.example.com/favicon.ico" },
    ]);
  });

  it("无法识别扩展名时仍发链接，但不带 type", () => {
    expect(siteIconLinks("/notifications/icon")).toEqual([
      { rel: "icon", href: "/notifications/icon" },
    ]);
  });

  it("首尾空白被裁掉", () => {
    expect(siteIconLinks("  /uploads/icon.png  ")).toEqual([
      { rel: "icon", type: "image/png", href: "/uploads/icon.png" },
    ]);
  });
});
