import { describe, expect, it } from "vitest";

import {
  DEFAULT_THEME_MODE,
  THEME_COOKIE_NAME,
  THEME_MEDIA_QUERY,
  buildThemeInitScript,
  isThemeMode,
  parseThemeCookie,
  resolveThemeMode,
  resolveThemePreference,
} from "~/utils/themePreference";

/**
 * 初始偏好解析（issue #12 硬要求：系统偏好 vs cookie 的单测）。
 *
 * 这里钉的是优先级：用户选择 > 系统偏好 > 兜底浅色；以及"脏 cookie 不能把站点带歪"。
 */
describe("parseThemeCookie：只认合法值", () => {
  it("light / dark 原样返回", () => {
    expect(parseThemeCookie("light")).toBe("light");
    expect(parseThemeCookie("dark")).toBe("dark");
  });

  it("未设置或脏值一律当没选择（回落到系统偏好）", () => {
    for (const value of [null, undefined, "", "DARK", "auto", 0, {}, ["dark"]]) {
      expect(parseThemeCookie(value)).toBeNull();
    }
  });

  it("cookie 名沿用历史值 blogx-theme（改名会让老用户的选择丢失）", () => {
    expect(THEME_COOKIE_NAME).toBe("blogx-theme");
  });
});

describe("resolveThemePreference：用户选择 > 系统偏好 > 兜底", () => {
  it("用户切过：cookie 说了算，系统偏好不能翻盘", () => {
    expect(resolveThemePreference({ cookie: "light", systemPrefersDark: true })).toEqual({
      theme: "light",
      source: "user",
    });
    expect(resolveThemePreference({ cookie: "dark", systemPrefersDark: false })).toEqual({
      theme: "dark",
      source: "user",
    });
  });

  it("从没切过：跟随系统偏好", () => {
    expect(resolveThemePreference({ systemPrefersDark: true })).toEqual({ theme: "dark", source: "system" });
    expect(resolveThemePreference({ cookie: null, systemPrefersDark: true })).toEqual({
      theme: "dark",
      source: "system",
    });
    expect(resolveThemePreference({ systemPrefersDark: false })).toEqual({ theme: "light", source: "system" });
  });

  it("SSR / 读不到系统偏好：兜底浅色（与改造前的默认一致）", () => {
    expect(resolveThemePreference({})).toEqual({ theme: DEFAULT_THEME_MODE, source: "default" });
    expect(resolveThemeMode({ cookie: "not-a-theme" })).toBe(DEFAULT_THEME_MODE);
  });

  it("isThemeMode 只对 light/dark 为真", () => {
    expect(isThemeMode("dark")).toBe(true);
    expect(isThemeMode("light")).toBe(true);
    expect(isThemeMode("Dark")).toBe(false);
    expect(isThemeMode(undefined)).toBe(false);
  });
});

describe("buildThemeInitScript：首屏前置脚本", () => {
  const script = buildThemeInitScript();

  it("同一份判定进浏览器：读 cookie → 系统偏好 → 浅色", () => {
    expect(script).toContain(THEME_COOKIE_NAME);
    expect(script).toContain(THEME_MEDIA_QUERY);
    expect(script).toContain('"light"');
    expect(script).toContain('"dark"');
  });

  it("把根类钉成最终值（含 Tailwind 的 dark 类），且先清掉旧类", () => {
    expect(script).toContain("document.documentElement");
    expect(script).toContain('classList.remove("theme-light", "theme-dark", "dark")');
    expect(script).toContain('classList.add("dark")');
  });

  it("不写内联样式、不碰字号/间距，且异常不拦渲染", () => {
    expect(script).not.toMatch(/style\.|fontSize|padding|margin/u);
    expect(script).toContain("catch");
  });
});
