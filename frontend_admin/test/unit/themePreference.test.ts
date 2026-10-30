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
  themeBodyClass,
  themeHtmlClass,
  themeRootClass,
} from "../../app/utils/themePreference";

/**
 * 后台明暗偏好的纯函数层（#22）：后台此前没有任何明暗机制，现在与读者站同一套
 * 判定规则（用户 cookie > 系统偏好 > 兜底浅色），并由首屏脚本在水合前钉住根类。
 */
describe("明暗偏好解析：用户选择 > 系统偏好 > 兜底", () => {
  it("cookie 说了算：用户显式切过就不再跟随系统", () => {
    expect(resolveThemePreference({ cookie: "dark", systemPrefersDark: false })).toEqual({
      theme: "dark",
      source: "user",
    });
    expect(resolveThemePreference({ cookie: "light", systemPrefersDark: true })).toEqual({
      theme: "light",
      source: "user",
    });
  });

  it("没有用户选择时跟随系统，读不到系统时才兜底浅色", () => {
    expect(resolveThemePreference({ systemPrefersDark: true })).toEqual({
      theme: "dark",
      source: "system",
    });
    expect(resolveThemePreference({ cookie: null, systemPrefersDark: false })).toEqual({
      theme: "light",
      source: "system",
    });
    expect(resolveThemePreference({})).toEqual({ theme: DEFAULT_THEME_MODE, source: "default" });
  });

  it("只认 light / dark：被篡改或空值一律当「没选过」", () => {
    expect(parseThemeCookie("DARK")).toBeNull();
    expect(parseThemeCookie("auto")).toBeNull();
    expect(parseThemeCookie(undefined)).toBeNull();
    expect(isThemeMode("light")).toBe(true);
    expect(isThemeMode(1)).toBe(false);
  });

  it("resolveThemeMode 只是 resolveThemePreference 的取值投影", () => {
    expect(resolveThemeMode({ cookie: "dark" })).toBe("dark");
    expect(resolveThemeMode({})).toBe(DEFAULT_THEME_MODE);
  });
});

describe("站点根类：明暗映射挂在 .theme-* 上（与 tokens.css 的暗色块同一组名字）", () => {
  it("root / body / html 类都带 .theme-dark，且暗色同时带 Tailwind 的 .dark", () => {
    expect(themeRootClass("dark")).toBe("theme-dark");
    expect(themeRootClass("light")).toBe("theme-light");
    expect(themeBodyClass("dark")).toBe("theme-dark dark");
    expect(themeBodyClass("light")).toBe("theme-light");
    // 首屏脚本与 useHead 写的是同一个函数结果（html 与 body 不会不同步）。
    expect(themeHtmlClass("dark")).toBe(themeBodyClass("dark"));
  });
});

describe("首屏脚本：水合前把根类钉成最终值（不闪白）", () => {
  const script = buildThemeInitScript();

  it("读同一个 cookie 名与系统偏好查询", () => {
    expect(THEME_COOKIE_NAME).toBe("blogx-theme");
    expect(script).toContain(THEME_COOKIE_NAME);
    expect(script).toContain(THEME_MEDIA_QUERY);
  });

  it("增删同一组类（theme-dark / theme-light / dark）", () => {
    expect(script).toContain('classList.remove("theme-light", "theme-dark", "dark")');
    expect(script).toContain('"theme-dark"');
    expect(script).toContain('"theme-light"');
    expect(script).toContain('classList.add("dark")');
  });

  it("脚本失败也不拦渲染（try/catch 兜底）", () => {
    expect(script).toContain("try {");
    expect(script).toContain("catch (error)");
  });
});
