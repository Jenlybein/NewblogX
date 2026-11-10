import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 轮播底部渐黑（首屏封面图层 1）的静态设计契约。
 *
 * 旧立场是"整幅罩层从 70% 压到 95%"：它保证任意底图上的反白字可读，代价是把
 * 上半张图也一起压暗，视觉上就是一块压住内容的大黑块，而且不支持 color-mix 的
 * 浏览器会退化成 100% 不透明的纯黑面。
 *
 * 新立场：罩层只压底部（上半完全透明），渐变从透明过渡到深墨；标题与控件都住在
 * 这块渐黑区里，控件不再另起一颗玻璃胶囊。测试因此不再钉"整幅""玻璃""胶囊"，
 * 而是钉：只覆盖底部、顶部透明、控件在渐黑区内、整块不出现实心深色面。
 *
 * 对比度不能只靠"看着还行"：这里把渐黑区最弱一档的透明度求出来，按 WCAG 公式
 * 叠在**纯白底图**（最不利情况）上再算对比度，与 closeout-contrast 用的是同一套
 * 亮度/对比度算法（那边钉 token 配对，这边钉"渐变叠图之后"的配对）。
 */
const projectDir = process.cwd();
const bannerVue = readFileSync(
  join(projectDir, "app/components/article/BannerCarousel.vue"),
  "utf8",
);
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

const VEIL = ".banner-veil {";
const veilBlock = () => blockAfter(bannerVue, VEIL);

/** `<template>` 段（不扫 `<style>`，避免把声明当成结构）。 */
function templateOf(source: string): string {
  const start = source.indexOf("<template>");
  const end = source.indexOf("</template>");
  expect(start, "组件没有 <template> 段").toBeGreaterThan(-1);
  expect(end, "组件没有 </template> 段").toBeGreaterThan(start);
  return source.slice(start, end);
}

/** tokens.css 里某个十六进制 token 的全部取值（亮色在前、暗色在后，按文件顺序）。 */
function hexesOf(token: string): string[] {
  const matches = [...tokensCss.matchAll(new RegExp(`${token}:\\s*#([0-9a-f]{6})`, "giu"))].map(
    (match) => `#${match[1]!.toLowerCase()}`,
  );
  expect(matches.length, `${token} 在 tokens.css 里至少有一份十六进制取值`).toBeGreaterThan(0);
  return matches;
}

/** 焦点色经一层 var() 引用功能蓝，按亮/暗顺序解回十六进制。 */
function focusHexes(): string[] {
  const declarations = [...tokensCss.matchAll(/--color-focus:\s*([^;]+);/gu)].map((match) =>
    match[1]!.trim(),
  );
  return declarations.map((raw, index) => {
    if (raw.startsWith("#")) return raw.toLowerCase();
    const ref = /var\((--[a-z0-9-]+)\)/u.exec(raw)?.[1];
    expect(ref, "焦点色应是十六进制或一层 var()").toBeDefined();
    return hexesOf(ref!)[index]!;
  });
}

function hexToRgb(hex: string): [number, number, number] {
  const value = hex.replace("#", "");
  return [0, 2, 4].map((index) => parseInt(value.slice(index, index + 2), 16)) as [
    number,
    number,
    number,
  ];
}

/** WCAG 相对亮度。 */
function luminance(hex: string): number {
  const linear = (channel: number) => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  };
  const [r, g, b] = hexToRgb(hex).map(linear) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(foreground: string, background: string): number {
  const [light, dark] = [luminance(foreground), luminance(background)].sort((a, b) => b - a) as [
    number,
    number,
  ];
  return (light + 0.05) / (dark + 0.05);
}

/** 把一层半透明深色叠在纯白底图上，返回合成后的十六进制色（最不利的底图）。 */
function compositeOverWhite(hex: string, alpha: number): string {
  const [r, g, b] = hexToRgb(hex);
  const mix = (channel: number) =>
    Math.round(channel * alpha + 255 * (1 - alpha))
      .toString(16)
      .padStart(2, "0");
  return `#${mix(r)}${mix(g)}${mix(b)}`;
}

describe("底部渐黑：只压底部，上半透明", () => {
  it("渐黑层贴在底部，高度由内容撑出来（不再写死占面板的百分比）", () => {
    const veil = veilBlock();

    expect(veil).toContain("inset: auto 0 0");
    expect(veil).not.toContain("inset: 0");
    // 具体像素高度交给内容（标题 + 控件），CSS 里只声明 height:auto。
    // 曾经写死 `height: 44%`：没标题时渐黑区里只剩一行控件，却仍压暗近半张图，
    // 观感就是"一块盖在轮播图上的大黑块"。
    expect(veil).toContain("height: auto");
    expect(veil).not.toMatch(/height:\s*[\d.]+%/u);
  });

  it("无标题时过渡带自动收窄，不留一条空压暗带", () => {
    expect(bannerVue).toContain(".banner-veil.is-titleless");
    const titleless = blockAfter(bannerVue, ".banner-veil.is-titleless {");

    expect(titleless, "无标题变体必须收窄上过渡带").toMatch(/padding-top:\s*clamp\(/u);
    const veil = veilBlock();
    const withTitle = /padding:\s*\n?\s*clamp\(([\d.]+)rem/u.exec(veil)?.[1];
    const without = /padding-top:\s*clamp\(([\d.]+)rem/u.exec(titleless)?.[1];
    expect(Number(withTitle), "有标题时上过渡带更长").toBeGreaterThan(Number(without));
  });

  it("模板按有没有标题挂 is-titleless，别让空标题也占一块压暗带", () => {
    const template = templateOf(bannerVue);
    expect(template).toMatch(/class="banner-veil"[\s\S]{0,80}is-titleless/u);
  });

  it("渐变自顶部透明过渡到深墨，不是一块实心深色面", () => {
    const veil = veilBlock();
    const backgrounds = [...veil.matchAll(/background:\s*([^;]+);/gu)].map((match) =>
      match[1]!.trim(),
    );

    expect(backgrounds.length).toBeGreaterThanOrEqual(1);
    for (const declaration of backgrounds) {
      // 每一条 background 都必须是渐变：降级分支也不许写实心色。
      expect(declaration, declaration).toContain("linear-gradient(");
    }
    expect(veil).toContain("transparent 0%");
    expect(veil).toContain("var(--color-surface-inverse)");
    expect(veil).not.toMatch(/background:\s*var\(--color-surface-inverse\)\s*;/u);
  });

  it("控件与文字所在的渐黑区保证对比度：叠在纯白底图上仍 ≥4.5:1", () => {
    const stops = [
      ...veilBlock().matchAll(
        /color-mix\(in srgb, var\(--color-surface-inverse\) (\d+)%, transparent\)/gu,
      ),
    ].map((match) => Number(match[1]));

    expect(stops.length, "渐黑至少要有两档半透明深墨").toBeGreaterThanOrEqual(2);
    const weakestAlpha = Math.min(...stops) / 100;
    expect(weakestAlpha).toBeGreaterThan(0);

    const inverses = hexesOf("--color-surface-inverse");
    const onInverses = hexesOf("--color-on-inverse");
    const pairs = inverses.map((inverse, index) => [inverse, onInverses[index] ?? onInverses[0]!] as const);

    for (const [inverse, onInverse] of pairs) {
      const background = compositeOverWhite(inverse, weakestAlpha);
      // 文字坐在更靠下的位置，透明度只比最弱一档更高；用最弱一档算是最保守的界。
      expect(contrast(onInverse, background), `${onInverse} on ${background}`).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe("控件并入渐黑区：不再单独一颗黑色胶囊", () => {
  it("控件在结构上嵌套在渐黑区里（不是与它并列的浮层）", () => {
    const template = templateOf(bannerVue);
    const veilAt = template.indexOf('class="banner-veil"');
    const controlsAt = template.indexOf('class="banner-controls"');

    expect(veilAt, "模板里找不到 .banner-veil").toBeGreaterThan(-1);
    expect(controlsAt, "模板里找不到 .banner-controls").toBeGreaterThan(veilAt);
    // 从渐黑区开到控件之间不许出现闭合标签：控件就是它的内容。
    expect(template.slice(veilAt, controlsAt)).not.toContain("</div>");
  });

  it("控件不再消费玻璃材质与胶囊底（它就是渐黑区里的一行内容）", () => {
    expect(stripComments(bannerVue)).not.toContain("glass-thin");

    const controls = blockAfter(bannerVue, ".banner-controls {");
    expect(controls).not.toContain("position: absolute");
    expect(controls).not.toMatch(/border-radius:\s*var\(--radius-pill\)/u);
    expect(controls).not.toMatch(/\bbackground\s*:/u);
  });

  it("控件的字色与图形走反白 token，不在深墨区上用纸面墨色", () => {
    expect(blockAfter(bannerVue, ".banner-arrow {")).toContain("color: var(--color-on-inverse)");
    expect(blockAfter(bannerVue, ".banner-dot span {")).toContain("var(--color-on-inverse-muted)");
    expect(blockAfter(bannerVue, ".banner-dot.is-active span {")).toContain(
      "var(--color-on-inverse)",
    );
  });

  it("键盘焦点环在渐黑区上可见：功能蓝描边 + 深墨垫环（对相邻色 ≥3:1）", () => {
    expect(bannerVue).toMatch(/:focus-visible[\s\S]*outline: 2px solid var\(--color-focus\)/u);
    expect(bannerVue).toMatch(/box-shadow:[^;]*var\(--color-surface-inverse\)/u);

    const focus = focusHexes();
    const inverses = hexesOf("--color-surface-inverse");
    expect(focus.length, "两套主题各有一份焦点色").toBe(inverses.length);
    for (const [index, inverse] of inverses.entries()) {
      // 非文字对比度门槛 3:1：蓝描边压深墨垫环，怎么看都在图上。
      expect(contrast(focus[index]!, inverse), `主题 ${index}`).toBeGreaterThanOrEqual(3);
    }
  });
});

describe("单张轮播：控件行的高度要补回来，标题不能掉到最下沿", () => {
  /**
   * 只有 1 张时圆点与箭头都不渲染（没有可操作的对象），渐黑区因此短一整行，
   * 标题就贴到面板最下沿 —— 用户报的"单张时样式不对"就是这个。
   *
   * 修法是把控件行本该占的高度补进下内边距。这里锁住"补出来的高度与有控件时
   * 逐项对齐"：控件行高度、标题与控件的间隙、下内边距三项都必须实测得到，
   * 任何一项改了而这里没跟着改，用例就会红。
   */
  const CONTROL_ROW_REM = 2.75; // .banner-arrow / .banner-dot 的 min-height: 44px

  it("补的高度 = 下内边距 + 控件行高度 + 标题与控件的间隙", () => {
    const single = blockAfter(bannerVue, ".banner-veil.is-single {");

    expect(single, "单张变体必须显式补 padding-bottom").toMatch(/padding-bottom:\s*calc\(/u);
    // 三段都从源码实测，不在这里抄死一份"应该是什么"。
    const veilBottom = /padding:\s*\n?\s*clamp\([\d.]+rem, [\d.]+vw, [\d.]+rem\) clamp\([\d.]+rem, [\d.]+vw, [\d.]+rem\) clamp\(([\d.]+)rem, ([\d.]+)vw, ([\d.]+)rem\)/u.exec(
      veilBlock(),
    );
    expect(veilBottom, "渐黑区下内边距应是一段 clamp()").not.toBeNull();
    const gap = /gap:\s*clamp\(([\d.]+)rem, ([\d.]+)vw, ([\d.]+)rem\)/u.exec(
      blockAfter(bannerVue, ".banner-veil__body {"),
    );
    expect(gap, "标题与控件的间隙应是 `body` 的 gap").not.toBeNull();

    // 把三项拼成补出来的那段文本，确认它确实出现在 padding-bottom 里。
    const parts = [
      `clamp(${veilBottom![1]}rem, ${veilBottom![2]}vw, ${veilBottom![3]}rem)`,
      `${CONTROL_ROW_REM}rem`,
      `clamp(${gap![1]}rem, ${gap![2]}vw, ${gap![3]}rem)`,
    ];
    const flat = single!.replace(/\s+/gu, " ");
    for (const part of parts) {
      expect(flat, `补出来的高度里缺少「${part}」`).toContain(part);
    }
  });

  it("控件行确实是 44px：少于它标题就会比多张时更低", () => {
    // 触控区 44px 是既有的无障碍约定（圆点/箭头都不许更小）。
    expect(blockAfter(bannerVue, ".banner-arrow {")).toContain("min-height: 44px");
    expect(blockAfter(bannerVue, ".banner-dot {")).toContain("min-height: 44px");
    // 2.75rem 与 44px 必须同值（默认根字号 16px）。
    expect(CONTROL_ROW_REM * 16).toBe(44);
  });

  it("只在「有标题且没有控件」时挂 is-single，空标题不补（否则是一条空压暗带）", () => {
    expect(bannerVue).toMatch(/singleWithoutControls/u);
    expect(bannerVue).toMatch(/Boolean\(titleText\.value\) && !hasControls\.value/u);
    const template = templateOf(bannerVue);
    expect(template).toMatch(/class="banner-veil"[\s\S]{0,180}'is-single': singleWithoutControls/u);
  });
});

describe("没有遮挡内容的大黑块：图层不会整幅压暗", () => {
  it("旧的整幅罩层退场，底图层与舞台层都不画底色", () => {
    expect(bannerVue).not.toContain("banner-scrim");
    expect(blockAfter(bannerVue, ".banner-photo {")).not.toContain("background");
    expect(blockAfter(bannerVue, ".banner-stage {")).not.toContain("background");
  });

  it("图片加载失败走浅色占位，不落成整块纯黑", () => {
    expect(bannerVue).toContain(".banner-photo.is-failed");
    const failed = blockAfter(bannerVue, ".banner-photo.is-failed {");

    expect(failed).toContain("linear-gradient(");
    expect(failed).not.toMatch(/background:\s*var\(--color-surface-inverse\)\s*;/u);
  });

  it("图片的 error 事件会把底图层切到失败态", () => {
    expect(bannerVue).toMatch(/<img[\s\S]*?@error=/u);
    expect(bannerVue).toContain("is-failed");
  });
});
