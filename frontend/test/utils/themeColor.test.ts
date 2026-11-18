import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it, vi } from "vitest";

import { THEME_COOKIE_NAME, buildThemeInitScript } from "~/utils/themePreference";
import {
  THEME_COLOR_META_NAME,
  buildThemeColorMeta,
  parseThemeColorPalette,
  themeColorFor,
} from "~/utils/themeColor";

/**
 * 移动端状态栏颜色（#434，spec #423 第 3 批第 53 条）。
 *
 * 硬约束：`theme-color` 的取值必须和 `--color-bg-page` 是**同一份**（不是再抄一遍
 * 色值），并随站点主题（不是只看系统偏好）切换。
 *
 * 首屏对齐的难点：SSR 只读得到 cookie，读不到系统偏好；没有 cookie 的新访客
 * 若要跟随系统暗色，服务端会先输出浅色 meta。所以水合前的首屏脚本必须把 meta
 * 一并定成最终主题 —— 这里直接执行脚本，断言 DOM 里最终留下的 meta 内容。
 */

const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const appSource = readFileSync(join(projectDir, "app/app.vue"), "utf8");
const palette = parseThemeColorPalette(tokensCss);

describe("theme-color 取值与 --color-bg-page 同源", () => {
  it("亮档就是 tokens.css 的页面底 #F5F3EE，暗档就是 #161617", () => {
    // 期望值来自 tokens.css 的语义层（独立来源），不是把解析结果再算一遍。
    expect(palette).toEqual({ light: "#f5f3ee", dark: "#161617" });
  });

  it("meta 内容按当前主题取自同一份配色", () => {
    expect(themeColorFor("light", palette)).toBe(palette.light);
    expect(themeColorFor("dark", palette)).toBe(palette.dark);
  });

  it("theme-color 排在首屏同步脚本之前（脚本才能在绘制前把它改成最终值）", () => {
    // unhead 的同步内联脚本权重 50、普通 meta 权重 100；不给负数优先级的话
    // meta 会落在脚本后面，脚本执行时找不到它。
    expect(buildThemeColorMeta("dark", palette)).toEqual({
      name: THEME_COLOR_META_NAME,
      content: palette.dark,
      tagPriority: -1,
    });
  });
});

/** 在 happy-dom 里跑一次首屏脚本，返回执行后 DOM 的最终状态。 */
function runInitScript(input: {
  cookie?: "light" | "dark";
  systemDark?: boolean;
  existingMeta?: string;
}): { content: string | null; metaCount: number; rootClass: string } {
  document.head.innerHTML =
    input.existingMeta === undefined
      ? ""
      : `<meta name="${THEME_COLOR_META_NAME}" content="${input.existingMeta}">`;
  document.documentElement.className = "";
  // 每个用例都显式写一次 cookie：没给值时必须清掉，否则会继承上一个用例。
  document.cookie = input.cookie
    ? `${THEME_COOKIE_NAME}=${input.cookie}; path=/`
    : `${THEME_COOKIE_NAME}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: Boolean(input.systemDark),
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));

  // 首屏脚本是一段字符串（SSR 直接内联），这里执行它而不是只对源码做正则。
  new Function(buildThemeInitScript(palette))();

  return {
    content: document
      .querySelector(`meta[name="${THEME_COLOR_META_NAME}"]`)
      ?.getAttribute("content") ?? null,
    metaCount: document.querySelectorAll(`meta[name="${THEME_COLOR_META_NAME}"]`).length,
    rootClass: document.documentElement.className,
  };
}

describe("首屏对齐：绘制前 theme-color 跟随最终主题（cookie > 系统偏好）", () => {
  it("用户选了暗色：即使系统是亮色，状态栏也是暗档", () => {
    const state = runInitScript({ cookie: "dark", systemDark: false });

    expect(state.content).toBe(palette.dark);
    expect(state.rootClass).toContain("theme-dark");
  });

  it("用户选了亮色：即使系统是暗色，状态栏也是亮档", () => {
    const state = runInitScript({ cookie: "light", systemDark: true });

    expect(state.content).toBe(palette.light);
  });

  it("没有 cookie 的新访客跟随系统暗色（这是 SSR 输出浅色后必须补上的那一步）", () => {
    const state = runInitScript({ systemDark: true });

    expect(state.content).toBe(palette.dark);
  });

  it("没有 cookie 且系统是亮色：亮档", () => {
    expect(runInitScript({ systemDark: false }).content).toBe(palette.light);
  });

  it("原地更新已有 meta，不制造第二个 theme-color", () => {
    const state = runInitScript({ cookie: "dark", existingMeta: "#ffffff" });

    expect(state.content).toBe(palette.dark);
    expect(state.metaCount).toBe(1);
  });

  it("head 里没有 theme-color 时补一个（无 JS 兜底之外的最后一道）", () => {
    const state = runInitScript({ cookie: "light" });

    expect(state.content).toBe(palette.light);
    expect(state.metaCount).toBe(1);
  });
});

describe("app.vue 接线：同一份配色既给 head 也给首屏脚本", () => {
  it("配色从 tokens.css 解析，不给第二个色值来源", () => {
    expect(appSource).toContain("parseThemeColorPalette(tokensCss)");
    expect(appSource).not.toMatch(/#(?:f5f3ee|161617)/iu);
  });

  it("head 的 theme-color 由当前主题驱动（切主题即时生效）", () => {
    expect(appSource).toContain("buildThemeColorMeta(uiStore.theme, themeColorPalette)");
  });

  it("首屏脚本用同一份配色，水合前就对齐（不闪白 / 不水合打架）", () => {
    expect(appSource).toContain("buildThemeInitScript(themeColorPalette)");
  });
});
