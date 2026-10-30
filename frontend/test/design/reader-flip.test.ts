import { readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// 遍历逻辑与其余 design 测试重复，收进 helpers。原实现没有跳过目录这一步，但 app/
// 下并不存在 node_modules / .nuxt 之类的目录，显式跳过与否行为完全一致。
import { collectFiles, extensionFilter } from "../helpers/designFiles";

/**
 * #20「读者站视觉语言翻转」的验收断言（结构测试）：
 *
 *   1. 克莱因蓝彻底退场（任何文件都不再出现 #002FA7 / --jelly-blue*）；
 *   2. 功能蓝只出现在**文字链接**与**焦点环**，不做铺底、不做激活态、不做进度条；
 *   3. 形状语义退役（页头黄三角 / 空态三色记号 / 40px 网格底纹）；
 *   4. 原先直接消费原始色相的组件改走语义层（页脚 / 封面 / 字标 / 主按钮 / 激活 tab）；
 *   5. 阅读进度与目录激活、写作页主题开关落到语义层。
 */
const projectDir = process.cwd();
const appDir = join(projectDir, "app");

const SOURCES = collectFiles(appDir, extensionFilter("vue", "css", "ts")).map((absolute) => ({
  file: relative(projectDir, absolute).replace(/\\/gu, "/"),
  text: readFileSync(absolute, "utf8"),
}));

function read(relativePath: string): string {
  return readFileSync(join(projectDir, relativePath), "utf8");
}

/** CSS 规则块（selector + declarations）。@media 外壳不会挡住内部规则。 */
function cssBlocks(source: string): { selector: string; declarations: string }[] {
  return [...source.matchAll(/([^{}]+)\{([^{}]*)\}/gu)]
    .map((match) => ({ selector: match[1]!.trim(), declarations: match[2]! }))
    .filter((block) => !block.selector.endsWith(","));
}

/** 把 `.vue` 的 `<style>`、纯 `.css`、`.ts` 里的 CSS 字符串统一成 CSS 文本。 */
function cssSourceOf(entry: { file: string; text: string }): string[] {
  if (entry.file.endsWith(".css")) return [entry.text];
  if (entry.file.endsWith(".vue")) {
    return [...entry.text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gu)].map((match) => match[1]!);
  }
  return [...entry.text.matchAll(/`(?:\\[\s\S]|[^`\\])*`/gu)]
    .map((match) => match[0].slice(1, -1))
    .filter((body) => /(?:^|[;{}\s])(?:--)?[a-z][a-z0-9-]*\s*:\s*[^;{}\n]+;/iu.test(body));
}

describe("克莱因蓝退役：读者站任何位置都不再出现", () => {
  it("组件 / 样式 / TS 字符串里没有 #002FA7 或 --jelly-blue*", () => {
    const problems = SOURCES.filter((entry) => !entry.file.endsWith("assets/css/tokens.css")).filter(
      (entry) => /#002fa7|--jelly-blue|rgb\(\s*0\s*47\s*167/iu.test(entry.text),
    );

    expect(problems.map((entry) => entry.file)).toEqual([]);
  });

  it("tokens.css 自己也不再有克莱因蓝", () => {
    expect(read("app/assets/css/tokens.css")).not.toMatch(/#002fa7|--jelly-blue/iu);
  });
});

describe("功能蓝只用于文字链接与焦点环", () => {
  const blocks = SOURCES.filter((entry) => !entry.file.endsWith("assets/css/tokens.css")).flatMap((entry) =>
    cssSourceOf(entry).flatMap((css) => cssBlocks(css).map((block) => ({ file: entry.file, ...block }))),
  );

  it("焦点蓝只出现在 :focus 选择器的 outline 声明里", () => {
    const offenders: { file: string; selector: string; line: string }[] = [];

    for (const block of blocks) {
      for (const line of block.declarations.split(/\r?\n/u)) {
        if (!line.includes("var(--color-focus)")) continue;
        const isOutline = /outline(?:-color|-offset)?\s*:/u.test(line.trim());
        const isFocusSelector = /:focus(?:-visible|-within)?/u.test(block.selector);
        if (!isOutline || !isFocusSelector) {
          offenders.push({ file: block.file, selector: block.selector, line: line.trim() });
        }
      }
    }

    expect(offenders).toEqual([]);
  });

  it("功能蓝不做铺底：组件里不再消费 --color-focus-soft，也没有 text/bg/border-focus 工具类", () => {
    const problems = SOURCES.filter((entry) => !entry.file.endsWith("assets/css/tokens.css")).flatMap((entry) => {
      const hits: string[] = [];
      if (entry.text.includes("var(--color-focus-soft)")) hits.push("--color-focus-soft");
      for (const match of entry.text.matchAll(
        /(?:text|bg|border|fill|stroke|ring)-focus(?:-soft)?\b/gu,
      )) {
        hits.push(match[0]);
      }
      for (const match of entry.text.matchAll(/var\(--color-focus\)[^;]*;/gu)) {
        // 已经由上面的块级规则覆盖；这里只收集明显是填充/字色的写法。
        if (/background|color\s*:/u.test(match[0])) hits.push(match[0]);
      }
      return hits.map((hit) => ({ file: entry.file, hit }));
    });

    expect(problems).toEqual([]);
  });

  it("链接蓝只出现在文字链接选择器（a / link）里", () => {
    const offenders: { file: string; selector: string }[] = [];

    for (const block of blocks) {
      if (!block.declarations.includes("var(--color-link)")) continue;
      const isLinkSelector = /(?:^|[\s,>+~])a(?=$|[:.\s,>+~])/u.test(block.selector) || /link/u.test(block.selector);
      if (!isLinkSelector) {
        offenders.push({ file: block.file, selector: block.selector });
      }
    }

    expect(offenders).toEqual([]);
  });
});

describe("形状语义退役", () => {
  it("页头不再有黄三角记号", () => {
    const source = read("app/components/layout/AppHeader.vue");

    expect(source).not.toContain("site-brand__mark");
    expect(source).not.toMatch(/border-bottom:\s*9px/u);
    expect(source).not.toMatch(/--jelly-yellow|bg-jelly-yellow|fill-jelly-yellow/u);
  });

  it("空态不再有三角/方/圆三色记号", () => {
    const source = read("app/components/profile/ProfileEmptyState.vue");

    expect(source).not.toContain("empty-state__mark");
    expect(source).not.toMatch(/fill-jelly-|\b(?:polygon|circle)\b/u);
  });

  it("页面与样式里不再有 40px 网格底纹", () => {
    for (const { file, text } of SOURCES) {
      expect(text, `${file} 仍有 40px 网格底纹`).not.toMatch(/40px\s*网格|background-size:\s*40px/u);
    }
    expect(read("app/assets/css/tokens.css")).not.toContain("40px 网格");
  });
});

describe("原先直接消费原始色相的组件改走语义层", () => {
  it("页脚是深墨面 + 反白字（不是克莱因蓝满铺）", () => {
    const source = read("app/components/layout/AppFooter.vue");

    expect(source).toContain("var(--color-surface-inverse)");
    expect(source).toContain("var(--color-on-inverse)");
    expect(source).toContain("var(--color-on-inverse-muted)");
    expect(source).not.toMatch(/--jelly-/u);
  });

  it("封面兜底是中性格石渐变 + 反白字（没有蓝色渐变、没有未定义 token）", () => {
    const source = read("app/components/article/ArticleCover.vue");

    expect(source).toContain("var(--color-surface-inverse)");
    expect(source).toContain("var(--color-on-inverse)");
    expect(source).not.toMatch(/--jelly-/u);
    expect(source).not.toMatch(/--color-on-strong-(?:soft|line)/u);
  });

  it("品牌字标倒色走反白语义层（不是原始奶油白）", () => {
    const source = read("app/components/brand/Wordmark.vue");

    expect(source).toContain("text-inverse");
    expect(source).not.toMatch(/text-jelly-cream/u);
  });

  it("激活 tab / 主按钮 / 阅读进度 / 目录激活都改中性语义层", () => {
    const tailwindCss = read("app/assets/css/tailwind.css");
    const isActive = (selector: string) => {
      const start = tailwindCss.indexOf(`${selector} {`);
      return start < 0 ? "" : tailwindCss.slice(start, tailwindCss.indexOf("}", start));
    };

    expect(isActive(".blueprint-tab.is-active")).toMatch(/background:\s*var\(--color-ink\)/u);
    expect(isActive(".blueprint-tab.is-active")).toMatch(/color:\s*var\(--color-on-strong\)/u);
    expect(isActive(".article-action--primary")).toMatch(/background:\s*var\(--color-ink\)/u);
    expect(isActive(".article-action--primary")).toMatch(/color:\s*var\(--color-on-strong\)/u);
    expect(isActive(".article-reading-progress__fill")).toMatch(/background:\s*var\(--color-ink\)/u);
    expect(isActive(".article-toc-link.is-active")).toMatch(/border-left-color:\s*var\(--color-ink\)/u);
  });
});

describe("写作页：不渲染站点页头，主题开关在创作页自己顶栏的最右端", () => {
  it("write 布局不渲染 AppHeader（创作页是专心写的一屏，不要站点导航）", () => {
    const source = read("app/layouts/write.vue");

    expect(source).not.toMatch(/<AppHeader\b/u);
    expect(source).not.toMatch(/AppHeader/u);
  });

  it("write 布局仍不写死亮色底（暗色下不发白）", () => {
    const source = read("app/layouts/write.vue");

    expect(source).toContain("bg-page");
    expect(source).toContain("text-ink");
    expect(source).not.toContain("#");
    expect(source).not.toContain("linear-gradient");
    expect(source).not.toMatch(/text-slate/iu);
  });

  it("主题开关落在 StudioWriteHeader 顶栏最右端：同一份 store、同一份文案、带无障碍标签", () => {
    const source = read("app/components/studio/StudioWriteHeader.vue");

    expect(source).toContain("useUiStore");
    expect(source).toContain("themeToggleLabel");
    expect(source).toContain("uiStore.toggleTheme()");
    expect(source).toContain('data-chrome="theme-toggle"');
    // 位置：在头像链接之后（模板里是 .write-page__actions 的最后一个元素）。
    expect(source.indexOf('data-chrome="theme-toggle"')).toBeGreaterThan(
      source.indexOf('class="write-avatar-link"'),
    );
  });
});
