import { readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

// blockAfter 与 collectFiles 在多个 design 测试里逐字节重复，已收进 helpers。
import { blockAfter, collectFiles, extensionFilter } from "../helpers/designFiles";

/**
 * 液态玻璃层的作用域 / 材料 / 降级设计测试（ADR-0012 决策 5 与约束 6–7）。
 *
 * 玻璃是最容易"顺手多用一点"的东西：一张卡片加 `backdrop-filter` 只要一行，
 * 但会同时付出两层代价 —— 可读性（正文压在模糊层上）与性能（每层一次 GPU pass，
 * 且 `backdrop-filter` 让元素成为 backdrop root，嵌套玻璃读成平灰）。
 * 所以这里把"能出现在哪 / 能叠几层 / 不支持的浏览器怎么办"锁成断言。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");
const tailwindCss = read("app/assets/css/tailwind.css");

// 只扫 .vue：玻璃层断言面向组件，不是样式表。
const appFiles = collectFiles(join(projectDir, "app"), extensionFilter("vue")).map((file) => ({
  file: relative(projectDir, file).split(sep).join("/"),
  text: readFileSync(file, "utf8"),
}));

const GLASS_CLASS_RE = /\bglass-(thin|thick)\b/gu;

/**
 * 允许出现玻璃层的组件清单 + 每个组件最多几层。
 *
 * 为什么是"每组件一层"而不是"全站 ≤3"：一层是**不能嵌套**的结构保证 ——
 * 单个组件里只有一个玻璃元素，就不可能出现"玻璃套玻璃"（那会读成平灰面）。
 * 同屏上限由下面「同屏预算」单独断言。
 *
 * 轮播控件已从"浮在图上的一颗玻璃药丸"改为底部渐黑区里的一行内容，因此轮播组件
 * 不再出现在清单里（它的新立场由 banner-veil 专项测试锁住）。
 */
const GLASS_LAYERS: Record<string, number> = {
  "app/components/layout/AppHeader.vue": 1,
  "app/components/jelly/JellyLauncher.vue": 1,
  "app/components/jelly/JellyAiPanel.vue": 1,
  "app/components/auth/AuthModal.vue": 1,
  "app/components/favorite/FavoriteFolderModal.vue": 1,
  "app/components/article/ArticleScoreModal.vue": 1,
  "app/components/studio/StudioSelectionAiToolbar.vue": 1,
  "app/components/studio/StudioWriteHeader.vue": 1,
};

/** 与页头同时挂在阅读站默认布局上的常驻玻璃层（1 页头 + 1 捏捏把手 + 1 捏捏面板）。 */
const SAME_SCREEN_BUDGET = [
  "app/components/layout/AppHeader.vue",
  "app/components/jelly/JellyLauncher.vue",
  "app/components/jelly/JellyAiPanel.vue",
];

/** 内容面（正文 / 卡片 / 列表）：一律不透明，绝不出现玻璃类。 */
const CONTENT_SURFACES = [
  "app/components/common/MarkdownRenderSurface.vue",
  "app/components/article/ArticleFeedItem.vue",
  "app/components/comment/CommentThread.vue",
  "app/pages/index.vue",
  "app/pages/search.vue",
  "app/pages/users/[id].vue",
];

function glassCount(text: string): number {
  // 只数"作为组件类名"的使用：注释里的说明不算，`var(--glass-thin-fill)` 这类
  // token 引用也不算（NCard 的 --n-color 会被绑到同一份填充 token 上）。
  const code = text
    .replace(/\/\*[\s\S]*?\*\//gu, "")
    .replace(/<!--[\s\S]*?-->/gu, "")
    .replace(/^\s*\/\/.*$/gmu, "")
    .replace(/var\(--glass-[^)]*\)/gu, "");
  return [...code.matchAll(GLASS_CLASS_RE)].length;
}

describe("玻璃作用域：只出现在导航 / 浮层 / 悬浮控件 / 胶囊", () => {
  it("除允许清单外，任何组件都不得出现 glass-thin / glass-thick", () => {
    const offenders = appFiles
      .filter(({ file, text }) => !(file in GLASS_LAYERS) && glassCount(text) > 0)
      .map(({ file }) => file);

    expect(offenders).toEqual([]);
  });

  it("允许清单里的组件每处最多一层玻璃（单组件内不可能出现嵌套玻璃）", () => {
    const violations = Object.entries(GLASS_LAYERS).flatMap(([file, expected]) => {
      const entry = appFiles.find((item) => item.file === file);
      expect(entry, `允许清单里的 ${file} 不存在（清单过期）`).toBeDefined();
      const actual = glassCount(entry!.text);
      return actual === expected ? [] : [`${file}: 期望 ${expected} 层，实际 ${actual} 层`];
    });

    expect(violations).toEqual([]);
  });

  it("正文 / 卡片 / 列表组件一个玻璃类都没有（内容面必须不透明）", () => {
    const offenders = CONTENT_SURFACES.filter((file) => {
      const entry = appFiles.find((item) => item.file === file);
      expect(entry, `内容面清单里的 ${file} 不存在（清单过期）`).toBeDefined();
      return glassCount(entry!.text) > 0;
    });

    expect(offenders).toEqual([]);
  });

  it("同屏常驻玻璃层 ≤3（页头 + 捏捏把手 + 捏捏面板）", () => {
    const total = SAME_SCREEN_BUDGET.reduce(
      (sum, file) => sum + glassCount(appFiles.find((item) => item.file === file)!.text),
      0,
    );

    expect(total).toBeLessThanOrEqual(3);
  });

  it("内容面的组件类（卡片 / 区块 / 列表条目）没有 backdrop-filter", () => {
    for (const selector of [".surface-card", ".surface-section", ".feed-entry"]) {
      const block = blockAfter(tailwindCss, selector);

      expect(block, selector).not.toContain("backdrop-filter");
      expect(block, selector).not.toContain("glass-thin");
      expect(block, selector).not.toContain("glass-thick");
    }
  });

  it("旧的 .glass-badge 是实心徽标（名字带 glass，但不是玻璃层）", () => {
    const block = blockAfter(tailwindCss, ".glass-badge");

    expect(block).not.toContain("backdrop-filter");
  });
});

describe("玻璃材料：两档 token 消费（#18 种下的玻璃组）", () => {
  const baseRule = blockAfter(tailwindCss, ".glass-thin");

  it("薄玻璃消费 thin 填充 / 模糊 / 饱和度 token（不写死数字）", () => {
    expect(baseRule).toContain("background: var(--glass-thin-fill)");
    expect(baseRule).toContain("backdrop-filter: saturate(var(--glass-thin-saturate)) blur(var(--glass-thin-blur))");
    expect(baseRule).toContain("-webkit-backdrop-filter:");
    expect(baseRule).not.toMatch(/\bborder(-width)?:\s*[^;0]/u);
  });

  it("厚玻璃消费 thick 填充 / 模糊 / 饱和度 token", () => {
    expect(tailwindCss).toContain("background: var(--glass-thick-fill)");
    expect(tailwindCss).toContain(
      "saturate(var(--glass-thick-saturate)) blur(var(--glass-thick-blur))",
    );
    expect(tailwindCss).toContain("var(--glass-thin-saturate)) blur(var(--glass-thin-blur))");
  });

  it("1px 折射边缘带用 mask-composite: exclude + padding:1px，而不是 border", () => {
    const rimRule = blockAfter(tailwindCss, ".glass-thin::before");

    expect(rimRule).toContain("padding: 1px");
    expect(rimRule).toContain("mask-composite: exclude");
    expect(rimRule).toContain("-webkit-mask-composite: xor");
    expect(rimRule).not.toMatch(/\bborder(-width)?:\s*[^;0]/u);
    expect(rimRule).toContain("var(--glass-rim)");
  });

  it("噪点层是内联 feTurbulence 的 data-URI，透明度走 token", () => {
    const grainRule = blockAfter(tailwindCss, ".glass-thin::after");

    expect(grainRule).toContain("feTurbulence");
    expect(grainRule).toContain("data:image/svg+xml");
    expect(grainRule).toContain("opacity: var(--glass-grain-opacity)");
    expect(grainRule).toContain("pointer-events: none");
  });

  it("指针镜面由 --mx/--my 驱动，且只作用于伪元素（不改布局）", () => {
    const rimRule = blockAfter(tailwindCss, ".glass-thin::before");

    expect(rimRule).toContain("var(--mx");
    expect(rimRule).toContain("var(--my");
    expect(baseRule).toContain("position: relative");
    expect(baseRule).toContain("isolation: isolate");
  });
});

describe("四类降级各自生效", () => {
  it("① 不支持 backdrop-filter → 退化为不透明面", () => {
    const block = blockAfter(tailwindCss, "@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)))");

    expect(block).toContain("background: var(--color-surface)");
    expect(block).toContain("backdrop-filter: none");
    expect(block).toContain("-webkit-backdrop-filter: none");
  });

  it("② 系统「减少透明度」→ 不透明面 + 关模糊 + 关指针高光", () => {
    const block = blockAfter(tailwindCss, "@media (prefers-reduced-transparency: reduce)");

    expect(block).toContain("background: var(--color-surface)");
    expect(block).toContain("backdrop-filter: none");
    expect(block).toContain("--glass-specular: none");
  });

  it("③ 系统「减少动态」→ 关指针高光（进场归零由 .motion-reveal 承担）", () => {
    const block = blockAfter(tailwindCss, "@media (prefers-reduced-motion: reduce)", true);

    expect(block).toContain("--glass-specular: none");
    expect(block).toContain(".motion-reveal");
  });

  it("④ 移动端 → blur 降到 --glass-blur-mobile 并关指针高光", () => {
    const block = blockAfter(tailwindCss, "@media (max-width: 768px)", true);

    expect(block).toContain("var(--glass-blur-mobile)");
    expect(block).toContain("--glass-specular: none");
  });
});
