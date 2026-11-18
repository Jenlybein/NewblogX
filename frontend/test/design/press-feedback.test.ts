import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, blockAfterContaining, stripComments } from "../helpers/designFiles";

/**
 * 全站按下态（票 #427 / spec #423 第 1 批第 1 条）。
 *
 * 问题：自绘可点面在 pointer-down 那一刻什么都不发生，反馈要等到松手后的
 * hover 变色 —— 这是"手感发木"最大的单一来源。这里钉住四件事：
 *   1. **词表覆盖**：页签、按钮、条目卡、互动条、弹窗内次按钮都在同一份 `:is(...)`
 *      选择器表里，并统一缩放一档；
 *   2. **一份词表**：列表只出现一次，`:active` 与 reduce 覆盖是同一个规则里的嵌套
 *      分支 —— 新增一个可点面只改一处（票 437 评审，删掉了此前抄三遍的选择器）；
 *   3. **取值来源**：时长只用 `var(--motion-fast)`、缓动只用 `var(--motion-ease)`，
 *      不出现新的毫秒/曲线字面量，也不新增裸色值；
 *   4. **不是 hover 效果**：按下态不放进 `(hover: hover)` / `(pointer: fine)` 媒体查询，
 *      触屏同样生效；减少动态下位移归零但保留底色/描边反馈（不是整段失效）。
 *
 * Naive 按钮（NButton）的颜色反馈由组件库自带，这里只钉"补同一档缩放 + 只动
 * transform 的过渡"。
 *
 * 轮播箭头/圆点是 BannerCarousel.vue 的 scoped 类：全局 tailwind.css 再声明同名类会
 * 撞 `global-class-collision` 门禁（组件没声明的属性会被全局补齐），所以它们的按下态
 * 落回组件自己的样式块 —— 用例在文件后半段同样钉住。
 */

const projectDir = process.cwd();

function tailwindCss(): string {
  return readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");
}

function carouselVue(): string {
  return readFileSync(join(projectDir, "app/components/article/BannerCarousel.vue"), "utf8");
}

/**
 * 按下态区块在源码里的 `[start, end)` 区间。
 *
 * 从本章节注释的开头（`/* ====`）切：从章节标题中间切会让 `stripComments`
 * 拿不到配对的 `/*`，标题里的「票 427」会被裸色值正则当成 3 位十六进制颜色。
 *
 * 区块必须落在文件末尾（后面没有别的类定义），所以 `end` 取文件尾；万一未来又有
 * 章节排到它后面，退回「路由切换过渡」注释之前，至少不会把整段后续源码算进来。
 */
function pressSpan(source: string): [number, number] {
  const marker = source.indexOf("按下态（票 427）");
  expect(marker, "tailwind.css 里找不到按下态词表（票 427）").toBeGreaterThan(-1);
  const start = source.lastIndexOf("/*", marker);
  const next = source.indexOf("路由切换过渡", marker);
  const end = next > marker ? source.lastIndexOf("/*", next) : source.length;
  return [start, end];
}

function pressSection(source: string): string {
  const [start, end] = pressSpan(source);
  return stripComments(source.slice(start, end));
}

/**
 * 词表：从 CSS 的 `:is(...)` 列表里解析。
 *
 * 测试**不维护第四份名单**（票 437 评审判据）：唯一来源是源码里那份选择器表，
 * 这里只负责把它读出来，再拿它去验"每条规则都真的覆盖到它"。
 */
function pressVocabulary(section: string): string[] {
  const start = section.indexOf(":is(");
  expect(start, "词表不是以 :is(...) 分组的形式写的").toBeGreaterThan(-1);

  let depth = 0;
  let end = -1;
  for (let index = start + 4; index < section.length; index += 1) {
    if (section[index] === "(") depth += 1;
    if (section[index] === ")") {
      if (depth === 0) {
        end = index;
        break;
      }
      depth -= 1;
    }
  }
  expect(end, ":is(...) 没有闭合").toBeGreaterThan(start);

  const list = section.slice(start + 4, end);
  return [...list.matchAll(/\.([a-z0-9_-]+)/gu)].map((match) => match[1]!);
}

/** 类名出现在选择器里（后面跟逗号 / 空格 / 花括号 / 冒号，避免 `foo` 命中 `foo-bar`）。 */
function selectorHas(text: string, className: string): boolean {
  const escaped = className.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
  return new RegExp(`\\.${escaped}(?=[,\\s{:.])`, "u").test(text);
}

describe("按下态词表：一份选择器表 + 三条规则", () => {
  it("词表只写一处：`:is(...)` 是唯一列表，每个类名在整段只出现一次", () => {
    const section = pressSection(tailwindCss());
    const vocabulary = pressVocabulary(section);

    // 规模守卫：删到只剩两三个类名时判红（具体清单由源码自己维护）。
    expect(vocabulary.length, "按下态词表被删得只剩几个类名").toBeGreaterThanOrEqual(32);

    // 唯一性：类名不在 **:active / reduce 规则里** 被再抄一遍（那是三条规则的来源）。
    for (const className of vocabulary) {
      const escaped = className.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
      const occurrences = (section.match(new RegExp(`\\.${escaped}(?=[,\\s{:.])`, "gu")) ?? [])
        .length;
      expect(occurrences, `${className} 在按下态区块里出现了 ${occurrences} 次（应只写在词表里）`).toBe(1);
    }

    // 整段只有一份 `:is(` 列表；`:active` / reduce 都是嵌套分支（`&:active`）。
    expect(section.match(/:is\s*\(/gu) ?? []).toHaveLength(1);
    expect(section.match(/&:active/gu) ?? []).toHaveLength(2);
  });

  it("按下统一缩到 .97，不是每个控件各写一档", () => {
    const block = blockAfter(pressSection(tailwindCss()), "&:active {");

    expect(block).toMatch(/transform:\s*scale\(0?\.97\)/u);
    // 只缩放：颜色反馈归各控件/组件库自己的 hover 与选中态。
    expect(block).not.toMatch(/background(?:-color)?:/u);
    expect(block).not.toMatch(/(?:^|[;\s])color:/u);
  });

  it("基态登记 transform 过渡（松手才回弹），时长/缓动取 token", () => {
    const block = blockAfter(pressSection(tailwindCss()), ":is(");

    expect(block).toContain("transition:");
    expect(block).toContain("transform var(--motion-fast) var(--motion-ease)");
    expect(block).toContain("var(--motion-ease)");
  });

  it("不新增裸时长/裸曲线：整段只有 token 引用", () => {
    const section = pressSection(tailwindCss());

    expect(section).not.toMatch(/\b\d+(?:\.\d+)?m?s\b/u);
    expect(section).not.toContain("cubic-bezier(");
  });

  it("不新增裸色值（底色/描边一律语义 token）", () => {
    const section = pressSection(tailwindCss());

    expect(section).not.toMatch(/#[0-9a-fA-F]{3,8}(?![0-9a-zA-Z-])|(?:rgba?|hsla?)\(/u);
  });
});

describe("词表位置：基态规则压在所有可点面定义之后", () => {
  it("按下态区块之后不再出现词表类（否则后定义的 transition 会整条覆盖回弹过渡）", () => {
    const source = tailwindCss();
    const [, end] = pressSpan(source);
    const tail = stripComments(source.slice(end));
    const vocabulary = pressVocabulary(pressSection(source));

    // 同一特异性、靠后的规则会赢：若某个类在按下态之后又声明了 `transition`
    // （哪怕只写颜色），基态里的 `transform` 过渡就被整条替换掉 —— 按下能缩，
    // 松手却是瞬跳，正是本票要修的手感。所以词表区块必须是最后一段。
    const late = vocabulary.filter((className) => selectorHas(tail, className));

    expect(
      late,
      `这些可点面在按下态区块之后还有定义，会覆盖回弹过渡（把词表区块挪到文件末尾）：\n${late.join("\n")}`,
    ).toEqual([]);
  });
});

describe("按下反馈不是 hover 效果：触屏同样生效", () => {
  it("按下态不放进 hover / fine-pointer 媒体查询（否则 (hover: none) 的触屏没反馈）", () => {
    const section = pressSection(tailwindCss());

    expect(section).not.toMatch(/@media\s*\(\s*hover\s*:/u);
    expect(section).not.toMatch(/@media\s*\(\s*pointer\s*:/u);
    // 也不许写成 hover 之后才出现：`:hover:active` 等于"鼠标专属按下态"。
    expect(section).not.toMatch(/:hover[^{]*:active/u);
  });
});

describe("Naive 按钮：保留组件库颜色反馈，只补同一档缩放", () => {
  it("全局只一条 .n-button 缩放规则（弹窗内的次按钮一并覆盖）", () => {
    const section = pressSection(tailwindCss());
    const active = blockAfter(section, ".n-button:active {");

    expect(active).toMatch(/transform:\s*scale\(0?\.97\)/u);
    // 颜色反馈（含 pressed 态）仍由组件库的 :active / pressed 主题给出。
    expect(active).not.toMatch(/background(?:-color)?:/u);
    expect(active).not.toMatch(/(?:^|[;\s])color:/u);
  });

  it("基态只补 transform 过渡：不再替组件库决定颜色/边框/透明度的时长", () => {
    const section = pressSection(tailwindCss());
    const base = blockAfter(section, ".n-button.n-button {");

    expect(base).toContain("transform var(--motion-fast) var(--motion-ease)");
    // 票 437 评审：上一版把 color / background-color / border-color / opacity 统一改成
    // --motion-slow 并抬到 (0,2,0)，等于替组件库决定了它自己的过渡。
    expect(base).not.toContain("background-color");
    expect(base).not.toContain("border-color");
    expect(base).not.toContain("opacity");
    expect(base).not.toContain("--motion-slow");
  });
});

describe("减少动态：位移归零，但按下反馈不整段失效", () => {
  it("词表规则里的 reduce 覆盖改走底色 / 描边（不是 transition: none 一删了之）", () => {
    const section = pressSection(tailwindCss());
    const block = blockAfterContaining(
      section,
      "@media (prefers-reduced-motion: reduce)",
      "&:active",
    );

    expect(block).toContain("transform: none");
    expect(block).toContain("border-color: var(--color-line-strong)");
    expect(block).toContain("background-color: var(--color-bg-subtle)");
  });

  it("Naive 按钮在 reduce 下只停用位移，颜色反馈留给组件库", () => {
    const section = pressSection(tailwindCss());
    const block = blockAfterContaining(
      section,
      "@media (prefers-reduced-motion: reduce)",
      ".n-button:active",
    );

    expect(block).toContain("transform: none");
    expect(block).not.toMatch(/background(?:-color)?:/u);
  });
});

describe("轮播箭头 / 圆点：scoped 样式里同一档按下态", () => {
  function carouselStyle(): string {
    const match = /<style[^>]*scoped[^>]*>([\s\S]*?)<\/style>/u.exec(carouselVue());
    expect(match, "BannerCarousel.vue 里找不到 <style scoped>").not.toBeNull();
    return stripComments(match![1] ?? "");
  }

  it("箭头与圆点都有 :active 缩放，且基态登记 token 过渡", () => {
    const style = carouselStyle();

    const base = blockAfter(style, ".banner-arrow,");
    expect(base).toContain("transition: transform var(--motion-fast) var(--motion-ease)");

    const active = blockAfter(style, ".banner-arrow:active");
    expect(active).toMatch(/transform:\s*scale\(0?\.97\)/u);
    expect(style).toMatch(/\.banner-dot:active/u);
  });

  it("减少动态下停用缩放，改走反白描边 / 底色", () => {
    const block = blockAfterContaining(
      carouselStyle(),
      "@media (prefers-reduced-motion: reduce)",
      ".banner-dot:active",
    );

    expect(block).toContain("transform: none");
    expect(block).toMatch(/border-color|background-color/u);
  });
});
