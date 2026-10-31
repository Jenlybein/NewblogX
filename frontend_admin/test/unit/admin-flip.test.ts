import { readFileSync, readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 后台翻转的结构断言（#22）：
 *   - 两端 tokens.css 逐字节一致（后台特有变体已合并进主表）；
 *   - admin.css 全量走语义层、圆角从阶梯取、无克莱因蓝、无后台玻璃；
 *   - 每页都有可见主题开关（默认布局顶栏 + 登录/初始化布局）；
 *   - 暗色正式启用（app.vue 接 token 的第二份取值）。
 *
 * 这些断言读的是**真实文件**，不做 mock：它们要拦的是"改了一半"这类事故。
 */
const projectDir = process.cwd();
const appDir = join(projectDir, "app");
const repoRoot = join(projectDir, "..");

const tokensCss = readFileSync(join(appDir, "assets/css/tokens.css"), "utf8");
const readerTokensCss = readFileSync(join(repoRoot, "frontend/app/assets/css/tokens.css"), "utf8");
const adminCss = readFileSync(join(appDir, "assets/css/admin.css"), "utf8");
const appVue = readFileSync(join(appDir, "app.vue"), "utf8");
const shellVue = readFileSync(join(appDir, "components/admin/AdminShell.vue"), "utf8");
const authLayout = readFileSync(join(appDir, "layouts/auth.vue"), "utf8");
const defaultLayout = readFileSync(join(appDir, "layouts/default.vue"), "utf8");

/** 去掉注释：注释里可以提到旧 token 名/旧写法（可读性需要），不算"还在用"。 */
function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//gu, "").replace(/\/\/[^\n]*/gu, "");
}

const adminCssCode = stripComments(adminCss);

/** 页面文件清单（相对 app/ 的路径）。 */
function pageFiles(): string[] {
  const pages: string[] = [];
  const walk = (dir: string) => {
    for (const name of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, name.name);
      if (name.isDirectory()) {
        walk(full);
        continue;
      }
      if (name.name.endsWith(".vue")) pages.push(relative(appDir, full).split(sep).join("/"));
    }
  };
  walk(join(appDir, "pages"));
  return pages.sort();
}

describe("两端 tokens.css 逐字节一致（后台特有变体合并进主表）", () => {
  it("内容完全相同（同源义务，不是两份可漂移的配置）", () => {
    expect(tokensCss).toBe(readerTokensCss);
  });

  it("后台特有的语义变体也在同一张表里（合并而不是各留一份）", () => {
    for (const token of [
      "--color-surface-glass",
      "--color-on-strong-line",
      "--color-on-strong-soft",
      "--font-mono",
    ]) {
      expect(tokensCss).toContain(`${token}:`);
    }
  });

  it("克莱因蓝与旧蓝档全部消失（后台旧值随读者站取中性石墨阶）", () => {
    expect(stripComments(tokensCss)).not.toMatch(/--jelly-blue/iu);
    expect(tokensCss).not.toMatch(/#002fa7/iu);
    expect(tokensCss).not.toMatch(/rgb\(0\s*47\s*167/iu);
  });

  it("后台额定语义 token 与读者站同名同值（明暗两份都在）", () => {
    for (const token of [
      "--color-ink",
      "--color-bg-page",
      "--color-surface",
      "--color-focus",
      "--color-danger-ink",
    ]) {
      const pattern = new RegExp(`${token}\\s*:\\s*([^;]+);`, "gu");

      // 明暗各一份：同一组名字出现两处（值可以相同表达式、由基底翻转决定实际色）。
      expect([...tokensCss.matchAll(pattern)].length, token).toBeGreaterThanOrEqual(2);
    }

    // 取值本身是中性石墨阶（不是旧的蓝档）。
    expect(tokensCss).toContain("--color-ink: #1c1b19;");
    expect(tokensCss).toContain("--color-ink: #f5f5f7;");
    expect(tokensCss).toContain("--color-bg-page: #161617;");
  });
});

describe("admin.css 全量走语义层（后台颜色只有一个来源）", () => {
  it("不再消费任何原始层色相（--jelly-* 全部退出后台样式）", () => {
    expect(adminCssCode).not.toMatch(/var\(--jelly-/u);
  });

  it("圆角从五档阶梯取：没有 `border-radius: 0` 反例，也不许字面量", () => {
    const radiusValues = [...adminCssCode.matchAll(/border-radius:\s*([^;]+);/gu)].map((match) =>
      match[1]!.replace(/\s+/gu, " ").trim(),
    );

    expect(radiusValues.length).toBeGreaterThan(8);
    for (const value of radiusValues) {
      expect(value).toMatch(/^var\(--radius-(?:sm|md|lg|xl|pill)\)$/u);
    }
    expect(adminCssCode).not.toMatch(/border-radius:\s*0/u);
  });

  it("圆角映射到正确的档位：卡片/面板 20、控件 8、胶囊 pill", () => {
    expect(adminCssCode).toMatch(/\.admin-card\s*\{[^}]*border-radius: var\(--radius-lg\)/u);
    expect(adminCssCode).toMatch(/\.admin-stat-card\s*\{[^}]*border-radius: var\(--radius-lg\)/u);
    expect(adminCssCode).toMatch(/\.admin-user-pill\s*\{[^}]*border-radius: var\(--radius-pill\)/u);
    expect(adminCssCode).toMatch(/\.admin-chart\s*\{[^}]*border-radius: var\(--radius-lg\)/u);
    expect(adminCssCode).toMatch(/\.admin-login-hero\s*\{[^}]*border-radius: var\(--radius-lg\)/u);
  });

  it("不做后台玻璃：侧栏 / 顶栏 / 登录卡都不再上 backdrop-filter", () => {
    expect(adminCssCode).not.toMatch(/backdrop-filter/u);
    expect(adminCssCode).not.toMatch(/var\(--glass-/u);
  });

  it("登录页 hero 与封面兜底改成中性石墨渐变（不再是克莱因蓝铺色）", () => {
    expect(adminCssCode).toMatch(/\.admin-login-hero\s*\{[^}]*background: linear-gradient\(135deg, var\(--color-ink\)/u);
    expect(adminCssCode).toMatch(/color: var\(--color-on-strong\)/u);
    expect(adminCssCode).toMatch(/\.admin-article-cover__fallback\s*\{[^}]*var\(--color-ink\)/u);
    // 旧的「蓝 → 暖灰」渐变必须整段消失。
    expect(adminCssCode).not.toMatch(/var\(--jelly-warm\)/u);
  });

  it("图表柱与折线走中性墨色（不是功能蓝，也不是克莱因蓝）", () => {
    const bar = /\.admin-chart__bar\s*\{([^}]*)\}/u.exec(adminCssCode)?.[1] ?? "";
    const line = /\.admin-token-line__line\s*\{([^}]*)\}/u.exec(adminCssCode)?.[1] ?? "";

    expect(bar).toContain("var(--color-ink");
    expect(bar).not.toContain("--color-focus");
    expect(line).toContain("stroke: var(--color-ink)");
    expect(adminCssCode).not.toMatch(/stroke: var\(--color-focus\)/u);
  });

  it("功能蓝只保留在焦点环与链接上（不再当按钮/悬停字的颜色）", () => {
    const focusUses = [...adminCssCode.matchAll(/var\(--color-focus\)/gu)].length;

    // 只剩 `:focus-visible` 的 outline 这一处（链接文字在 .vue 模板里走 text-focus 类）。
    expect(focusUses).toBe(1);
    expect(adminCssCode).toMatch(/:focus-visible\s*\{[^}]*outline: 2px solid var\(--color-focus\)/u);
  });

  it("暗色下仍只剩语义层：没有任何硬编码 alpha（rgba 常量）", () => {
    expect(adminCssCode).not.toMatch(/\brgba\(/u);
  });
});

describe("每页都有可见且可用的主题开关", () => {
  it("默认布局的顶栏放开关（AdminShell 渲染默认布局的全部页面）", () => {
    expect(defaultLayout).toContain("AdminShell");
    expect(shellVue).toContain("AdminThemeToggle");
    expect(shellVue).toMatch(/<AdminThemeToggle[^>]*\/>/u);
  });

  it("登录 / 初始化布局也放开关（auth.vue 是这两页的落点）", () => {
    expect(authLayout).toContain("AdminThemeToggle");
    expect(authLayout).toMatch(/<AdminThemeToggle/u);
  });

  it("开关可切换：调用 store 的 toggleTheme，并有说清结果的 aria-label", () => {
    const toggle = readFileSync(join(appDir, "components/admin/ThemeToggle.vue"), "utf8");

    expect(toggle).toMatch(/useThemeStore/u);
    expect(toggle).toMatch(/toggleTheme/u);
    expect(toggle).toMatch(/:aria-label=/u);
    expect(toggle).toMatch(/IconSun|IconMoon/u);
  });

  it("12 个路由页面全部被两种布局覆盖（不存在没有开关的页面）", () => {
    const pages = pageFiles();

    // 页面数量本身就是既有验收项（菜单 ↔ 页面文件对账另有门禁用例）。
    // 12 = 11 个既有页面 + ADR-0017 决策 5 新增的「热门置顶」页。
    expect(pages.length).toBe(12);
    for (const page of pages) {
      const source = readFileSync(join(appDir, page), "utf8");
      const usesAuthLayout = /definePageMeta\(\{ layout: 'auth' \}\)/u.test(source);

      // 要么走 auth 布局（login/setup，它自带开关），要么走默认布局（AdminShell 顶栏）。
      // 第三选项"两处都不走"会让页面没有开关 —— 那正是本票要排除的事故。
      expect(!usesAuthLayout || page.endsWith("login.vue") || page.endsWith("setup.vue"), page).toBe(
        true,
      );
    }
  });
});

describe("后台暗色正式启用（此前只有映射、没有开关）", () => {
  it("app.vue 按当前主题切换 Naive 的明暗基础主题与 token 覆盖", () => {
    expect(appVue).toMatch(/darkTheme/u);
    expect(appVue).toMatch(/parseThemePalette\(tokensCss,\s*themeStore\.theme\)/u);
    expect(appVue).toMatch(/:theme=/u);
  });

  it("根类与水合前脚本都接上（跟随既有 .theme-dark / .dark 机制）", () => {
    expect(appVue).toMatch(/themeBodyClass\(themeStore\.theme\)/u);
    expect(appVue).toMatch(/buildThemeInitScript\(\)/u);
    expect(appVue).toMatch(/htmlAttrs/u);
    expect(appVue).toMatch(/bodyAttrs/u);
  });

  it("store 用与读者站同一个 cookie（切过一次之后两端选择一致）", () => {
    const store = readFileSync(join(appDir, "stores/theme.ts"), "utf8");

    expect(store).toContain("THEME_COOKIE_NAME");
    expect(store).toMatch(/toggleTheme/u);
  });
});
