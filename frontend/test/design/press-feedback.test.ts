import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 全站按下态（票 #427 / spec #423 第 1 批第 1 条）。
 *
 * 问题：自绘可点面在 pointer-down 那一刻什么都不发生，反馈要等到松手后的
 * hover 变色 —— 这是"手感发木"最大的单一来源。这里钉住三件事：
 *   1. **词表覆盖**：页签、按钮、条目卡、互动条、弹窗内次按钮都在 `:active` 词表里，
 *      并统一缩放一档；
 *   2. **取值来源**：时长只用 `var(--motion-fast)`、缓动只用 `var(--motion-ease)`，
 *      不出现新的毫秒/曲线字面量，也不新增裸色值；
 *   3. **不是 hover 效果**：按下态不放进 `(hover: hover)` / `(pointer: fine)` 媒体查询，
 *      触屏同样生效；减少动态下位移归零但保留底色/描边反馈（不是整段失效）。
 *
 * Naive 按钮（NButton）的颜色反馈由组件库自带，这里只钉"补同一档缩放"这一条规则，
 * 不逐组件抄。
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
 * 取包含 anchor 的那一段「减少动态」媒体块。
 *
 * 同一个媒体查询在文件里出现多次（玻璃层、路由过渡、通用兜底都有），所以按
 * 内容定位，不假设"第一段就是它"。
 */
function reducedBlock(source: string, anchor: string): string {
  let from = 0;
  for (;;) {
    const start = source.indexOf("@media (prefers-reduced-motion: reduce)", from);
    expect(start, `找不到包含 ${anchor} 的减少动态块`).toBeGreaterThan(-1);
    const open = source.indexOf("{", start);
    let depth = 0;
    for (let index = open; index < source.length; index += 1) {
      if (source[index] === "{") depth += 1;
      if (source[index] === "}") {
        depth -= 1;
        if (depth === 0) {
          const block = source.slice(open + 1, index);
          if (block.includes(anchor)) return block;
          from = start + 1;
          break;
        }
      }
    }
  }
}

/** 类名出现在选择器里（后面跟逗号 / 空格 / 花括号，避免 `foo` 命中 `foo-bar`）。 */
function selectorHas(text: string, className: string): boolean {
  const escaped = className.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
  return new RegExp(`\\.${escaped}(?=[,\\s{])`, "u").test(text);
}

/** 词表：spec #423 点到名的每一个自绘可点面。 */
const VOCABULARY = [
  // 页签
  "blueprint-tab",
  "soft-tab",
  "search-result-tab",
  "settings-tabs__tab",
  "message-center-tab",
  // 按钮 / 胶囊 / 图标按钮
  "article-action",
  "article-actions-bar__button",
  "comment-action",
  "comment-page-button",
  "comment-capsule",
  "comment-sheet__close",
  "profile-category-action",
  "studio-filter-chip",
  "studio-sidebar__link",
  "studio-sidebar__profile-link",
  "activity-heatmap__cell",
  "studio-article-row__action",
  "studio-folder-card__open",
  "article-grid-card__action",
  "message-row__delete",
  "favorite-chooser-row__toggle",
  "chip-select__trigger",
  "chip-select__chip",
  "chip-select__clear",
  // 条目卡
  "feed-entry",
  "article-grid-card",
  "studio-folder-card",
  "studio-article-row",
  "profile-folder-card",
  "profile-category-card",
  "favorite-chooser-row",
  "studio-chat-session",
] as const;

describe("按下态词表：自绘可点面统一回弹（scale .97）", () => {
  it("每一个自绘可点面都登记进词表，且都有 :active 规则", () => {
    const section = pressSection(tailwindCss());
    const baseSelectors = section.slice(0, section.indexOf("{"));

    const missingBase = VOCABULARY.filter(className => !selectorHas(baseSelectors, className));
    const missingActive = VOCABULARY.filter(
      className => !new RegExp(`\\.${className}:active`, "u").test(section),
    );

    expect(missingBase, `这些可点面没有登记进基态词表（松手不会回弹）：\n${missingBase.join("\n")}`).toEqual([]);
    expect(missingActive, `这些可点面没有 :active 按下态：\n${missingActive.join("\n")}`).toEqual([]);
  });

  it("按下统一缩到 .97，不是每个控件各写一档", () => {
    const block = blockAfter(pressSection(tailwindCss()), "blueprint-tab:active");

    expect(block).toMatch(/transform:\s*scale\(0?\.97\)/u);
    // 只缩放：颜色反馈归各控件/组件库自己的 hover 与选中态。
    expect(block).not.toMatch(/background(?:-color)?:/u);
    expect(block).not.toMatch(/(?:^|[;\s])color:/u);
  });

  it("基态登记 transform 过渡（松手才回弹），时长/缓动取 token", () => {
    const block = blockAfter(pressSection(tailwindCss()), "blueprint-tab,");

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

    // 同一特异性、靠后的规则会赢：若某个类在按下态之后又声明了 `transition`
    // （哪怕只写颜色），基态里的 `transform` 过渡就被整条替换掉 —— 按下能缩，
    // 松手却是瞬跳，正是本票要修的手感。所以词表区块必须是最后一段。
    const late = VOCABULARY.filter(className =>
      new RegExp(`\\.${className}(?=[,\\s{:.])`, "u").test(tail),
    );

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
    const active = blockAfter(section, ".n-button:active");

    expect(active).toMatch(/transform:\s*scale\(0?\.97\)/u);
    // 颜色反馈（含 pressed 态）仍由组件库的 :active / pressed 主题给出。
    expect(active).not.toMatch(/background(?:-color)?:/u);
    expect(active).not.toMatch(/(?:^|[;\s])color:/u);
  });

  it("基态把 transform 追加进组件库的颜色过渡，不整条覆盖掉颜色反馈", () => {
    const section = pressSection(tailwindCss());
    const base = blockAfter(section, ".n-button");

    expect(base).toContain("transform var(--motion-fast) var(--motion-ease)");
    expect(base).toContain("background-color");
    expect(base).toContain("border-color");
  });
});

describe("减少动态：位移归零，但按下反馈不整段失效", () => {
  it("自绘面在 reduce 下改为底色 / 描边反馈（不是 transition: none 一删了之）", () => {
    const block = reducedBlock(pressSection(tailwindCss()), "feed-entry:active");

    expect(block).toContain("transform: none");
    expect(block).toContain("border-color: var(--color-line-strong)");
    expect(block).toContain("background-color: var(--color-bg-subtle)");
  });

  it("Naive 按钮在 reduce 下只停用位移，颜色反馈留给组件库", () => {
    const media = reducedBlock(pressSection(tailwindCss()), ".n-button:active");
    const block = blockAfter(media, ".n-button:active");

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
    const block = reducedBlock(carouselStyle(), ".banner-dot:active");

    expect(block).toContain("transform: none");
    expect(block).toMatch(/border-color|background-color/u);
  });
});
