import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

// 票 #19 的「退场断言」：暗色专用 Logo 变体必须真的消失，而不是只在页头藏起来。
//
// 为什么用静态扫描 + PNG 头解析而不是渲染断言：删除后的正确状态是
// "文件与资产键根本不存在"，而"两套主题统一用透底资产"这件事的证据是
// 资产文件自身的 alpha 通道（IHDR color type 6 = RGBA）。渲染断言只能
// 证明某个主题当前没画白底图，证明不了下一个组件不会又把它引回来。

const projectDir = process.cwd();
const adminDir = join(projectDir, "..", "frontend_admin");
const readerBrandModule = readFileSync(join(projectDir, "app/utils/brand.ts"), "utf8");
const adminBrandModule = readFileSync(join(adminDir, "app/utils/brand.ts"), "utf8");

/** 读 PNG 的 IHDR color type：6 = RGBA（带 alpha），2 = RGB（无 alpha）。 */
function pngColorType(path: string): number {
  const bytes = readFileSync(path);
  expect(bytes.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
  expect(bytes.subarray(12, 16).toString("ascii")).toBe("IHDR");
  return bytes[25]!;
}

describe("品牌资产：暗色专用变体已退场（票 #19）", () => {
  it("阅读站与后台都不再有无 alpha 通道的白底 PNG", () => {
    expect(existsSync(join(projectDir, "public/brand/logo-512-white.png"))).toBe(false);
    expect(existsSync(join(adminDir, "public/brand/logo-512-white.png"))).toBe(false);
  });

  it("统一的 logo 资产带 alpha 通道（亮暗两态共用同一张透底图）", () => {
    expect(pngColorType(join(projectDir, "public/brand/logo-256.png"))).toBe(6);
    expect(pngColorType(join(adminDir, "public/brand/logo-256.png"))).toBe(6);
  });

  it("两端品牌模块的资产清单里没有白色变体键与白底资产路径", () => {
    for (const source of [readerBrandModule, adminBrandModule]) {
      expect(source).not.toMatch(/logoWhite/u);
      expect(source).not.toMatch(/logo-512-white/u);
    }
  });

  it("两端 Logo 组件只接受「通用/扁平」两种形态", () => {
    const readerLogo = readFileSync(join(projectDir, "app/components/brand/Logo.vue"), "utf8");
    const adminLogo = readFileSync(join(adminDir, "app/components/admin/BrandLogo.vue"), "utf8");

    for (const source of [readerLogo, adminLogo]) {
      expect(source).toMatch(/variant\?:\s*"render"\s*\|\s*"flat"/u);
      expect(source).not.toMatch(/logoWhite|logo-512-white/u);
    }
  });

  it("页头源码里没有按主题挑 Logo 的逻辑", () => {
    const header = readFileSync(join(projectDir, "app/components/layout/AppHeader.vue"), "utf8");

    expect(header).not.toMatch(/logoWhite|logo-512-white/u);
    expect(header).not.toMatch(/theme\s*===\s*["']dark["'][^\n]*["']white["']/u);
  });
});
