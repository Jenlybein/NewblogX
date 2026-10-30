import { readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// 遍历逻辑与其余 design 测试重复，收进 helpers。原实现没有跳过目录这一步，但 app/
// 下并不存在 node_modules / .nuxt 之类的目录，显式跳过与否行为完全一致。
import { collectFiles, extensionFilter } from "../helpers/designFiles";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const appDir = join(projectDir, "app");
const tokensCss = readFileSync(join(appDir, "assets/css/tokens.css"), "utf8");
const tailwindCss = readFileSync(join(appDir, "assets/css/tailwind.css"), "utf8");

/** 圆角五档：组件里只能出现这五个 token 名（或 0 = 不圆角）。 */
const RADIUS_TOKENS = ["control", "field", "card", "overlay", "pill"] as const;
const ALLOWED_ROUNDED = new RegExp(
  `^rounded-(?:${RADIUS_TOKENS.join("|")}|none)$`,
  "u",
);
/** 方向后缀：`rounded-t-card` 这类按边圆角仍取自同一套阶梯。 */
const DIRECTIONAL_RE = new RegExp(
  `^rounded-(?:t|b|l|r|tl|tr|bl|br|s|e)-(?:${RADIUS_TOKENS.join("|")}|none)$`,
  "u",
);

/** `.vue` 里的 `<style>` 块 + 纯 `.css` 文件 + `.ts` 里的 CSS 字符串，统一成 CSS 文本。 */
function collectCssSources(): { file: string; css: string }[] {
  const sources: { file: string; css: string }[] = [];

  for (const absolute of collectFiles(appDir, extensionFilter("vue", "css", "ts"))) {
    const file = relative(projectDir, absolute).replace(/\\/gu, "/");
    const source = readFileSync(absolute, "utf8");

    if (file.endsWith(".css")) {
      sources.push({ file, css: source });
      continue;
    }
    if (file.endsWith(".vue")) {
      for (const match of source.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gu)) {
        sources.push({ file, css: match[1]! });
      }
      continue;
    }
    // .ts：只取看起来是 CSS 的模板字符串（代码块 / 注入式样式）。
    for (const match of source.matchAll(/`(?:\\[\s\S]|[^`\\])*`/gu)) {
      const body = match[0].slice(1, -1);
      if (/(?:^|[;{}\s])(?:--)?[a-z][a-z0-9-]*\s*:\s*[^;{}\n]+;/iu.test(body)) {
        sources.push({ file, css: body });
      }
    }
  }

  return sources;
}

const CSS_SOURCES = collectCssSources();

/** 正数字面量圆角：除了 0、`inherit`（继承父级阶梯，同轴曲率）与 `var(--radius-*)` 之外一律算逃逸。 */
function findLiteralRadii(css: string): string[] {
  const offenders: string[] = [];
  for (const match of css.matchAll(/border-radius\s*:\s*([^;}\n]+)/gu)) {
    const value = match[1]!.trim();
    if (
      value === "0" ||
      value === "inherit" ||
      /var\(--radius-(?:sm|md|lg|xl|pill)\)/u.test(value)
    ) {
      continue;
    }
    offenders.push(value);
  }
  return offenders;
}

/** Tailwind 圆角工具类里不在五档之列的那些（`rounded-[18px]`、`rounded-2xl`、`rounded-full` …）。 */
function findEscapeRadiusUtilities(css: string): string[] {
  const offenders: string[] = [];
  for (const match of css.matchAll(/rounded-(?:\[[^\]]*\]|[a-z0-9-]+)/gu)) {
    const utility = match[0];
    if (ALLOWED_ROUNDED.test(utility) || utility === "rounded-none") continue;
    if (DIRECTIONAL_RE.test(utility)) continue;
    // `rounded-…` 出现在自定义类名里（如 `.rounded-none`）时同样按上面的规则放行/拦截。
    offenders.push(utility);
  }
  return offenders;
}

function ruleBody(selector: string): string {
  const start = tailwindCss.indexOf(`${selector} {`);
  if (start < 0) throw new Error(`tailwind.css 里找不到 ${selector}`);
  return tailwindCss.slice(start, tailwindCss.indexOf("}", start));
}

describe("圆角五档：token 是唯一来源", () => {
  const tiers = new Map([
    ["--radius-sm", "8px"],
    ["--radius-md", "14px"],
    ["--radius-lg", "20px"],
    ["--radius-xl", "28px"],
    ["--radius-pill", "999px"],
  ]);

  it.each([...tiers])("%s = %s", (name, value) => {
    expect(tokensCss).toMatch(new RegExp(`${name}\\s*:\\s*${value}`, "u"));
  });

  it("Tailwind 只暴露五档类名，且全部指向 token 变量", () => {
    const config = readFileSync(join(projectDir, "tailwind.config.ts"), "utf8");

    expect(config).toContain('control: "var(--radius-sm)"');
    expect(config).toContain('field: "var(--radius-md)"');
    expect(config).toContain('card: "var(--radius-lg)"');
    expect(config).toContain('overlay: "var(--radius-xl)"');
    expect(config).toContain('pill: "var(--radius-pill)"');
  });
});

describe("全站圆角落位：反例字面量归零", () => {
  it("所有 border-radius 声明只取五档 token 或不圆角", () => {
    const problems = CSS_SOURCES.flatMap(({ file, css }) =>
      findLiteralRadii(css).map((value) => ({ file, value })),
    );

    expect(problems).toEqual([]);
  });

  it("所有 rounded-* 工具类都在五档之内（`rounded-[18px]` / `rounded-2xl` / `rounded-full` 归零）", () => {
    // 组件模板与样式都在 CSS_SOURCES 里（.vue 的 <template> 不在此列，单独扫 .vue 全文）。
    const vueSources = collectFiles(appDir, extensionFilter("vue"))
      .filter((file) => file.endsWith(".vue"))
      .map((file) => ({
        file: relative(projectDir, file).replace(/\\/gu, "/"),
        source: readFileSync(file, "utf8"),
      }));

    const problems = vueSources.flatMap(({ file, source }) =>
      findEscapeRadiusUtilities(source).map((utility) => ({ file, utility })),
    );

    expect(problems).toEqual([]);
  });

  it("五处圆角逃逸（设置页 / 历史页 / 收藏页 / 聊天面 / 收藏夹弹窗）已归 token", () => {
    for (const file of [
      "app/pages/studio/settings.vue",
      "app/pages/studio/history.vue",
      "app/pages/studio/favorites.vue",
      "app/components/inbox/InboxChatPanel.vue",
      "app/components/favorite/FavoriteFolderModal.vue",
    ]) {
      const source = readFileSync(join(projectDir, file), "utf8");

      expect(source, `${file} 仍有正数字面量圆角`).not.toMatch(/border-radius:\s*[1-9]/u);
      expect(source, `${file} 仍有越界 rounded 工具类`).not.toMatch(
        /rounded-(?:\[[^\]]*\]|full|sm|md|lg|xl|2xl|3xl)\b/u,
      );
    }
  });
});

describe("结构件按阶梯取值（卡片 20 / 浮层 28 / 胶囊 pill）", () => {
  it("卡片与分节用 rounded-card（20px），不再是圆角归零", () => {
    for (const selector of [".surface-card", ".surface-section", ".feed-entry", ".article-paper", ".profile-hero"]) {
      expect(ruleBody(selector)).toMatch(/rounded-card/u);
    }
  });

  it("胶囊/徽标用 rounded-pill，控件用 rounded-control / rounded-field", () => {
    expect(ruleBody(".glass-badge")).toMatch(/rounded-pill/u);
    expect(ruleBody(".soft-tab")).toMatch(/rounded-pill/u);
    expect(ruleBody(".blueprint-tab")).toMatch(/rounded-(?:control|field)/u);
    expect(ruleBody(".article-action")).toMatch(/rounded-(?:control|field)/u);
    expect(ruleBody(".article-toc__progress")).toMatch(/rounded-pill/u);
    expect(ruleBody(".article-reading-progress")).toMatch(/rounded-pill/u);
  });

  it("写作区控件、聊天气泡、收藏夹弹窗列表都按阶梯取值", () => {
    const writeCss = readFileSync(join(appDir, "assets/css/studio-write.css"), "utf8");

    expect(writeCss).toMatch(/var\(--radius-(?:sm|md|lg|xl|pill)\)/u);
    expect(ruleBody(".studio-chat-bubble__content")).toMatch(/rounded-(?:control|field)/u);
    expect(ruleBody(".studio-chat-composer")).toMatch(/rounded-(?:control|field|card)/u);
  });
});
