import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, blockAfterContaining, stripComments } from "../helpers/designFiles";

/**
 * 材料分级：阴影层级 / 玻璃分档 / 页头材质到场 / 明暗同帧（票 428，spec 423 第 2、4 批）。
 *
 * 这组测试钉的是**关系**而不是像素：内容卡的影必须明显小于浮层；两档玻璃的
 * 影 / 边缘带 / specular 必须能互相区分；页头转玻璃必须有一段 200ms 的到场
 * 而不是一帧跳变；明暗切换时纸面与结构线必须同帧。所有取值只允许来自 token，
 * 所以断言的是 token 名与关系（谁比谁大），不是具体像素。
 *
 * 为什么不断言像素：仓库的视觉回归口径是"设计门禁 + 目视验收"，截图比对明确
 * 在 spec 的 Out of Scope 里；像素断言会把每一次合理的取值调整都变成红。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");
const tokensCss = read("app/assets/css/tokens.css");
const tailwindCss = read("app/assets/css/tailwind.css");

/** 取 `:root` 块（第一份；减少动态块在它之后，不会被选中）。 */
function rootBlock(css: string): string {
  const match = /:root\s*\{([\s\S]*?)\n\}/u.exec(css);
  if (!match) throw new Error("tokens.css 里找不到 :root 块");
  return match[1]!;
}

/** 取暗色映射块（`.dark` / `.theme-dark`）。 */
function darkBlock(css: string): string {
  const match = /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\}/u.exec(css);
  if (!match) throw new Error("tokens.css 里找不到暗色映射块");
  return match[1]!;
}

/** 某个块里某条自定义属性的取值（去注释、去首尾空白）。 */
function declaration(block: string, name: string): string | undefined {
  const match = new RegExp(`${name}\\s*:\\s*([^;\\n]+?)\\s*(?:;|\\/\\*)`, "u").exec(block);
  return match?.[1]?.trim();
}

/** 解析单条 box-shadow 的偏移/模糊/扩散/颜色（`0 Xpx Ypx [Zpx] 颜色`）。 */
function parseShadow(value: string | undefined) {
  if (!value) throw new Error("阴影 token 缺失");
  const match = /^0\s+(-?[0-9.]+)px\s+(-?[0-9.]+)px(?:\s+(-?[0-9.]+)px)?\s+(.+)$/u.exec(value);
  if (!match) throw new Error(`阴影不是 0 / X / Y / [Z] / 颜色 形态：${value}`);
  return {
    offsetY: Number(match[1]),
    blur: Number(match[2]),
    spread: match[3] === undefined ? 0 : Number(match[3]),
    color: match[4]!.trim(),
  };
}

/** 从 `rgb(R G B / A)` 里取 alpha；取不到返回 NaN（用于比较两档高光亮暗）。 */
function alphaOf(value: string | undefined): number {
  if (!value) throw new Error("颜色 token 缺失");
  const match = /\/\s*([0-9.]+)\s*\)/u.exec(value);
  return match ? Number(match[1]) : Number.NaN;
}

/** 找出正文包含某段声明的第一个声明块（用于取某一档玻璃自己的规则）。 */
function blockContaining(source: string, needle: string): string {
  for (const match of source.matchAll(/([^{}]+)\{([^{}]*)\}/gu)) {
    if (match[2]!.includes(needle)) return match[2]!;
  }
  throw new Error(`找不到包含 ${needle} 的声明块`);
}

/** 取某个 px 类 token 的数值（`240px` → 240）。 */
function pxValue(block: string, name: string): number {
  const raw = declaration(block, name);
  if (!raw) throw new Error(`${name} 缺失`);
  return Number.parseFloat(raw);
}

/** 内容面：一律是"贴纸边"而不是"悬空"，不得吃浮层影。 */
const CONTENT_SURFACES: Array<[string, string]> = [
  ["frontend/tailwind.css .surface-card", ".surface-card {"],
  ["frontend/tailwind.css .surface-section", ".surface-section {"],
  ["frontend/tailwind.css .feed-entry", ".feed-entry {"],
];

describe("内容卡投影：发丝级（1–2px 偏移、小模糊），不再是一层雾", () => {
  it("tokens.css 里的 --shadow-card 是 1–2px 偏移的小模糊，仍由 --color-shadow 派生", () => {
    const card = parseShadow(declaration(rootBlock(tokensCss), "--shadow-card"));

    expect(card.offsetY, "内容卡偏移只能 1–2px").toBeGreaterThanOrEqual(1);
    expect(card.offsetY, "内容卡偏移只能 1–2px").toBeLessThanOrEqual(2);
    expect(card.blur, "内容卡模糊必须是发丝级").toBeLessThanOrEqual(4);
    expect(card.color).toBe("var(--color-shadow)");
  });

  it("内容卡消费 --shadow-card，不再写 24px 偏移 / 60px 模糊的大影", () => {
    const block = blockAfter(tailwindCss, ".surface-card {");

    expect(block).toContain("box-shadow: var(--shadow-card)");
    expect(block).not.toContain("24px 60px");
  });

  it("后台概览报头（studio/dashboard 的内容纸面）同样降到发丝影", () => {
    const dashboard = read("app/pages/studio/dashboard.vue");
    const block = blockAfter(dashboard, ".overview__masthead {");

    expect(block).toContain("box-shadow: var(--shadow-card)");
    expect(block).not.toContain("24px 60px");
  });

  it("暗色阴影基底不含「大面积黑晕」：仍是 55% 以内的黑，且影由它派生", () => {
    const darkShadow = declaration(darkBlock(tokensCss), "--color-shadow");

    expect(alphaOf(darkShadow)).toBeLessThanOrEqual(0.55);
    // 内容卡的影只有 --color-shadow 一个来源：token 文件里不许再出现第二份阴影色。
    expect(stripComments(tokensCss)).not.toMatch(/--shadow-card:\s*[^;]*rgba?\(/u);
  });

  it(".surface-section / .feed-entry 这类内容面没有投影（投影只贴边，不外溢）", () => {
    for (const [label, marker] of CONTENT_SURFACES.filter(([, marker]) => !marker.includes(".surface-card"))) {
      const block = blockAfter(tailwindCss, marker);

      expect(block, label).not.toContain("box-shadow");
    }
  });
});

describe("--shadow-lg 只给浮层（内容面不许用它抬升）", () => {
  const OVERLAY_NAME_RE = /panel|modal|dialog|menu|popover|drawer|dropdown|sheet/iu;

  /** 找出所有"声明块里含某个 token"的选择器。 */
  function blocksUsing(source: string, needle: string): string[] {
    const code = stripComments(source);
    const selectors: string[] = [];
    for (const match of code.matchAll(/([^{}]+)\{([^{}]*)\}/gu)) {
      const selector = match[1]!.trim();
      if (match[2]!.includes(needle)) selectors.push(selector);
    }
    return selectors;
  }

  it("--shadow-lg 仍在 token 层定义（浮层影的唯一来源）", () => {
    expect(declaration(rootBlock(tokensCss), "--shadow-lg")).toBe(
      "0 32px 80px -32px var(--color-shadow)",
    );
  });

  it("消费 --shadow-lg 的只有浮层（下拉面板 / 消息面板），没有内容面", () => {
    const consumers = [
      ...blocksUsing(tailwindCss, "var(--shadow-lg)").map((selector) => ["tailwind.css", selector]),
      ...blocksUsing(read("app/components/layout/AppHeader.vue"), "var(--shadow-lg)").map(
        (selector) => ["AppHeader.vue", selector],
      ),
    ];

    expect(consumers.length, "至少要让一处浮层真的用上 --shadow-lg").toBeGreaterThanOrEqual(2);
    for (const [file, selector] of consumers) {
      expect(OVERLAY_NAME_RE.test(String(selector)), `${file} 的 ${selector}`).toBe(true);
      expect(String(selector)).not.toMatch(/surface-card|surface-section|feed-entry/iu);
    }
  });

  it("内容面（卡片 / 报头）的块里看不到 --shadow-lg", () => {
    for (const [label, marker] of [
      ...CONTENT_SURFACES,
      ["frontend/dashboard .overview__masthead", ".overview__masthead {"],
    ] as Array<[string, string]>) {
      const block = blockAfter(label.includes("dashboard") ? read("app/pages/studio/dashboard.vue") : tailwindCss, marker);

      expect(block, label).not.toContain("var(--shadow-lg)");
    }
  });
});

describe("玻璃两档：阴影 / 边缘带 / 内侧高光必须互相区分", () => {
  it("两档阴影取 spec 给定值，thick 的偏移与模糊都更大（大面板读得出更厚）", () => {
    expect(declaration(rootBlock(tokensCss), "--shadow-glass-thin")).toBe(
      "0 8px 24px -16px var(--color-shadow)",
    );
    expect(declaration(rootBlock(tokensCss), "--shadow-glass-thick")).toBe(
      "0 24px 56px -24px var(--color-shadow)",
    );

    const thin = parseShadow(declaration(rootBlock(tokensCss), "--shadow-glass-thin"));
    const thick = parseShadow(declaration(rootBlock(tokensCss), "--shadow-glass-thick"));

    expect(thick.offsetY).toBeGreaterThan(thin.offsetY);
    expect(thick.blur).toBeGreaterThan(thin.blur);
    // 两档都由同一个 --color-shadow 派生：暗色下不会各自变成第二份黑。
    expect(thin.color).toBe("var(--color-shadow)");
    expect(thick.color).toBe("var(--color-shadow)");
  });

  it("薄 / 厚两档各自消费自己的阴影 token（不再共用一条 18px 44px）", () => {
    const base = blockContaining(tailwindCss, "background: var(--glass-thin-fill)");
    const thick = blockContaining(tailwindCss, "background: var(--glass-thick-fill)");

    expect(base).toContain("box-shadow: var(--shadow-glass-thin)");
    expect(base).toContain("--glass-rim-tone: var(--glass-thin-rim)");
    expect(thick).toContain("box-shadow: var(--shadow-glass-thick)");
    expect(thick).toContain("--glass-rim-tone: var(--glass-thick-rim)");
    expect(thick).toContain("--glass-edge-opacity: var(--glass-thick-edge-opacity)");
    expect(thick).toContain("--glass-specular-size: var(--glass-thick-specular-size)");
  });

  it("边缘带 / 镜面高光走两档变量：thick 更亮、specular 更大", () => {
    const light = rootBlock(tokensCss);

    expect(alphaOf(declaration(light, "--glass-thick-rim"))).toBeGreaterThan(
      alphaOf(declaration(light, "--glass-thin-rim")),
    );
    expect(Number(declaration(light, "--glass-thick-edge-opacity"))).toBeGreaterThan(
      Number(declaration(light, "--glass-thin-edge-opacity")),
    );
    expect(pxValue(light, "--glass-thick-specular-size")).toBeGreaterThan(
      pxValue(light, "--glass-thin-specular-size"),
    );
  });

  it("暗色下两档仍可区分（thick 的边缘带更亮），且不会引发大面积黑晕", () => {
    expect(alphaOf(declaration(darkBlock(tokensCss), "--glass-thick-rim"))).toBeGreaterThan(
      alphaOf(declaration(darkBlock(tokensCss), "--glass-thin-rim")),
    );
    // 暗色玻璃影与内容卡同源：都只从 --color-shadow 派生，没有第二份写死的黑。
    expect(stripComments(tokensCss)).not.toMatch(/--shadow-glass-(?:thin|thick):\s*[^;]*(?:rgba?|#)/u);
  });

  it("1px 边缘带与 specular 都读两档变量（不是只改阴影不改高光）", () => {
    const rim = blockAfter(tailwindCss, ".glass-thin::before");
    const base = blockContaining(tailwindCss, "background: var(--glass-thin-fill)");

    expect(rim).toContain("var(--glass-rim-tone)");
    expect(rim).toContain("var(--glass-specular-size)");
    expect(rim).toContain("opacity: var(--glass-edge-opacity)");
    expect(base).toContain("var(--glass-thin-specular-size)");
  });
});

describe("降级仍在：不支持 blur / 减少透明度时两档一起退成实面", () => {
  it("@supports not (backdrop-filter) 两档都退化为不透明面", () => {
    const block = blockAfter(
      tailwindCss,
      "@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)))",
    );

    expect(block).toContain(".glass-thin");
    expect(block).toContain(".glass-thick");
    expect(block).toContain("background: var(--color-surface)");
    expect(block).toContain("backdrop-filter: none");
    expect(block).toContain("-webkit-backdrop-filter: none");
  });

  it("prefers-reduced-transparency 两档都退化为不透明面并关掉指针高光", () => {
    const block = blockAfter(tailwindCss, "@media (prefers-reduced-transparency: reduce)");

    expect(block).toContain(".glass-thin");
    expect(block).toContain(".glass-thick");
    expect(block).toContain("background: var(--color-surface)");
    expect(block).toContain("--glass-specular: none");
  });
});

describe("页头材质到场：背景 / 边框 / 阴影一起过渡 200ms（不是一帧跳玻璃）", () => {
  const header = read("app/components/layout/AppHeader.vue");

  function zIndex(block: string): number {
    const match = /z-index:\s*(-?[0-9]+)/u.exec(block);
    return match ? Number(match[1]) : Number.NaN;
  }

  it("模板里有一层不透明面（veil）：玻璃类照旧挂页头，实面靠 veil 淡出", () => {
    const template = header.slice(0, header.indexOf("<style"));

    expect(template).toContain('class="site-header__veil"');
    expect(template).toContain('aria-hidden="true"');
    // 滚动仍由 is-scrolled → glass-thin 的类映射驱动（不是另起一套状态）。
    expect(template).toContain("'glass-thin': scrolled");
  });

  it("veil 承载静止态的纸面与发丝线，滚过阈值后 200ms 淡出", () => {
    const veil = blockAfter(header, ".site-header__veil {");
    const scrolledVeil = blockAfter(header, ".site-header.is-scrolled .site-header__veil {");

    expect(veil).toContain("position: absolute");
    expect(veil).toContain("inset: 0");
    expect(veil).toContain("background: var(--color-surface)");
    expect(veil).toContain("border-bottom: 1px solid var(--color-line-strong)");
    expect(veil).toContain("pointer-events: none");
    expect(veil).toMatch(/transition:\s*opacity var\(--motion-base\) var\(--motion-ease\)/u);
    // 时长取 token（--motion-base = 200ms），不是写死毫秒。
    expect(declaration(rootBlock(tokensCss), "--motion-base")).toBe("200ms");
    expect(scrolledVeil).toContain("opacity: 0");
  });

  it("阴影也走过渡：静止态是零影，滚过阈值换成薄玻璃那档 + 底部 1px 高光", () => {
    const base = blockAfter(header, ".site-header {");
    const scrolled = blockAfter(header, ".site-header.is-scrolled {");

    expect(base).toContain("box-shadow: 0 0 0 0 var(--color-shadow)");
    expect(base).toMatch(/box-shadow var\(--motion-base\) var\(--motion-ease\)/u);
    expect(scrolled).toContain("var(--shadow-glass-thin)");
    expect(scrolled).toContain("0 1px 0 var(--glass-rim)");
    // 静止态的不透明面 + 边框已整体搬到 veil，页头自己不再两套背景互相覆盖。
    expect(header).not.toContain(".site-header:not(.is-scrolled)");
  });

  it("veil 盖住玻璃的伪元素、页头内容盖住 veil（过渡期不闪 1px 边缘带）", () => {
    const veil = blockAfter(header, ".site-header__veil {");
    const inner = blockAfter(header, ".site-header__inner {");
    const skip = blockAfter(header, ".site-skip {");

    // 玻璃 ::before（rim）z-index 3、::after（噪点）z-index 4（tailwind.css 的玻璃层）。
    expect(zIndex(veil)).toBeGreaterThan(4);
    expect(inner).toContain("position: relative");
    expect(zIndex(inner)).toBeGreaterThan(zIndex(veil));
    expect(zIndex(skip)).toBeGreaterThan(zIndex(veil));
  });

  it("减少动态下 veil 也不过渡（归零而不是放慢）", () => {
    const reduced = blockAfterContaining(
      header,
      "@media (prefers-reduced-motion: reduce)",
      ".site-header__veil",
    );

    expect(reduced).toContain("transition: none");
  });
});

describe("明暗切换同帧：纸面与结构线一起换（不允许底在渐变、面在跳变）", () => {
  const appVue = read("app/app.vue");

  it("body 不再自己渐变：颜色 / 背景过渡已从 body 摘除（整页瞬时而非半明半暗）", () => {
    const bodyRule = /body\s*\{[\s\S]*?\}/u.exec(stripComments(tailwindCss))?.[0] ?? "";

    expect(bodyRule).toContain("background: var(--color-bg-page)");
    expect(bodyRule).not.toContain("transition");
  });

  it("切换那一帧由根上的 .theme-switching 关掉全站过渡，下一帧再放开", () => {
    expect(tailwindCss).toContain("html.theme-switching");
    expect(tailwindCss).toMatch(
      /html\.theme-switching\s*,\s*html\.theme-switching[\s\S]*?\{[\s\S]*?transition:\s*none !important;/u,
    );
  });

  it("app.vue 监听 uiStore.theme 并调用 beginThemeSwitch（唯一写入路径被覆盖）", () => {
    expect(appVue).toContain("beginThemeSwitch");
    expect(appVue).toMatch(/watch\(\s*\(\)\s*=>\s*uiStore\.theme/u);
  });

  it("主题切换的守卫只在客户端接触 DOM（SSR 不读 document）", () => {
    const util = read("app/utils/themeTransition.ts");

    expect(util).toContain("typeof document");
    expect(util).toContain("documentElement");
    expect(util).toContain("requestAnimationFrame");
  });
});
